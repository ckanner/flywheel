import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import * as api from '../../antigravity/lib/machine.js';
import { ADAPTER, doctorResult, invokeAdapter, machineRequest } from '../src/antigravity-bridge.js';
import { validProviderActivity } from '../src/machine-worker.js';

async function request(t) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-agy-bridge-')));
  t.after(() => rm(cwd, {recursive:true,force:true}));
  return {type:'start',protocol:1,requestId:'native-request',cwd,model:'gemini-4-neon-high',reasoningEffort:'high',permissionMode:'danger-full-access',prompt:'Approved work',subscription:{version:1,mode:'subscription-only'}};
}

test('Antigravity bridge forwards the complete native request and keeps host attribution separate',async t=>{
  const base=await request(t);
  for(const mode of ['native','subagents','swarm']) {
    const native={version:1,mode,maxObservedAgents:4};
    const managed={version:1,runId:'run',invocationId:base.requestId,role:'execute',scope:'M01-T01',channel:'google',taskDir:base.cwd};
    const parsed=machineRequest({...base,native,managed,model:'gemini-pro-orchestrator-neon',reasoningEffort:''},api,{});
    assert.deepEqual(parsed.native,native);
    assert.equal(parsed.effort,'');
    assert.equal(Object.hasOwn(parsed,'managed'),false);
  }
  assert.equal(Object.hasOwn(machineRequest(base,api,{}),'native'),false);
  const doctor=doctorResult();
  assert.equal(doctor.version,ADAPTER.version);
  assert.equal(doctor.version,'0.2.0');
  assert.deepEqual(doctor.nativeModes,['native','subagents','swarm']);
  assert.equal(doctor.hardNativeAgentLimit,false);
  assert.equal(doctor.inferenceTested,false);
});

test('Antigravity bridge rejects collaboration authority drift and evidence-only delegation',async t=>{
  const base=await request(t);
  const managed={version:1,runId:'run',invocationId:base.requestId,role:'plan',scope:'plan',channel:'google',taskDir:base.cwd};
  for(const fields of [
    {native:{version:1,mode:'swarm'},managed},
    {native:{version:1,mode:'native'},evidenceOnly:true,permissionMode:'read-only'},
    {model:'gemini-pro-orchestrator',reasoningEffort:'',evidenceOnly:true,permissionMode:'read-only'},
    {native:{version:1,mode:'swarm',maxObservedAgents:9}},
    {native:{version:1,mode:'swarm'},managed:{...managed,invocationId:'other'}},
  ]) assert.throws(()=>machineRequest({...base,...fields},api,{}),{code:'INVALID_REQUEST'});
});

test('Antigravity child activity projects liveness while retaining native evidence only in the receipt', async () => {
  const activity = {version:1,sequence:2,kind:'provider-progress',source:'antigravity',scope:'child',eventTypes:['step'],observedEvents:1};
  const native = {children:[{id:'child',role:'PRIVATE_CHILD_METADATA'}],completion:{children:'unknown'}};
  const event = {...activity,native};
  const receipt = {ok:false,native,failure:{category:'observed-agent-limit'}};
  const observed=[];
  const result = await invokeAdapter({async runMachine(request,options) {
    options.onActivity(event);
    options.onActivity({...event,content:'UNEXPECTED_PRIVATE_CONTENT'});
    options.onActivity({...event,sequence:1});
    options.onActivity(null);
    return receipt;
  }}, {}, {onActivity:value=>observed.push(value)});
  assert.deepEqual(observed[0],activity);
  assert.equal(validProviderActivity(observed[0],'antigravity',1),true);
  assert.equal(JSON.stringify(observed[0]).includes('PRIVATE_CHILD_METADATA'),false);
  assert.equal(validProviderActivity(observed[1],'antigravity',1),false,'unknown fields remain invalid even with a newer sequence');
  assert.equal(validProviderActivity(observed[2],'antigravity',2),false,'stale activity remains invalid');
  assert.equal(validProviderActivity(observed[3],'antigravity',2),false,'malformed activity remains invalid');
  assert.equal(event.native,native,'projection cannot mutate the adapter observation');
  assert.equal(result,receipt,'projection cannot replace the native failure receipt');
  assert.equal(result.native,native);
});
