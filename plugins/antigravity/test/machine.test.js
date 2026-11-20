import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, existsSync, readdirSync, realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { runMachine, recoverMachine, parseMachineRequest, doctor } from '../lib/machine.js'

const command=resolve('test/fixtures/fake-agy.js')
function fixture(scenario='success') {
  const home=realpathSync(mkdtempSync(join(tmpdir(),'agy-test-'))),cwd=join(home,'work'),journals=join(home,'journals');mkdirSync(cwd);mkdirSync(journals,{mode:0o700});mkdirSync(join(home,'.gemini','antigravity-cli'),{recursive:true})
  const env={PATH:process.env.PATH,HOME:home,FIXTURE_SCENARIO:scenario,FIXTURE_CALLS:join(home,'calls'),FIXTURE_INPUT:join(home,'input'),FIXTURE_PID:join(home,'pid'),FIXTURE_CHILD_PID:join(home,'child-pid')}
  const request={protocol:1,requestId:'task-1',cwd,command,prompt:'hello',model:'gemini-4-hydrogen-high',effort:'high',executionMode:'default',permissionMode:'request-review',sandbox:false,sessionMode:'new',timeoutMs:10000,disposeGraceMs:50,subscriptionPolicy:{version:1,mode:'subscription-only'}}
  return {home,cwd,env,request,runtime:{env,recoveryDirectory:join(journals,'run')}}
}
test('valid stream persists receipts, authenticates model and exact resume binding',async()=>{
  const f=fixture(),events=[]
  const receipt=await runMachine(f.request,{...f.runtime,onLifecycle:e=>events.push(e)})
  assert.equal(receipt.ok,true,JSON.stringify(receipt.failure));assert.equal(receipt.output[0].text,'done');assert.equal(receipt.usage.scope,'conversation');assert.equal(receipt.costUSD,null)
  assert.deepEqual(events.map(e=>e.event),['dispatch-intent','dispatch-possible','terminal','cleanup-confirmed'])
  assert.equal(events[0].configuration,null)
  assert.equal(events[0].cleanupConfirmed,false)
  assert.equal(events[1].cleanupConfirmed,false)
  assert.equal(events[2].outcome,'completed')
  assert.equal(events[3].cleanupConfirmed,true)
  const before=readFileSync(f.env.FIXTURE_CALLS,'utf8')
  const recovered=await recoverMachine({protocol:1,requestId:receipt.requestId,runId:receipt.runId,recoveryDirectory:f.runtime.recoveryDirectory})
  assert.deepEqual(recovered.receipt,receipt);assert.equal(readFileSync(f.env.FIXTURE_CALLS,'utf8'),before)
  const resumed=await runMachine({...f.request,requestId:'task-2',sessionMode:'resume',sessionId:receipt.session.id},{env:f.env})
  assert.equal(resumed.ok,true,JSON.stringify(resumed.failure));assert.equal(resumed.session.id,receipt.session.id)
  const wrong=await runMachine({...f.request,requestId:'task-3',sessionMode:'resume',sessionId:receipt.session.id,model:'gemini-4-neon-high'},{env:f.env})
  assert.equal(wrong.execution.dispatch,'not-dispatched');assert.match(wrong.failure.detail,/identical/)
})
test('validation rejects unsupported inputs before any CLI call',()=>{
  const f=fixture()
  for(const change of [{effort:'low'},{images:['x']},{permissionMode:'auto'},{sessionMode:'resume',sessionId:'latest'},{subscriptionPolicy:{version:1,mode:'api'}},{toolPolicy:'none'}])assert.throws(()=>parseMachineRequest({...f.request,...change}))
  assert.equal(existsSync(f.env.FIXTURE_CALLS),false)
})
test('subscription conflicts and settings drift never become successful',async()=>{
  for(const settings of [{useG1Credits:true},{modelProvider:'gemini'},{gcp:{project:'billing'}},{toolPermission:'always-proceed'}]){
    const f=fixture();writeFileSync(join(f.home,'.gemini/antigravity-cli/settings.json'),JSON.stringify(settings))
    const r=await runMachine(f.request,f.runtime);assert.equal(r.ok,false);assert.equal(r.execution.dispatch,'not-dispatched');assert.equal(existsSync(f.env.FIXTURE_INPUT),false)
  }
  const f=fixture('drift'),r=await runMachine(f.request,f.runtime);assert.equal(r.ok,false);assert.match(r.failure.detail,/credits|settings/i)
})
test('protocol failure and soft denials cannot hide behind exit zero',async()=>{
  for(const scenario of ['wrong-model','truncated','duplicate','denied','waiting','exit-fail','quota','early-error']){
    const f=fixture(scenario),r=await runMachine(f.request,f.runtime)
    assert.equal(r.ok,false,scenario);assert.ok(r.failure,scenario)
    if(scenario==='quota')assert.equal(r.failure.category,'quota')
    if(scenario==='early-error')assert.equal(r.failure.category,'auth')
    if(scenario==='waiting'||scenario==='truncated')assert.equal(r.providerWork.state,'unknown')
  }
})
test('structured output validates locally and long progress is not retained result',async()=>{
  const schema={type:'object',properties:{answer:{type:'string'}},required:['answer'],additionalProperties:false}
  for(const scenario of ['success','schema-bad']){
    const f=fixture(scenario),r=await runMachine({...f.request,outputSchema:schema},f.runtime);assert.equal(r.ok,scenario==='success',JSON.stringify(r.failure))
  }
  const f=fixture('long-stream');let progress=0
  const r=await runMachine({...f.request,maxOutputBytes:16},{...f.runtime,onProgress:text=>{progress+=text.length}})
  assert.equal(r.ok,true,JSON.stringify(r.failure));assert.ok(progress>1000000);assert.equal(r.output[0].text,'done')
})
test('evidence-only verifies empty tools before dispatch and cleans only its agent',async()=>{
  for(const scenario of ['success','tools-inherited','missing-agent','wrong-agent','no-init','illegal-tool']){
    const f=fixture(scenario),r=await runMachine({...f.request,toolPolicy:'evidence-only',timeoutMs:scenario==='no-init'?1000:10000},f.runtime)
    assert.equal(r.ok,scenario==='success',scenario+': '+JSON.stringify(r.failure))
    if(['tools-inherited','missing-agent','wrong-agent','no-init'].includes(scenario)){assert.equal(existsSync(f.env.FIXTURE_INPUT),false);assert.equal(r.execution.dispatch,'not-dispatched')}
    assert.equal(existsSync(join(f.cwd,'.agents/agents')),false)
    if(scenario==='success')assert.deepEqual(r.configuration.observed.tools,[])
  }
})
test('doctor is inference-free, uses the same filtered environment and does not report quota',async()=>{
  const f=fixture(),d=await doctor({command,cwd:f.cwd,model:f.request.model,effort:'high'},{env:{...f.env,GEMINI_API_KEY:'not-passed',NODE_OPTIONS:'--bad-option'}})
  assert.equal(d.ok,true,JSON.stringify(d));assert.equal(d.remainingQuota,'unknown');assert.equal(existsSync(f.env.FIXTURE_INPUT),false)
  assert.equal(readFileSync(f.env.FIXTURE_CALLS,'utf8').trim().split('\n').length,3)
})
test('cancel and total timeout preserve unknown provider work and local cleanup',async()=>{
  for(const kind of ['abort','deadline']){
    const f=fixture('hang'),controller=new AbortController()
    const r=await runMachine({...f.request,timeoutMs:kind==='deadline'?900:10000},{...f.runtime,signal:controller.signal,onActivity:()=>{if(kind==='abort')controller.abort()}})
    assert.equal(r.ok,false);assert.equal(r.cleanup.confirmed,true,JSON.stringify(r));assert.equal(r.execution.cancelled,kind==='abort');assert.equal(r.execution.timedOut,kind==='deadline')
    if(r.execution.dispatch==='dispatched'){assert.equal(r.providerWork.state,'unknown');assert.equal(r.providerWork.replaySafe,false)}
  }
})
