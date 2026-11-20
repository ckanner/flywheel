import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtempSync,mkdirSync,writeFileSync,copyFileSync,chmodSync,readFileSync,realpathSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join,resolve } from 'node:path'
import { parsePackOutput } from './pack-output.js'
const root=resolve('.'),temp=realpathSync(mkdtempSync(join(tmpdir(),'antigravity-package-'))),installed=join(temp,'installed'),home=join(temp,'home')
mkdirSync(installed);mkdirSync(home,{mode:0o700})
function run(command,args,options={}){const r=spawnSync(command,args,{encoding:'utf8',cwd:root,timeout:60000,maxBuffer:8*1024*1024,...options});assert.equal(r.status,0,(r.error?.message??'')+'\n'+r.stderr+'\n'+r.stdout);return r.stdout}
const manifest=JSON.parse(readFileSync(join(root,'package.json'),'utf8'))
const packed=parsePackOutput(run('npm',['pack','--ignore-scripts','--json','--pack-destination',temp]),manifest.name,manifest.version),tarball=join(temp,packed.filename)
for(const file of ['lib/machine.js','lib/machine.d.ts','lib/worker.js','lib/native.js','lib/evidence-agent.js','lib/index.js','assets/client.js','bin/antigravity-machine.js','cordis.patch.yml','README.md','README.zh-CN.md','docs/MACHINE.md','docs/MACHINE.zh-CN.md'])assert.ok(packed.files.some(f=>f.path===file),'missing packed '+file)
writeFileSync(join(installed,'package.json'),JSON.stringify({type:'module',private:true}))
run('npm',['install','--ignore-scripts','--no-audit','--no-fund','--omit=dev','--prefix',installed,tarball])
const fake=join(temp,'fake-agy.mjs');copyFileSync(join(root,'test/fixtures/fake-agy.js'),fake);chmodSync(fake,0o755)
const request={protocol:1,requestId:'packed',command:fake,cwd:installed,prompt:'check packed worker',model:'gemini-4-hydrogen-high',effort:'high',executionMode:'default',permissionMode:'request-review',sandbox:false,toolPolicy:'evidence-only',sessionMode:'new',timeoutMs:10000,disposeGraceMs:50,subscriptionPolicy:{version:1,mode:'subscription-only'}}
writeFileSync(join(installed,'request.json'),JSON.stringify(request))
writeFileSync(join(installed,'consumer.mjs'),`import assert from 'node:assert/strict';import {readFileSync,mkdirSync} from 'node:fs';import {createRequire} from 'node:module';import {runMachine,recoverMachine,MACHINE_CAPABILITIES} from 'dsh-subagent-antigravity/machine';
const require=createRequire(import.meta.url);assert.throws(()=>require.resolve('@deepseek-ai/schemastery'));
const input=JSON.parse(readFileSync(new URL('./request.json',import.meta.url),'utf8'));const root=process.env.HOME+'/journals';mkdirSync(root,{mode:0o700});
const receipt=await runMachine(input,{recoveryDirectory:root+'/run'});assert.equal(receipt.ok,true,JSON.stringify(receipt.failure));const recovery=await recoverMachine({protocol:1,requestId:receipt.requestId,runId:receipt.runId,recoveryDirectory:root+'/run'});assert.deepEqual(recovery.receipt,receipt);assert.equal(MACHINE_CAPABILITIES.evidenceOnly,'verified-empty-init-tools');
const swarm=await runMachine({...input,requestId:'packed-swarm',model:'gemini-pro-orchestrator',effort:'',toolPolicy:'native',native:{version:1,mode:'swarm',maxObservedAgents:3}},{env:{...process.env,FIXTURE_SCENARIO:'swarm-children'}});assert.equal(swarm.ok,true,JSON.stringify(swarm.failure));assert.equal(swarm.native.observedAgents,2);assert.equal(swarm.native.children[0].state,'unknown');assert.equal(swarm.native.completion.scope,'cli-invocation');assert.equal(swarm.native.limit.hard,false);assert.equal(MACHINE_CAPABILITIES.nativeOrchestrator,true);
console.log(JSON.stringify({ok:receipt.ok,swarm:swarm.ok,version:require('dsh-subagent-antigravity/package.json').version}));`)
const consumer=run(process.execPath,[join(installed,'consumer.mjs')],{cwd:installed,env:{...process.env,HOME:home}})
const bin=join(installed,'node_modules','.bin','antigravity-machine')
const capabilities=JSON.parse(run(bin,['--capabilities'],{cwd:installed}));assert.equal(capabilities.protocol,1)
const cli=JSON.parse(run(bin,['--json'],{cwd:installed,input:JSON.stringify({...request,requestId:'wrapper'}),env:{...process.env,HOME:home}}));assert.equal(cli.ok,true)
writeFileSync(join(installed,'types.mts'),`import {runMachine,MachineRequest,MachineReceipt,MachineLifecycleEvent,NativeOptions,NativeReceipt} from 'dsh-subagent-antigravity/machine';declare const request:MachineRequest;const native:NativeOptions={version:1,mode:'swarm',maxObservedAgents:4};const receipt:MachineReceipt=await runMachine({...request,model:'gemini-pro-orchestrator',effort:'',native},{onLifecycle:(event:MachineLifecycleEvent)=>{event.dispatch;},onActivity:event=>{event.native?.children[0]?.workspaceUris;}});receipt.configuration.observed?.tools;const observed:NativeReceipt=receipt.native;const hard:false=observed.limit.hard;receipt.execution.finishedReason;`)
run(process.execPath,[join(root,'node_modules/typescript/bin/tsc'),'--strict','--noEmit','--module','NodeNext','--target','ES2022','--typeRoots',join(root,'node_modules/@types'),join(installed,'types.mts')])
console.log('Installed public ESM, types, wrapper, supervisor, journal, evidence-only and Orchestrator/Swarm smoke passed: '+consumer.trim())
console.log('Candidate: '+tarball+'\nIntegrity: '+packed.integrity+'\nInstalled: '+installed)
writeFileSync(join(root,'candidate.json'),JSON.stringify({version:packed.version,tarball,integrity:packed.integrity,installed},null,2)+'\n')
