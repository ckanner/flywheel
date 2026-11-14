import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { chmod, copyFile, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require=createRequire(import.meta.url);
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));

async function setup(t,harness,scenario='ok'){
  const cwd=await realpath(await mkdtemp(join(tmpdir(),'flywheel native lifecycle ')));
  const claude=harness==='claude-code-pro';
  const native=join(cwd,'native-fixture.mjs');
  await copyFile(new URL('./fixtures/fake-'+harness+'.js',import.meta.url),native);
  await chmod(native,0o755);
  await writeFile(join(cwd,'claude-fixture.json'),JSON.stringify({scenario}));
  const env={...process.env,CODEX_HOME:join(cwd,'isolated-codex'),CLAUDE_CONFIG_DIR:join(cwd,'isolated-claude'),
    FLYWHEEL_CODEX_BIN:native,FLYWHEEL_CLAUDE_BIN:native,TEST_CODEX_PRO_SCENARIO:scenario,
    TEST_CODEX_PRO_LOG:join(cwd,'rpc.jsonl'),TEST_CODEX_PRO_HISTORY:join(cwd,'codex-history.json')};
  for(const key of ['NODE_OPTIONS','NODE_PATH','ANTHROPIC_API_KEY','ANTHROPIC_AUTH_TOKEN','CLAUDE_CODE_OAUTH_TOKEN','ANTHROPIC_BASE_URL'])delete env[key];
  const original={cwd,model:'fixture-model',reasoningEffort:'high',permissionMode:claude?'danger-full-access':'workspace-write',threadMode:'persistent'};
  const request={...original,type:'start',protocol:3,requestId:'lifecycle-attempt-1',prompt:'offline lifecycle task',timeoutMs:5000,recoveryDirectory:join(cwd,'run-1')};
  const runs=[];
  const launch=(input=request,{hold,killAt,environment=env}={})=>{
    const child=spawn(process.execPath,[fileURLToPath(new URL('../dist/'+harness+'-worker.js',import.meta.url))],{cwd,env:environment,stdio:['pipe','pipe','pipe'],detached:true});
    const run={child,frames:[],stderr:'',closed:false,buffer:''};
    run.ack=e=>child.stdin.write(JSON.stringify({type:'ack',protocol:3,requestId:e.requestId,runId:e.runId,sequence:e.sequence})+'\n');
    child.stdout.setEncoding('utf8');child.stderr.setEncoding('utf8');
    child.stdout.on('data',text=>{
      run.buffer+=text;let n;
      while((n=run.buffer.indexOf('\n'))>=0){
        const f=JSON.parse(run.buffer.slice(0,n));run.buffer=run.buffer.slice(n+1);run.frames.push(f);
        if(f.type==='lifecycle'){
          if(f.lifecycle.event===killAt){process.kill(-child.pid,'SIGKILL');}
          else if(f.lifecycle.event!==hold)run.ack(f.lifecycle);
        }
      }
    });
    child.stderr.on('data',text=>run.stderr+=text);
    child.stdin.on('error',()=>{});
    run.done=new Promise((resolve,reject)=>{
      child.once('error',reject);
      child.once('close',(code,signal)=>{run.closed=true;resolve({code,signal});});
    });
    run.wait=async event=>{
      const deadline=Date.now()+8000;
      while(!run.frames.some(f=>f.type==='lifecycle'&&f.lifecycle.event===event)){
        assert.ok(!run.closed&&Date.now()<deadline,run.stderr+JSON.stringify(run.frames));await delay(15);
      }
      return run.frames.find(f=>f.type==='lifecycle'&&f.lifecycle.event===event).lifecycle;
    };
    child.stdin.write(JSON.stringify(input)+'\n');runs.push(run);return run;
  };
  const records=async()=>JSON.parse('['+(await readFile(join(cwd,claude?'claude-fixture.jsonl':'rpc.jsonl'),'utf8').catch(()=>'' )).trim().split('\n').filter(Boolean).join(',')+']');
  const turns=async()=>{const rows=await records();return claude?rows.length:rows.filter(r=>r.method==='turn/start').length;};
  const recover=async runId=>{
    const run=launch({type:'recover',protocol:3,requestId:request.requestId,runId,recoveryDirectory:request.recoveryDirectory,original});
    assert.equal((await run.done).code,0,run.stderr+JSON.stringify(run.frames));
    assert.equal(run.frames.length,1);assert.equal(run.frames[0].type,'recovery');assert.equal(run.frames[0].error,null);
    return run.frames[0].recovery;
  };
  t.after(async()=>{
    for(const run of runs){
      if(!run.closed)run.child.kill('SIGTERM');
      const timer=setTimeout(()=>{if(!run.closed)run.child.kill('SIGKILL');},10000);
      await run.done;clearTimeout(timer);
    }
    // A killed worker's independent supervisor completes its bounded cleanup
    // before tests remove its evidence directory.
    if(scenario==='hang'||scenario==='stubborn')await delay(100);
    await rm(cwd,{recursive:true,force:true});
  });
  return {cwd,request,original,launch,records,turns,recover};
}

