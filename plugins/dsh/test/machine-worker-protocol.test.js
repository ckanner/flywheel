import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { recoveryRequest } from '../src/machine-worker.js';

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const transport = new URL('../src/machine-worker.js', import.meta.url).href;

async function fixture(t, { scenario = 'normal', ackMs = 1500, firstInput } = {}) {
  const cwd = await mkdtemp(join(tmpdir(), 'flywheel lifecycle transport '));
  const script = join(cwd, 'worker.mjs');
  await writeFile(script, [
    'import {runMachineWorker} from '+JSON.stringify(transport)+';',
    'import {appendFile} from "node:fs/promises";',
    'class WorkerError extends Error {constructor(code,message){super(message);this.code=code;}}',
    'const scenario='+JSON.stringify(scenario)+';',
    'const api={MACHINE_LIFECYCLE_VERSION:1,MACHINE_LIFECYCLE_CAPABILITIES:{durableJournal:true,readOnlyRecovery:true},',
    'async recoverMachine(q){await appendFile("queries.txt",q.runId+"\\n");return {protocol:1,type:"recovery",status:"unknown",requestId:q.requestId,runId:q.runId,reason:"fixture",receipt:null,lifecycle:null};},',
    'async runMachine(request,options){let phase="before-dispatch";try {',
    'for(const [i,name] of ["dispatch-intent","dispatch-possible","turn-accepted","terminal","cleanup-confirmed"].entries()){',
    'const event={lifecycleVersion:1,type:"lifecycle",requestId:request.requestId,runId:scenario==="wrong-run"&&i===1?"other-run":"run-a",sequence:scenario==="out-of-order"&&i===1?9:i+1,event:name,threadId:null,turnId:null,persistent:false,configuration:null,dispatch:i<2?"not-dispatched":"accepted",outcome:i<3?"unknown":"completed",cleanupConfirmed:i===4};',
    'if(scenario==="oversize-event")event.extra="x".repeat(2097152);',
    'await options.onLifecycle(event);',
    'if(i===1){await appendFile("dispatch.txt","one turn\\n");phase="dispatched"; if(scenario.startsWith("native-"))options.onNativeEvent({version:scenario==="native-invalid"?2:1,sequence:1,kind:"child.created",id:"child-a",parentId:"root-a",state:scenario==="native-overflow"?"x".repeat(2097152):"running"});}',
    '}return {ok:true,phase};}catch(error){return {ok:false,phase,failure:{code:error.code,message:error.message}};}}};',
    'await runMachineWorker({ADAPTER:{package:"transport-fixture",version:"1.0.0",protocol:1},LIMITS:{requestBytes:1048576,receiptFrameBytes:8388608,progressBytes:1048576,admissionMs:500,lifecycleAckMs:'+ackMs+'},WorkerError,loadAdapter:async()=>api,machineRequest:x=>x,doctorResult:()=>({}),label:"Test",commandName:"worker",attestAdmission:true});',
  ].join('\n'));
  const child = spawn(process.execPath, [script], { cwd, stdio: ['pipe', 'pipe', 'pipe'] });
  const run = { child, cwd, frames: [], stderr: '', closed: false };
  let buffer = '';
  child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
  child.stdout.on('data', text => {
    buffer += text;
    for (;;) {
      const nl = buffer.indexOf('\n');
      if (nl < 0) break;
      const frame = JSON.parse(buffer.slice(0, nl)); buffer = buffer.slice(nl + 1);
      run.frames.push(frame);
      run.onFrame?.(frame);
    }
  });
  child.stderr.on('data', text => { run.stderr += text; });
  child.stdin.on('error', () => {});
  run.done = new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('close', (code, signal) => { run.closed = true; resolve({ code, signal }); });
  });
  run.ack = (event, overrides = {}) => child.stdin.write(JSON.stringify({ type: 'ack', protocol: 3,
    requestId: event.requestId, runId: event.runId, sequence: event.sequence, ...overrides })+'\n');
  run.wait = async predicate => {
    const deadline = Date.now()+5000;
    while (!predicate()) { assert.ok(!run.closed && Date.now()<deadline, run.stderr+JSON.stringify(run.frames)); await delay(10); }
  };
  run.exists = async name => readFile(join(cwd,name),'utf8').catch(error => { if(error.code==='ENOENT')return ''; throw error; });
  t.after(async()=>{
    if (!run.closed) child.kill('SIGTERM');
    const timer = setTimeout(()=>child.kill('SIGKILL'),5000);
    await run.done; clearTimeout(timer);
    await rm(cwd,{recursive:true,force:true});
  });
  const request = { type:'start',protocol:3,requestId:'request-a',cwd,prompt:'one task',model:'fixture-model',
    reasoningEffort:'high',permissionMode:'workspace-write',threadMode:'ephemeral',recoveryDirectory:join(cwd,'evidence') };
  child.stdin.write(firstInput ?? JSON.stringify(request)+'\n');
  return run;
}

