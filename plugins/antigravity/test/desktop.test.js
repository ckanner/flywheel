import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync,mkdirSync,readFileSync,realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join,resolve } from 'node:path'
import { randomUUID } from 'node:crypto'
import { DesktopTasks } from '../lib/desktop.js'
import { resolveConfig,apply } from '../lib/index.js'
import { assertSupportedJsonSchema } from '@deepseek-ai/dsh-tools'
import { jsonSchemaToTs,renderToolsSdk } from '@deepseek-ai/dsh-tool-executor-ptc'

function fixture(){const home=realpathSync(mkdtempSync(join(tmpdir(),'agy-desktop-'))),workspace=join(home,'work');mkdirSync(workspace);return {home,context:{owner:'desktop-session',workspace},env:{PATH:process.env.PATH,HOME:home},config:resolveConfig({command:resolve('test/fixtures/fake-agy.js'),disposeGraceMs:50})}}
test('manual actions use the same runner, bind ownership, and make duplicate start idempotent',async()=>{
  const f=fixture(),tasks=new DesktopTasks({getConfig:()=>f.config,env:f.env}),id=randomUUID(),action={version:1,action:'start',invocationId:id,prompt:'task'}
  try{
    const start=await tasks.action(f.context,action);assert.equal(start.status,'running')
    assert.equal((await tasks.action(f.context,action)).invocationId,id)
    await assert.rejects(()=>tasks.action({...f.context,owner:'another'},{version:1,action:'cancel',invocationId:id}),/ENOENT|ownership/)
    const result=await tasks.wait(f.context,id);assert.equal(result.status,'completed',JSON.stringify(result.failure));assert.equal(result.receipt.ok,true)
    await assert.rejects(()=>tasks.action(f.context,{...action,prompt:'changed'}),/identity conflicts/)
    await assert.rejects(()=>tasks.action({...f.context,owner:'another'},{version:1,action:'start',prompt:'resume',sessionId:result.session.id}),/owned by this Desktop/)
    const restarted=new DesktopTasks({getConfig:()=>f.config,env:f.env});assert.equal((await restarted.action(f.context,{version:1,action:'result',invocationId:id})).status,'completed');await restarted.dispose()
  }finally{await tasks.dispose()}
})
test('fresh defaults apply only to future tasks; tools cannot widen permissions',async()=>{
  const f=fixture();let config=f.config
  const tasks=new DesktopTasks({getConfig:()=>config,env:f.env})
  try{
    const first=await tasks.action(f.context,{version:1,action:'start',prompt:'first'})
    config=resolveConfig({...config,model:'gemini-4-neon-high'})
    const next=await tasks.action(f.context,{version:1,action:'start',prompt:'next'})
    assert.equal((await tasks.wait(f.context,first.invocationId)).requested.model,'gemini-4-hydrogen-high')
    assert.equal((await tasks.wait(f.context,next.invocationId)).requested.model,'gemini-4-neon-high')
    await assert.rejects(()=>tasks.action(f.context,{version:1,action:'start',prompt:'not allowed',permissionMode:'always-proceed'},{direct:false}),/cannot expand/)
  }finally{await tasks.dispose()}
})
test('published DSH schema/PTC contracts, direct command, committed preset and jobs',async()=>{
  const f=fixture(),oldHome=process.env.HOME;process.env.HOME=f.home
  const events=new Map(),tools=new Map(),commands=new Map(),jobs=new Map();let preset='daily'
  const agent={id:'agent-1',session:{id:f.context.owner,header:{cwd:f.context.workspace,agentPreset:'daily'}},ctx:{tools:{register:tool=>tools.set(tool.name,tool)}}}
  const ctx={commands:{register:command=>commands.set(command.name,command)},workingDirectory:{get:()=>f.context.workspace},on:(event,fn)=>events.set(event,fn),get:name=>name==='agentPresets'?{composedPreset:()=>preset}:name==='agents'?{get:()=>agent}:name==='jobs'?{start:spec=>{const id=randomUUID();jobs.set(id,spec.run({append(){},updateProgress(){}}));return id}}:undefined}
  try{
    apply(ctx,f.config);events.get('agent/created')({agent});const tool=tools.get('subagent_antigravity');assert.ok(tool)
    assertSupportedJsonSchema(tool.parameters);assertSupportedJsonSchema(tool.output.schema)
    assert.match(jsonSchemaToTs(tool.parameters),/prompt: string/);assert.match(jsonSchemaToTs(tool.output.schema),/jobId: string/)
    assert.match(jsonSchemaToTs(tool.parameters),/maxObservedAgents/);assert.match(jsonSchemaToTs(tool.parameters),/"swarm"/)
    assert.match(renderToolsSdk([{name:tool.name,description:tool.description,parameters:tool.parameters,output:tool.output.schema}]),/subagent_antigravity/)
    const start=await commands.get('antigravity').handler({agent,rawInput:JSON.stringify({version:1,action:'start',prompt:'direct'})});assert.equal(start.kind,'success')
    const result=await tool.execute({prompt:'background',run_in_background:true},{});assert.equal(result.kind,'background');assert.equal((await jobs.get(result.jobId).done).status,'completed')
    preset='other';await assert.rejects(()=>tool.execute({prompt:'forbidden'},{}),/current committed preset/)
    preset='heavy';events.get('agent-preset/selected')(f.context.owner);assert.ok(tools.get('subagent_antigravity'))
  }finally{await events.get('dispose')?.();process.env.HOME=oldHome}
})
test('DSH defaults and per-call selections preserve native profiles and derive fixed effort from the selected model',async()=>{
  const f=fixture();f.env.FIXTURE_SCENARIO='children'
  let config=resolveConfig({...f.config,model:'gemini-pro-orchestrator-neon',effort:'',native:{version:1,mode:'swarm',maxObservedAgents:3}})
  const tasks=new DesktopTasks({getConfig:()=>config,env:f.env})
  try{
    const first=await tasks.action(f.context,{version:1,action:'start',prompt:'configured swarm'})
    config=resolveConfig({...f.config,native:{version:1,mode:'native'}})
    const second=await tasks.action(f.context,{version:1,action:'start',prompt:'explicit subagents',model:'gemini-pro-orchestrator',native:{version:1,mode:'subagents',maxObservedAgents:2}},{direct:false})
    const [a,b]=await Promise.all([tasks.wait(f.context,first.invocationId),tasks.wait(f.context,second.invocationId)])
    assert.equal(a.status,'completed',JSON.stringify(a.failure));assert.equal(a.requested.native.mode,'swarm');assert.equal(a.requested.effort,'')
    assert.equal(b.status,'completed',JSON.stringify(b.failure));assert.equal(b.requested.native.mode,'subagents');assert.equal(b.requested.effort,'')
    assert.equal(b.native.observedAgents,2);assert.equal(b.native.children[1].state,'unknown')
    await assert.rejects(()=>tasks.action(f.context,{version:1,action:'start',prompt:'invalid',native:null}),/unsupported fields/)
    assert.equal(resolveConfig({model:'gemini-pro-orchestrator-argon'}).effort,'')
    assert.throws(()=>resolveConfig({model:'gemini-pro-orchestrator',effort:'high'}),/fixed effort/)
  }finally{await tasks.dispose()}
})
test('DSH status exposes live child observations and cancel remains bound to its invocation',async()=>{
  const f=fixture();f.env.FIXTURE_SCENARIO='children-hang'
  const tasks=new DesktopTasks({getConfig:()=>f.config,env:f.env})
  try{
    const started=await tasks.action(f.context,{version:1,action:'start',prompt:'observe children',native:{version:1,mode:'subagents'}})
    let live
    for(let i=0;i<200;i++){
      live=await tasks.action(f.context,{version:1,action:'status',invocationId:started.invocationId})
      if(live.native.observedAgents===2)break
      await new Promise(resolve=>setTimeout(resolve,20))
    }
    assert.equal(live.status,'running');assert.equal(live.native.observedAgents,2);assert.equal(live.native.completion.state,'unknown')
    await tasks.action(f.context,{version:1,action:'cancel',invocationId:started.invocationId})
    const finished=await tasks.wait(f.context,started.invocationId)
    assert.equal(finished.status,'unknown');assert.equal(finished.receipt.execution.cancelled,true);assert.equal(finished.native.children[0].state,'unknown')
    assert.equal(finished.cleanup.confirmed,true)
  }finally{await tasks.dispose()}
})
test('resuming an omitted native binding does not inherit a newly saved Swarm default',async()=>{
  const f=fixture();let config=f.config
  const tasks=new DesktopTasks({getConfig:()=>config,env:f.env})
  try{
    const first=await tasks.action(f.context,{version:1,action:'start',prompt:'original ordinary task'})
    const original=await tasks.wait(f.context,first.invocationId);assert.equal(original.status,'completed')
    assert.equal(Object.hasOwn(original.requested,'native'),false)
    config=resolveConfig({...f.config,native:{version:1,mode:'swarm'}})
    const next=await tasks.action(f.context,{version:1,action:'start',prompt:'same conversation follow-up',sessionId:original.session.id})
    const resumed=await tasks.wait(f.context,next.invocationId)
    assert.equal(resumed.status,'completed',JSON.stringify(resumed.failure));assert.equal(Object.hasOwn(resumed.requested,'native'),false)
    const changed=await tasks.action(f.context,{version:1,action:'start',prompt:'changed exact profile',sessionId:original.session.id,native:{version:1,mode:'native'}})
    const rejected=await tasks.wait(f.context,changed.invocationId)
    assert.equal(rejected.receipt.execution.dispatch,'not-dispatched');assert.match(rejected.failure.detail,/identical/)
  }finally{await tasks.dispose()}
})
