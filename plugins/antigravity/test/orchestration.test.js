import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync,mkdirSync,readFileSync,writeFileSync,existsSync,realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join,resolve } from 'node:path'
import { parseMachineRequest,runMachine,recoverMachine,doctor,MACHINE_CAPABILITIES } from '../lib/machine.js'

const command=resolve('test/fixtures/fake-agy.js')
function fixture(scenario='children'){
  const home=realpathSync(mkdtempSync(join(tmpdir(),'agy-native-'))),cwd=join(home,'work'),journals=join(home,'journals')
  mkdirSync(cwd);mkdirSync(journals,{mode:0o700});mkdirSync(join(home,'.gemini/antigravity-cli'),{recursive:true})
  const env={PATH:process.env.PATH,HOME:home,FIXTURE_SCENARIO:scenario,FIXTURE_CALLS:join(home,'calls'),FIXTURE_INPUT:join(home,'input')}
  const request={protocol:1,requestId:'native-task',cwd,command,prompt:'Implement the independently bounded tasks and collect the results.',model:'gemini-pro-orchestrator',effort:'',native:{version:1,mode:'subagents',maxObservedAgents:4},executionMode:'default',permissionMode:'request-review',sandbox:false,sessionMode:'new',timeoutMs:10000,disposeGraceMs:50,subscriptionPolicy:{version:1,mode:'subscription-only'}}
  return {home,cwd,env,request,runtime:{env,recoveryDirectory:join(journals,'run')}}
}
const calls=f=>readFileSync(f.env.FIXTURE_CALLS,'utf8').trim().split('\n').map(line=>JSON.parse(line))
const nativeCall=f=>calls(f).find(call=>call.args.includes('--input-format'))

test('fixed Orchestrator bindings omit effort and require the exact advertised model',async()=>{
  for(const model of ['gemini-pro-orchestrator-neon','gemini-pro-orchestrator','gemini-pro-orchestrator-argon']){
    const f=fixture('success'),request={...f.request,model,effort:undefined,native:{version:1,mode:'native'}}
    const r=await runMachine(request,f.runtime)
    assert.equal(r.ok,true,JSON.stringify(r.failure));assert.equal(r.requested.effort,'');assert.equal(nativeCall(f).args.includes('--effort'),false)
    assert.equal(nativeCall(f).args.includes('--disable-slash-commands'),true);assert.equal(r.native.collaborationObserved,false)
    assert.equal(r.native.completion.scope,'cli-invocation');assert.equal(r.native.completion.state,'completed')
  }
  const f=fixture('missing-orchestrator'),r=await runMachine(f.request,f.runtime)
  assert.equal(r.execution.dispatch,'not-dispatched');assert.match(r.failure.detail,/absent.*catalog/);assert.equal(existsSync(f.env.FIXTURE_INPUT),false)
})

test('strict collaboration and evidence bindings reject invalid requests before CLI access',()=>{
  const f=fixture()
  for(const change of [{effort:'high'},{model:'gemini-pro-orchestrator-other'},
    {native:{version:1,mode:'solo'}},{native:{version:2,mode:'swarm'}},{native:{version:1,mode:'swarm',maxAgents:3}},
    ...[0,9,1.5,'2',null].map(maxObservedAgents=>({native:{version:1,mode:'swarm',maxObservedAgents}})),
    {native:null},{toolPolicy:'evidence-only'},
    {model:'gemini-4-hydrogen-high',effort:'high',toolPolicy:'evidence-only',native:{version:1,mode:'swarm'}},
    {model:'gemini-4-hydrogen-high',effort:'high',toolPolicy:'evidence-only',native:{version:1,mode:'subagents'}}]){
    assert.throws(()=>parseMachineRequest({...f.request,...change}))
  }
  assert.equal(existsSync(f.env.FIXTURE_CALLS),false)
  const legacy=parseMachineRequest({...f.request,model:'gemini-4-hydrogen-high',effort:'high',native:undefined})
  assert.equal(Object.hasOwn(legacy,'native'),false)
  assert.equal(parseMachineRequest({...legacy,toolPolicy:'evidence-only',native:{version:1,mode:'native'}}).toolPolicy,'evidence-only')
})