for(const harness of ['codex-pro','claude-code-pro']){
  let installed=true;try{require.resolve('dsh-subagent-'+harness+'/package.json');}catch(error){
    if(error.code!=='MODULE_NOT_FOUND')throw error;
    installed=false;
  }
  const opts={skip:installed?false:'install the exact public source tarball to verify '+harness,timeout:20000};
  test(harness+': native turn is gated by the possible-dispatch ACK and has one durable original receipt',opts,async t=>{
    const f=await setup(t,harness);
    const run=f.launch(f.request,{hold:'dispatch-possible'});
    const event=await run.wait('dispatch-possible');
    assert.equal(await f.turns(),0);
    await delay(40);assert.equal(await f.turns(),0);
    const pending=await f.recover(event.runId);
    assert.equal(pending.status,'pending');assert.equal(pending.receipt,null);
    assert.equal(await f.turns(),0,'read-only recovery must not dispatch');
    run.ack(event);
    assert.equal((await run.done).code,0,run.stderr+JSON.stringify(run.frames));
    const result=run.frames.at(-1);assert.equal(result.receipt.ok,true);
    const saved=await f.recover(event.runId);
    assert.equal(saved.status,'terminal');
    assert.deepEqual(saved.receipt,result.receipt);
    const again=await f.recover(event.runId);
    assert.deepEqual(again,saved);
    assert.equal(await f.turns(),1);
    const names=run.frames.filter(x=>x.type==='lifecycle').map(x=>x.lifecycle.event);
    assert.equal(names.includes('thread-ready'),harness==='codex-pro');
    assert.ok(names.includes('cleanup-confirmed'));
    assert.equal((await f.recover('unrelated-run')).status,'unknown');
    assert.equal(await f.turns(),1);
  });

  test(harness+': hard-killed worker after cleanup notification recovers the unchanged terminal result',opts,async t=>{
    const f=await setup(t,harness);
    const run=f.launch(f.request,{killAt:'cleanup-confirmed'});
    const evidence=await run.wait('cleanup-confirmed');
    assert.equal((await run.done).signal,'SIGKILL');
    assert.equal(run.frames.some(x=>x.type==='result'),false);
    const recovered=await f.recover(evidence.runId);
    assert.equal(recovered.status,'terminal');
    assert.equal(recovered.receipt.ok,true);
    assert.equal(await f.turns(),1);
    assert.equal((await f.recover(evidence.runId)).receipt.runId,evidence.runId);
    assert.equal(await f.turns(),1);
  });

  test(harness+': hard-killed worker during native work leaves a supervised receipt without replay',opts,async t=>{
    const f=await setup(t,harness,'hang');
    const run=f.launch();
    const early=await run.wait('dispatch-possible');
    const deadline=Date.now()+5000;
    while(await f.turns()===0){assert.ok(Date.now()<deadline);await delay(20);}
    process.kill(-run.child.pid,'SIGKILL');
    assert.equal((await run.done).signal,'SIGKILL');
    let result;
    const recoveryDeadline=Date.now()+10000;
    do{
      result=await f.recover(early.runId);
      if(result.status==='terminal')break;
      assert.equal(result.status,'pending');assert.ok(Date.now()<recoveryDeadline,'supervised terminal receipt never became durable');
      await delay(30);
    }while(true);
    assert.equal(result.receipt.ok,false);
    const facts=harness==='codex-pro'?(result.receipt.outcome??result.receipt.failure):result.receipt;
    assert.equal(facts.cleanup.confirmed,true);
    assert.equal(await f.turns(),1);
  });

  if (harness === 'claude-code-pro') test('independent cloud review uses native command and read-only original recovery through worker v3', opts, async t => {
    const f = await setup(t, harness);
    f.original.threadMode = f.request.threadMode = 'ephemeral';
    f.original.nativeOperation = f.request.nativeOperation = { kind: 'remote-review', target: 'main', consent: true, timeoutMinutes: 2 };
    const run = f.launch();
    assert.equal((await run.done).code, 0, run.stderr + JSON.stringify(run.frames));
    const receipt = run.frames.at(-1).receipt;
    assert.equal(receipt.operation, 'native-review');
    assert.equal(receipt.remote.state, 'completed');
    assert.equal(receipt.remote.cancelSupported, false);
    assert.deepEqual(receipt.structured, { bugs: [{ title: 'offline finding' }] });
    assert.deepEqual((await f.records())[0].argv, ['ultrareview', 'main', '--json', '--no-post', '--timeout', '2']);
    const recovered = await f.recover(receipt.runId);
    assert.equal(recovered.status, 'terminal');
    assert.deepEqual(recovered.receipt, receipt);
    assert.equal((await f.records()).length, 1);
    assert.equal(run.frames.some(frame => frame.lifecycle?.event === 'thread-ready'), false);
  });
}
