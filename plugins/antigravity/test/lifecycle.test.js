import test from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdtempSync, mkdirSync, existsSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { runMachine, recoverMachine } from '../lib/machine.js'
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms))
async function until(check){for(let i=0;i<300;i++){if(check())return;await delay(20)}throw new Error('timed out waiting for lifecycle evidence')}
function fixture(scenario='hang'){
  const home=mkdtempSync(join(tmpdir(),'agy-life-')),cwd=join(home,'work'),root=join(home,'journal');mkdirSync(cwd);mkdirSync(root,{mode:0o700})
  const env={PATH:process.env.PATH,HOME:home,FIXTURE_SCENARIO:scenario,FIXTURE_INPUT:join(home,'input'),FIXTURE_PID:join(home,'pid'),FIXTURE_CHILD_PID:join(home,'child')}
  const request={protocol:1,requestId:'lifecycle',cwd,command:resolve('test/fixtures/fake-agy.js'),prompt:'complete task',model:'gemini-4-hydrogen-high',effort:'high',executionMode:'default',permissionMode:'request-review',sandbox:false,sessionMode:'new',timeoutMs:10000,disposeGraceMs:50,subscriptionPolicy:{version:1,mode:'subscription-only'}}
  return {home,request,runtime:{env,recoveryDirectory:join(root,'run')},env}
}
function alive(pid){try{const text=readFileSync('/proc/'+pid+'/stat','utf8');return text.split(') ')[1][0]!=='Z'}catch(error){if(error.code==='ENOENT')return false;try{process.kill(pid,0);return true}catch{return false}}}
test('parent death cancels the owned CLI, writes unknown receipt, and read-only recovery never replays',async()=>{
  const f=fixture();f.request.toolPolicy='evidence-only';const input=join(f.home,'input.json');writeFileSync(input,JSON.stringify({request:f.request,runtime:f.runtime}),{mode:0o600})
  const parent=spawn(process.execPath,[resolve('test/fixtures/owner.js'),input],{stdio:'ignore'});const closed=new Promise(resolve=>parent.once('close',resolve))
  await until(()=>existsSync(f.env.FIXTURE_INPUT));const pid=Number(readFileSync(f.env.FIXTURE_PID,'utf8'))
  parent.kill('SIGKILL');await closed
  await until(()=>existsSync(join(f.runtime.recoveryDirectory,'receipt.json')))
  const id=JSON.parse(readFileSync(join(f.runtime.recoveryDirectory,'identity.json'),'utf8'))
  const recovery=await recoverMachine({protocol:1,requestId:id.requestId,runId:id.runId,recoveryDirectory:f.runtime.recoveryDirectory})
  assert.equal(recovery.receipt.ok,false);assert.equal(recovery.receipt.execution.cancelled,true);assert.equal(recovery.receipt.providerWork.state,'unknown');assert.equal(alive(pid),false)
  assert.equal(existsSync(join(f.request.cwd,'.agents/agents')),false)
})
test('concurrent resume and unresolved work cannot start another native turn',async()=>{
  const f=fixture('success'),first=await runMachine(f.request,f.runtime);assert.equal(first.ok,true)
  f.env.FIXTURE_SCENARIO='hang';const controller=new AbortController(),resume={...f.request,sessionMode:'resume',sessionId:first.session.id}
  let initialized
  const ready=new Promise(resolve=>{initialized=resolve})
  const pending=runMachine(resume,{env:f.env,signal:controller.signal,onActivity:e=>{if(e.eventTypes.includes('init'))initialized()}})
  await ready
  const second=await runMachine({...resume,requestId:'parallel'},{env:f.env});assert.equal(second.execution.dispatch,'not-dispatched');assert.match(second.failure.detail,/active or unresolved/)
  controller.abort();const interrupted=await pending;assert.equal(interrupted.providerWork.state,'unknown')
  const third=await runMachine({...resume,requestId:'after-unknown'},{env:f.env});assert.equal(third.execution.dispatch,'not-dispatched');assert.match(third.failure.detail,/active or unresolved/)
})
test('lifecycle acknowledgement rejection blocks stdin and structured identity recovery detects tampering',async()=>{
  const f=fixture('success'),r=await runMachine(f.request,{...f.runtime,onLifecycle:e=>{if(e.event==='dispatch-possible')throw new Error('caller journal rejected')}})
  assert.equal(r.ok,false);assert.equal(r.execution.dispatch,'not-dispatched');assert.equal(existsSync(f.env.FIXTURE_INPUT),false)
  const recovery=await recoverMachine({protocol:1,requestId:'different',runId:r.runId,recoveryDirectory:f.runtime.recoveryDirectory});assert.equal(recovery.status,'unknown');assert.equal(recovery.receipt,null)
})
test('timeout terminates an owned stubborn descendant without claiming unknown native work completed',async()=>{
  const f=fixture('grandchild'),r=await runMachine({...f.request,timeoutMs:1500},f.runtime)
  assert.equal(r.ok,false);assert.equal(r.providerWork.state,'unknown');assert.equal(r.execution.timedOut,true)
  assert.equal(alive(Number(readFileSync(f.env.FIXTURE_PID,'utf8'))),false);assert.equal(alive(Number(readFileSync(f.env.FIXTURE_CHILD_PID,'utf8'))),false)
  // A non-reaping container init may retain a zombie; cleanup must then stay unconfirmed.
  if(!r.cleanup.confirmed)assert.equal(r.session.resumable,false)
})