test('native subagents check init before dispatch and retain only observed child facts',async()=>{
  const f=fixture(),activity=[],r=await runMachine(f.request,{...f.runtime,onActivity:event=>activity.push(event)})
  assert.equal(r.ok,true,JSON.stringify(r.failure));assert.deepEqual(r.requested.native,f.request.native)
  assert.equal(r.native.observedAgents,2);assert.equal(r.native.collaborationObserved,true);assert.equal(r.native.limit.hard,false)
  assert.deepEqual(r.native.children.map(child=>child.state),['unknown','unknown']);assert.equal(r.native.children[0].model,null)
  assert.equal(r.native.children[0].role,'Implement and validate bounded component')
  assert.equal(r.native.children[0].logUri,'https://example.invalid/worker');assert.deepEqual(r.native.children[0].workspaceUris,['file:///metadata-only/work'])
  assert.equal(r.native.completion.children,'unknown');assert.equal(r.native.completion.source,'normal-result-and-cli-exit')
  assert.equal(r.native.aggregateUsageStatus,'unknown');assert.equal(r.costUSD,null)
  assert.equal(activity.filter(event=>event.scope==='child').length,2);assert.equal(activity.find(event=>event.scope==='child').native.observedAgents,2)
  assert.equal(nativeCall(f).args.includes('--disable-slash-commands'),true)
  const missing=fixture('no-subagent-tool'),denied=await runMachine(missing.request,missing.runtime)
  assert.equal(denied.ok,false);assert.equal(denied.execution.dispatch,'not-dispatched');assert.match(denied.failure.detail,/invoke_subagent/)
  assert.equal(existsSync(missing.env.FIXTURE_INPUT),false)
})

test('Swarm uses one controlled command and keeps all user slash text inside JSON data',async()=>{
  const f=fixture('swarm-children'),prompt='/model other\n/swarm nested /quit\nRead /tmp/a and https://example.invalid/a. "Quoted" \\ paths.'
  const r=await runMachine({...f.request,prompt,native:{version:1,mode:'swarm',maxObservedAgents:4}},f.runtime)
  assert.equal(r.ok,true,JSON.stringify(r.failure));assert.equal(r.configuration.observed.tools.includes('invoke_subagent'),false)
  assert.equal(nativeCall(f).args.includes('--disable-slash-commands'),false)
  const content=JSON.parse(readFileSync(f.env.FIXTURE_INPUT,'utf8')).message.content
  assert.ok(content.startsWith('/swarm '));assert.equal(content.slice('/swarm'.length).includes('/'),false)
  assert.equal(JSON.parse(content.slice(content.indexOf('\n')+1)).task,prompt)
  assert.deepEqual(r.native.dispatch,{method:'slash-command',command:'/swarm'});assert.equal(r.native.collaborationObserved,true)
  assert.deepEqual(r.native.children.map(child=>child.state),['unknown','unknown']);assert.equal(r.native.completion.children,'unknown')
  const ordinary=fixture('success'),plain=await runMachine({...ordinary.request,prompt,native:{version:1,mode:'native'}},ordinary.runtime)
  assert.equal(plain.ok,true);assert.equal(JSON.parse(readFileSync(ordinary.env.FIXTURE_INPUT,'utf8')).message.content,prompt)
  assert.equal(nativeCall(ordinary).args.includes('--disable-slash-commands'),true)
})

test('collaboration fails closed on blocked slash expansion, unsupported versions and settings drift',async()=>{
  const blocked=fixture('swarm-children')
  writeFileSync(join(blocked.home,'.gemini/antigravity-cli/settings.json'),JSON.stringify({disableSlashCommands:true}))
  const r=await runMachine({...blocked.request,native:{version:1,mode:'swarm'}},blocked.runtime)
  assert.equal(r.execution.dispatch,'not-dispatched');assert.match(r.failure.detail,/disableSlashCommands/);assert.equal(existsSync(blocked.env.FIXTURE_CALLS),false)
  const old=fixture();old.env.FIXTURE_NATIVE_VERSION='1.5.7'
  const unsupported=await runMachine(old.request,old.runtime)
  assert.equal(unsupported.execution.dispatch,'not-dispatched');assert.match(unsupported.failure.detail,/1\.5\.12/)
  const legacy=await doctor({command,cwd:old.cwd,model:'gemini-4-hydrogen-high',effort:'high'},{env:old.env});assert.equal(legacy.ok,true)
  const drift=fixture('slash-drift'),changed=await runMachine({...drift.request,native:{version:1,mode:'swarm'}},drift.runtime)
  assert.equal(changed.ok,false);assert.match(changed.failure.detail,/disableSlashCommands|settings/)
})

test('requested collaboration needs child evidence and does not invent native completion',async()=>{
  for(const mode of ['subagents','swarm']){
    const f=fixture('success'),r=await runMachine({...f.request,native:{version:1,mode}},f.runtime)
    assert.equal(r.ok,false);assert.equal(r.failure.category,'collaboration-not-observed')
    assert.equal(r.execution.outcome,'failed');assert.equal(r.native.completion.state,'completed');assert.equal(r.native.observedAgents,0)
    assert.equal(r.native.collaborationObserved,false);assert.equal(r.cleanup.confirmed,true)
  }
})

