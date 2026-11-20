import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync,mkdirSync,readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join,resolve } from 'node:path'
import { randomUUID } from 'node:crypto'
import { DesktopTasks } from '../lib/desktop.js'
import { resolveConfig,apply } from '../lib/index.js'
import { assertSupportedJsonSchema } from '@deepseek-ai/dsh-tools'
import { jsonSchemaToTs,renderToolsSdk } from '@deepseek-ai/dsh-tool-executor-ptc'

function fixture(){const home=mkdtempSync(join(tmpdir(),'agy-desktop-')),workspace=join(home,'work');mkdirSync(workspace);return {home,context:{owner:'desktop-session',workspace},env:{PATH:process.env.PATH,HOME:home},config:resolveConfig({command:resolve('test/fixtures/fake-agy.js'),disposeGraceMs:50})}}
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
    assert.match(renderToolsSdk([{name:tool.name,description:tool.description,parameters:tool.parameters,output:tool.output.schema}]),/subagent_antigravity/)
    const start=await commands.get('antigravity').handler({agent,rawInput:JSON.stringify({version:1,action:'start',prompt:'direct'})});assert.equal(start.kind,'success')
    const result=await tool.execute({prompt:'background',run_in_background:true},{});assert.equal(result.kind,'background');assert.equal((await jobs.get(result.jobId).done).status,'completed')
    preset='other';await assert.rejects(()=>tool.execute({prompt:'forbidden'},{}),/current committed preset/)
    preset='heavy';events.get('agent-preset/selected')(f.context.owner);assert.ok(tools.get('subagent_antigravity'))
  }finally{await events.get('dispose')?.();process.env.HOME=oldHome}
})