test('each dispatch checkpoint waits for its own correlated ACK and allows exactly one turn', async t=>{
  const run=await fixture(t);
  await run.wait(()=>run.frames.length===1);
  assert.equal(run.frames[0].lifecycle.event,'dispatch-intent');
  assert.equal(await run.exists('dispatch.txt'),'');
  await delay(35);
  assert.equal(run.frames.length,1);
  run.ack(run.frames[0].lifecycle);
  await run.wait(()=>run.frames.length===2);
  assert.equal(run.frames[1].lifecycle.event,'dispatch-possible');
  assert.equal(await run.exists('dispatch.txt'),'');
  run.onFrame=frame=>{if(frame.type==='lifecycle')run.ack(frame.lifecycle);};
  run.ack(run.frames[1].lifecycle);
  assert.deepEqual(await run.done,{code:0,signal:null});
  assert.equal(await run.exists('dispatch.txt'),'one turn\n');
  assert.deepEqual(run.frames.filter(f=>f.type==='lifecycle').map(f=>f.lifecycle.sequence),[1,2,3,4,5]);
  assert.equal(run.frames.at(-1).receipt.ok,true);
});

for(const overrides of [{requestId:'other-request'},{runId:'other-run'},{sequence:99},{extra:true},{type:'start'}]){
  test('a mismatched or unsolicited ACK cannot cross dispatch: '+JSON.stringify(overrides),async t=>{
    const run=await fixture(t);
    await run.wait(()=>run.frames.length===1);
    run.ack(run.frames[0].lifecycle,overrides);
    assert.equal((await run.done).code,1);
    assert.equal(await run.exists('dispatch.txt'),'');
    assert.equal(run.frames.at(-1).error.code,'INVALID_ACK');
    assert.equal(run.frames.at(-1).admission,undefined);
  });
}

test('EOF while waiting for durable acknowledgment cancels without dispatch',async t=>{
  const run=await fixture(t);
  await run.wait(()=>run.frames.length===1);
  run.child.stdin.end();
  assert.equal((await run.done).code,1);
  assert.equal(await run.exists('dispatch.txt'),'');
  assert.equal(run.frames.at(-1).error.code,'PARENT_DISCONNECTED');
});

test('a guessed ACK queued in the same chunk as start cannot authorize dispatch',async t=>{
  const start={type:'start',protocol:3,requestId:'request-a'};
  const guess={type:'ack',protocol:3,requestId:'request-a',runId:'run-a',sequence:1};
  const run=await fixture(t,{firstInput:JSON.stringify(start)+'\n'+JSON.stringify(guess)+'\n'});
  assert.equal((await run.done).code,1);
  assert.equal(await run.exists('dispatch.txt'),'');
  assert.equal(run.frames.some(f=>f.type==='lifecycle'),false);
  assert.equal(run.frames.at(-1).error.code,'INVALID_ACK');
  assert.deepEqual(run.frames.at(-1).admission,{dispatched:false,cleanupConfirmed:true});
});

for(const sequence of [1,2]){
  test('a duplicate or speculative second ACK cannot authorize the next checkpoint: '+sequence,async t=>{
    const run=await fixture(t);
    await run.wait(()=>run.frames.length===1);
    const first={type:'ack',protocol:3,requestId:'request-a',runId:'run-a',sequence:1};
    run.child.stdin.write(JSON.stringify(first)+'\n'+JSON.stringify({...first,sequence})+'\n');
    assert.equal((await run.done).code,1);
    assert.equal(await run.exists('dispatch.txt'),'');
    assert.equal(run.frames.filter(f=>f.type==='lifecycle').length,1);
    assert.equal(run.frames.at(-1).error.code,'INVALID_ACK');
    assert.equal(run.frames.at(-1).admission,undefined);
  });
}

for(const [signal,code] of [['SIGTERM',143],['SIGINT',130],['SIGHUP',129]]){
  test(signal+' while an ACK is pending settles the waiter without native dispatch',async t=>{
    const run=await fixture(t);
    await run.wait(()=>run.frames.length===1);
    run.child.kill(signal);
    assert.deepEqual(await run.done,{code,signal:null});
    assert.equal(await run.exists('dispatch.txt'),'');
    assert.equal(run.frames.at(-1).receipt.ok,false);
    assert.equal(run.frames.at(-1).receipt.failure.code,'CANCELLED');
  });
}