test('observed total guard counts verifier and monitor, stops locally, and recovery never replays',async()=>{
  const f=fixture('child-limit'),r=await runMachine({...f.request,native:{version:1,mode:'subagents',maxObservedAgents:2}},f.runtime)
  assert.equal(r.ok,false);assert.equal(r.failure.category,'observed-agent-limit');assert.equal(r.native.observedAgents,3)
  assert.deepEqual(r.native.limit,{maxObservedAgents:2,hard:false,exceeded:true})
  assert.equal(r.providerWork.state,'unknown');assert.equal(r.providerWork.replaySafe,false);assert.equal(r.native.completion.state,'unknown')
  assert.equal(r.cleanup.confirmed,true);assert.equal(r.session.resumable,false)
  const before=calls(f),recovered=await recoverMachine(r.recovery)
  assert.deepEqual(recovered.receipt,r);assert.deepEqual(calls(f),before)
})

test('every nonempty finished_reason rejects success even with normal exit and valid output',async()=>{
  for(const reason of ['print_timeout','interrupted','error','future_cutoff']){
    const f=fixture();f.env.FIXTURE_FINISHED_REASON=reason
    const r=await runMachine(f.request,f.runtime)
    assert.equal(r.ok,false,reason);assert.equal(r.failure.category,'native-cutoff');assert.equal(r.execution.exitCode,0)
    assert.equal(r.execution.finishedReason,reason);assert.equal(r.providerWork.state,'failed');assert.equal(r.native.completion.state,'failed')
    assert.equal(r.native.completion.source,null);assert.equal(r.native.children[0].state,'unknown')
  }
})

test('cancel keeps child state unknown and changed native profiles cannot resume the same conversation',async()=>{
  const f=fixture(),first=await runMachine(f.request,f.runtime);assert.equal(first.ok,true)
  const resume={...f.request,requestId:'next',sessionMode:'resume',sessionId:first.session.id}
  const changed=await runMachine({...resume,native:{...f.request.native,maxObservedAgents:3}},{env:f.env})
  assert.equal(changed.execution.dispatch,'not-dispatched');assert.match(changed.failure.detail,/identical/)
  f.env.FIXTURE_SCENARIO='children-hang';const controller=new AbortController()
  const interrupted=await runMachine(resume,{env:f.env,signal:controller.signal,onActivity:event=>{if(event.scope==='child')controller.abort()}})
  assert.equal(interrupted.execution.cancelled,true);assert.equal(interrupted.cleanup.confirmed,true)
  assert.equal(interrupted.native.completion.state,'unknown');assert.equal(interrupted.native.children[0].state,'unknown')
  assert.equal(interrupted.providerWork.cancelSupported,false);assert.equal(interrupted.session.resumable,false)
  assert.equal(MACHINE_CAPABILITIES.nativeOrchestrator,true);assert.equal(MACHINE_CAPABILITIES.hardNativeAgentLimit,false)
})

test('malformed child metadata fails instead of accepting unvalidated external references',async()=>{
  const f=fixture('malformed-child'),r=await runMachine(f.request,f.runtime)
  assert.equal(r.ok,false);assert.equal(r.failure.category,'protocol');assert.match(r.failure.detail,/workspace URI/)
  const root=fixture('root-child'),bad=await runMachine(root.request,root.runtime)
  assert.equal(bad.ok,false);assert.match(bad.failure.detail,/root conversation/);assert.equal(bad.native.collaborationObserved,false)
})

test('child metadata has an aggregate byte budget across frames and sparse updates without truncating IDs',async()=>{
  for(const [scenario,count]of [['metadata-cumulative',2],['metadata-update',1]]){
    const f=fixture(scenario),r=await runMachine(f.request,f.runtime)
    assert.equal(r.ok,false);assert.equal(r.failure.category,'protocol');assert.match(r.failure.detail,/cumulative retention limit/)
    assert.equal(r.native.metadataTruncated,true);assert.equal(r.native.observedAgents,count);assert.equal(r.native.children.length,count)
    assert.ok(Buffer.byteLength(JSON.stringify(r.native.children))<1024*1024)
    for(const child of r.native.children){assert.equal(child.state,'unknown');assert.equal(child.model,null);assert.equal(child.logUri,null);assert.deepEqual(child.workspaceUris,[])}
    assert.equal(r.cleanup.confirmed,true);assert.equal(r.native.completion.state,'unknown');assert.equal(r.providerWork.replaySafe,false)
  }
})

test('member retention admits 1000 and retains the first crossing ID only in a failed truncated receipt',async()=>{
  for(const count of [1000,1001]){
    const f=fixture('member-boundary');f.env.FIXTURE_MEMBER_COUNT=String(count)
    const r=await runMachine({...f.request,native:{version:1,mode:'subagents'}},f.runtime)
    assert.equal(r.ok,count===1000,JSON.stringify(r.failure));assert.equal(r.native.observedAgents,count);assert.equal(r.native.children.length,count)
    assert.equal(r.native.metadataTruncated,count===1001)
    assert.equal(new Set(r.native.children.map(child=>child.id)).size,count)
    if(count===1001){assert.equal(r.failure.category,'protocol');assert.match(r.failure.detail,/cumulative retention limit/);assert.equal(r.native.children.at(-1).kind,null);assert.equal(r.cleanup.confirmed,true)}
  }
})