test('the ACK deadline stops an abandoned checkpoint without dispatch',async t=>{
  const run=await fixture(t,{ackMs:50});
  assert.equal((await run.done).code,1);
  assert.equal(await run.exists('dispatch.txt'),'');
  assert.equal(run.frames.at(-1).error.code,'ACK_TIMEOUT');
});

for(const scenario of ['wrong-run','out-of-order','oversize-event']){
  test('invalid native lifecycle evidence is not forwarded as a new checkpoint: '+scenario,async t=>{
    const run=await fixture(t,{scenario});
    run.onFrame=frame=>{if(frame.type==='lifecycle')run.ack(frame.lifecycle);};
    assert.equal((await run.done).code,1);
    assert.equal(await run.exists('dispatch.txt'),'');
    assert.ok(run.frames.filter(f=>f.type==='lifecycle').length<=1);
    assert.ok(['INVALID_LIFECYCLE','OUTPUT_LIMIT'].includes(run.frames.at(-1).receipt.failure.code));
  });
}

test('an overlong ACK frame cancels instead of accumulating unbounded input',async t=>{
  const run=await fixture(t);
  await run.wait(()=>run.frames.length===1);
  run.child.stdin.write(' '.repeat(4097));
  assert.equal((await run.done).code,1);
  assert.equal(run.frames.at(-1).error.code,'INPUT_LIMIT');
  assert.equal(await run.exists('dispatch.txt'),'');
});

test('a read-only query only calls recoverMachine, including when the old cwd is gone',async t=>{
  const query={type:'recover',protocol:3,requestId:'request-a',runId:'run-a',recoveryDirectory:'/precise/recovery',
    original:{cwd:'/a/worktree/that/no/longer/exists',model:'fixture-model',reasoningEffort:'high',permissionMode:'workspace-write',threadMode:'persistent'}};
  const run=await fixture(t,{firstInput:JSON.stringify(query)+'\n'});
  assert.equal((await run.done).code,0);
  assert.equal(await run.exists('queries.txt'),'run-a\n');
  assert.equal(await run.exists('dispatch.txt'),'');
  assert.equal(run.frames.length,1);
  assert.equal(run.frames[0].type,'recovery');
  assert.equal(run.frames[0].recovery.status,'unknown');
});

for (const scenario of ['native-valid', 'native-invalid', 'native-overflow']) {
  test('native metadata is bounded, separately framed and never a lifecycle checkpoint: ' + scenario, async t => {
    const run = await fixture(t, { scenario });
    run.onFrame = frame => { if (frame.type === 'lifecycle') run.ack(frame.lifecycle); };
    assert.equal((await run.done).code, scenario === 'native-valid' ? 0 : 1);
    const events = run.frames.filter(frame => frame.type === 'native-event');
    assert.equal(events.length, scenario === 'native-valid' ? 1 : 0);
    if (scenario === 'native-valid') {
      assert.equal(events[0].native.id, 'child-a');
      assert.equal(run.frames.at(-1).receipt.ok, true);
    } else assert.equal(run.frames.at(-1).error.code, scenario === 'native-invalid' ? 'INVALID_NATIVE_EVENT' : 'OUTPUT_LIMIT');
  });
}

test('recovery rejects prompts, credentials, unknown fields and invalid history without a model call',()=>{
  class WorkerError extends Error{constructor(code,message){super(message);this.code=code;}}
  const value={type:'recover',protocol:3,requestId:'request',runId:'run',recoveryDirectory:'/evidence',
    original:{cwd:'/old-worktree',model:'m',reasoningEffort:'high',permissionMode:'read-only',threadMode:'ephemeral'}};
  assert.deepEqual(recoveryRequest(value,WorkerError),{protocol:1,requestId:'request',runId:'run',recoveryDirectory:'/evidence'});
  for(const changed of [{...value,prompt:'never'},{...value,env:{TOKEN:'never'}},{...value,recoveryDirectory:'relative'},
    {...value,original:{...value.original,prompt:'never'}},{...value,original:{...value.original,threadId:'not-persistent'}},
    {...value,runId:''},{...value,original:{...value.original,threadMode:'guess-latest'}}]){
    assert.throws(()=>recoveryRequest(changed,WorkerError),{code:'INVALID_REQUEST'});
  }
});
