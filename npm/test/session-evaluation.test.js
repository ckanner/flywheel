"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const { tmpdir } = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { VARIANTS, validateManifest, variantConfig, resolveMetricRows, summarizeTrial, compareTrials, evaluate } = require("../../scripts/evaluate-sessions.js");

function git(dir, ...args) {
  const p = spawnSync("git", ["-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-C", dir, ...args], { encoding: "utf8" });
  assert.equal(p.status, 0, p.stderr); return p.stdout.trim();
}
const baseConfig = { sessions: { mode: "scoped", "policy-version": 1, quality: { "max-confirmed-turns": 8 } },
  drivers: { execute: "real", review: "real" }, loops: { review: { harness: "codex-pro", model: "chosen-model", thinking: "high", channel: "subscription" } },
  routes: { default: { harness: "codex-pro", model: "chosen-model", thinking: "high", channel: "subscription" } }, permissions: { mode: "role-based" } };
const oracle = `const fs = require('node:fs'); const p=require('node:path');
const pass = fs.readFileSync(p.join(process.argv[2],'result.txt'),'utf8') === 'verified output';
process.stdout.write(JSON.stringify({version:1,pass,omissions:pass?[]:['required-output'],regressions:[],evidence:['independent exact artifact comparison']}));\n`;
// This is an evaluation-runner protocol fixture, explicitly labelled offline.
// Go quality policy and real harness behavior are tested separately; these
// synthesized numbers are never presented as measured model performance.
const engine = `#!/usr/bin/env node
const fs=require('node:fs'),p=require('node:path'),crypto=require('node:crypto');
const task=process.argv[3],config=JSON.parse(fs.readFileSync(process.env.FLYWHEEL_CONFIG||p.join(task,'flywheel.config.json'),'utf8'));
const scenario=JSON.parse(fs.readFileSync(p.join(task,'scenario.json'),'utf8')).scenario;
const file=p.join(task,'state.json'),prior=fs.existsSync(file)?JSON.parse(fs.readFileSync(file,'utf8')):null;
const n=(prior?.fixtureRun||0)+1;
function finish(cancelled=false){
 const roleList=(scenario==='l1-repair'||scenario==='review-return'||scenario==='long-context')?['execute','review','execute','review']:['execute','review'];
 const rows=roleList.map((role,i)=>{const scoped=config.sessions.roles[role]==='scoped';return {
  'invocation-id':n+'-'+role+'-'+i,role,model:config.routes.default.model,
  usage:scenario==='missing-usage'?null:{input:scenario==='long-context'?150000:100,output:20,cacheRead:scoped&&i>1?40:0,cacheWrite:0,totalTokens:scenario==='long-context'?150020:120},
  session:{role,scope:role==='review'?'final:M01-T01':'M01-T01',binding:crypto.createHash('sha256').update(JSON.stringify(config.loops)).digest('hex'),
   action:scoped&&i>1?'resume':'create',reason:'fixture',generation:1,
   last:{id:n+'-'+role+'-'+i,status:'consumed',accounted:true,dispatch:'dispatched',outcome:scenario==='unknown'?'unknown':cancelled?'cancelled':'completed','cleanup-confirmed':true,'requested-thread':scoped&&i>1?role+'-history':''}}};});
 fs.appendFileSync(p.join(task,'metrics.jsonl'),rows.map(x=>JSON.stringify(x)).join('\\n')+'\\n');
 const history=scenario==='l1-repair'?[{evidence:{l1:{pass:false}}}]:scenario==='review-return'?[{evidence:{review:{verdict:'return',reasons:['missing edge case']}}}]:[];
 const state={fixtureRun:n,status:cancelled?'interrupted':process.argv.includes('--pause-before')?'paused':'completed',packets:{'M01-T01':{attempts:roleList.length/2*n,'evidence-history':history,evidence:{l1:{pass:true},review:{verdict:'pass'}}}},sessions:{records:Object.fromEntries(rows.slice(-2).map(x=>[x.role,x.session]))}};
 fs.writeFileSync(file,JSON.stringify(state));fs.writeFileSync(p.join(process.cwd(),'result.txt'),'verified output');
 if(scenario==='tamper-oracle'){const f=p.resolve(process.cwd(),'../oracle.cjs');fs.chmodSync(f,0o600);fs.writeFileSync(f,'changed');}
 if(scenario==='tamper-config'){config.routes.default.model='different-model';fs.writeFileSync(process.env.FLYWHEEL_CONFIG,JSON.stringify(config));}
 console.log(JSON.stringify({status:state.status}));process.exit(cancelled?143:0);
}
if(scenario==='stop-resume'&&n===1&&!process.argv.includes('--pause-before')){process.on('SIGTERM',()=>finish(true));setInterval(()=>{},1000);}else finish();
`;

async function fixture(t, scenarios = ["first-pass"]) {
  const dir = await fs.mkdtemp(path.join(tmpdir(), "flywheel-session-eval-"));
  let retain=false;
  t.after(() => retain ? undefined : fs.rm(dir, { recursive: true, force: true }));
  const repo = path.join(dir, "source"); await fs.mkdir(repo);
  git(repo, "init", "-q"); git(repo, "config", "user.email", "eval-test@localhost"); git(repo, "config", "user.name", "Eval Test");
  await fs.writeFile(path.join(repo, "flywheel.config.json"), JSON.stringify(baseConfig));
  await fs.writeFile(path.join(repo, "oracle.cjs"), oracle);
  const cases=[];
  for(const scenario of scenarios) {
    const taskDir=`docs/${scenario}`; await fs.mkdir(path.join(repo, taskDir), { recursive:true });
    await fs.writeFile(path.join(repo, taskDir, "scenario.json"), JSON.stringify({scenario}));
    const c={id:scenario,scenario:["missing-usage","unknown","tamper-oracle","tamper-config"].includes(scenario)?"first-pass":scenario,taskDir,oracle:"oracle.cjs",steps:[{kind:"run"}]};
    if(scenario==='long-context')c.minRunInputTokens=100000;
    if(scenario==='configuration-change')c.steps=[{kind:"run",only:"execute",packet:"M01-T01"},{kind:"configure",section:"loops",name:"review",thinking:"low"},{kind:"run"}];
    if(scenario==='stop-resume')c.steps=[{kind:"run",pauseBeforeReview:true},{kind:"run"}];
    cases.push(c);
  }
  git(repo,"add","-A");git(repo,"commit","-qm","test: freeze experiment conditions");
  const commit=git(repo,"rev-parse","HEAD");
  const executable=path.join(dir,"engine.js");await fs.writeFile(executable,engine,{mode:0o755});
  return {dir,repo,executable,retain:()=>{retain=true;},manifest:{version:1,execution:"offline-fixture",source:{repository:repo,commit},config:"flywheel.config.json",repetitions:2,limits:{maxCalls:1000,runTimeoutMs:5000},cases}};
}

test("session evaluation validates immutable conditions and never widens a variant's model or permissions", () => {
  for(const variant of VARIANTS) {
    const cfg=variantConfig(baseConfig,variant);
    assert.deepEqual(cfg.loops,baseConfig.loops);assert.deepEqual(cfg.routes,baseConfig.routes);assert.deepEqual(cfg.permissions,baseConfig.permissions);
    assert.deepEqual(cfg.sessions.quality,baseConfig.sessions.quality);
    assert.equal(cfg.sessions.roles.plan,"fresh");assert.equal(cfg.sessions.roles.judge,"fresh");
    assert.equal(cfg.sessions.roles.execute,variant==='fresh'?'fresh':'scoped');
    assert.equal(cfg.sessions.roles.review,variant==='role-scoped'?'scoped':'fresh');
  }
  assert.throws(()=>variantConfig(baseConfig,"best"),/unknown/);
  assert.equal(baseConfig.sessions.mode,"scoped");
});

test("session evaluation rejects mutable refs, path escapes, unknown keys and unpaired trials", async t => {
  const f=await fixture(t);assert.equal(validateManifest(f.manifest),f.manifest);
  for(const mutate of [
    m=>m.source.commit="main",m=>m.config="../private",m=>m.cases[0].taskDir="/tmp/outside",m=>m.cases[0].oracle="evil/../../file",
    m=>m.repetitions=1,m=>m.credentials="literal-secret",m=>m.execution="auto",m=>m.cases[0].steps[0].shell="echo ignored",
    m=>m.cases[0].steps[0]={kind:"configure",section:"channels",name:"paid",model:"new"},
    m=>m.cases.push(structuredClone(m.cases[0])),m=>m.cases[0].steps[0].only="publish",
    m=>m.cases[0].steps[0].stopAfterMs=5000,
  ]){const m=structuredClone(f.manifest);mutate(m);assert.throws(()=>validateManifest(m));}
  const live={...f.manifest,execution:"live"};
  await assert.rejects(evaluate(live,{flywheel:f.executable,output:path.join(f.dir,"live")}),/require --live/);
  assert.equal(git(f.repo,"status","--porcelain"),"");
});

test("session evaluation preserves missing usage and refuses unpaired numeric savings", () => {
  const row={role:"execute",usage:null,session:{role:"execute",scope:"M01-T01",action:"resume",last:{id:"one",dispatch:"dispatched",outcome:"unknown","cleanup-confirmed":true,"requested-thread":"history"}}};
  const result=summarizeTrial({status:"escalated",packets:{},sessions:{records:{execute:row.session}}},[row,row],[],{pass:null},{scenario:"long-context",minRunInputTokens:100});
  assert.equal(result.calls,1);assert.equal(result.duplicateMetricRows,1);assert.equal(result.usage.input.known,0);assert.equal(result.usage.input.missingCalls,1);
  assert.equal(result.scenario.observed,null);assert.equal(result.recovery.unresolved,1);assert.equal(result.recovery.failedOrUnknownResumes,1);
  const t={case:"x",repetition:1,variant:"fresh",condition:"same",summary:result};
  const comparison=compareTrials([t,{...t,variant:"executor-scoped"}])[0];
  assert.equal(comparison.complete,false);assert.equal(comparison.deltas[0].usage.totalTokens,null);
});

test("recovery audit corrections replace measured facts without inventing model calls or summing usage twice", () => {
  const call={"invocation-id":"call-one",role:"execute",usage:null,"cost-usd":null,
    session:{action:"create",reason:"first call",last:{id:"call-one",outcome:"unknown",dispatch:"unknown","cleanup-confirmed":false}}};
  const correction={"invocation-id":"recover-one","record-kind":"reconciliation","reconciles-invocation":"call-one",
    usage:{input:12,output:4,cacheRead:8,cacheWrite:0,totalTokens:16},"cost-usd":null,
    session:{action:"reconcile",last:{id:"call-one",outcome:"completed",dispatch:"dispatched","cleanup-confirmed":true}}};
  const resolved=resolveMetricRows([call,correction,correction]);
  assert.equal(resolved.calls.length,1);assert.equal(resolved.corrections,1);assert.equal(resolved.duplicates,1);
  assert.equal(resolved.calls[0].usage.totalTokens,16);assert.equal(resolved.calls[0].session.action,"create");
  const summary=summarizeTrial({status:"completed",packets:{}},[call,correction],[],{pass:true},{scenario:"first-pass"});
  assert.equal(summary.calls,1);assert.equal(summary.usage.totalTokens.known,16);assert.equal(summary.usage.totalTokens.missingCalls,0);
  assert.equal(summary.recovery.reconciliations,1);assert.equal(summary.cost.missingCalls,1);
  assert.throws(()=>resolveMetricRows([correction]),/no original/);
});

test("session evaluation executes two paired repetitions of all six scenarios in separate real Git clones", async t => {
  const f=await fixture(t,["first-pass","l1-repair","review-return","long-context","stop-resume","configuration-change"]);
  const physicalParent = path.join(f.dir, "physical-output");
  const outputAlias = path.join(f.dir, "output-alias");
  await fs.mkdir(physicalParent);
  await fs.symlink(physicalParent, outputAlias);
  const output = path.join(outputAlias, "report");
  const report=await evaluate(f.manifest,{flywheel:f.executable,output});
  if(!report.complete){f.retain();t.diagnostic(JSON.stringify({output,stopReason:report.stopReason,last:report.trials.at(-1)}));}
  assert.equal(report.complete,true,report.stopReason);assert.equal(report.execution,"offline-fixture");assert.equal(report.trials.length,36);
  assert.equal(report.comparisons.length,12);assert.ok(report.comparisons.every(p=>p.complete));
  assert.ok(report.trials.every(t=>t.summary.quality.pass===true));
  assert.equal(new Set(report.trials.map(t=>t.workspace)).size,36);
  for(const trial of report.trials) {
    assert.equal(path.dirname(trial.configuration),path.dirname(trial.workspace));
    await assert.rejects(fs.stat(path.join(trial.workspace,"flywheel.config.json")),{code:"ENOENT"});
  }
  for(const status of Object.values(report.scenarioCoverage)){assert.equal(status.observed,6);assert.equal(status.unknown,0);}
  assert.notDeepEqual(report.trials[0].order,report.trials.find(t=>t.case===report.trials[0].case&&t.repetition===2).order);
  assert.equal(git(f.repo,"status","--porcelain"),"");assert.equal(git(f.repo,"rev-parse","HEAD"),f.manifest.source.commit);
  const saved=JSON.parse(await fs.readFile(path.join(output,"report.json"),"utf8"));assert.equal(saved.complete,true);
  await assert.rejects(evaluate(f.manifest,{flywheel:f.executable,output}),/EEXIST/);
});

test("session evaluation keeps unknown metrics distinct from zero in completed paired experiments", async t => {
  const f=await fixture(t,["missing-usage"]);
  const report=await evaluate(f.manifest,{flywheel:f.executable,output:path.join(f.dir,"report")});
  assert.equal(report.complete,true,report.stopReason);
  assert.ok(report.comparisons.flatMap(p=>p.deltas).every(d=>d.usage.totalTokens===null));
  assert.ok(report.trials.every(t=>t.summary.usage.input.missingCalls===2));
});

test("session evaluation stops all further dispatch when a terminal fact is unknown", async t => {
  const f=await fixture(t,["unknown"]);
  const report=await evaluate(f.manifest,{flywheel:f.executable,output:path.join(f.dir,"report")});
  assert.equal(report.complete,false);assert.match(report.stopReason,/unresolved/);assert.equal(report.trials.length,1);
  assert.equal(report.trials[0].summary.recovery.unresolved,2);
});

test("a planned stop during cold Node startup is recorded and never retried as a successful pause",async t=>{
  const f=await fixture(t,["stop-resume"]);
  const preload=path.join(f.dir,"slow-start.cjs");
  await fs.writeFile(preload,"Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,600);\n");
  f.manifest.cases[0].steps=[{kind:"run",stopAfterMs:150},{kind:"run"}];
  const output=path.join(f.dir,"report");
  const report=await evaluate(f.manifest,{flywheel:f.executable,output,env:{...process.env,NODE_OPTIONS:'--require='+preload}});
  assert.equal(report.complete,false);assert.equal(report.trials.length,1);assert.equal(report.calls,0);
  assert.match(report.stopReason,/engine admission or runtime failure/);
  const stopped=report.trials[0].steps[0].process;
  assert.equal(stopped.stopReason,"planned-stop");assert.equal(stopped.exitCode,null);assert.equal(stopped.signal,"SIGTERM");
  assert.equal(report.trials[0].summary.scenario.observed,false);
  if(process.env.FLYWHEEL_EVAL_TEST_KEEP==='1'){f.retain();t.diagnostic(JSON.stringify({output,stopReason:report.stopReason,process:stopped}));}
});

test("session evaluation rejects a candidate that tampers with the independent oracle", async t => {
  const f=await fixture(t,["tamper-oracle"]);
  const report=await evaluate(f.manifest,{flywheel:f.executable,output:path.join(f.dir,"report")});
  assert.equal(report.complete,false);assert.match(report.stopReason,/oracle changed/);assert.equal(report.trials.length,1);
});

test("session evaluation stops if a model changes the fixed configuration", async t => {
  const f=await fixture(t,["tamper-config"]);
  const report=await evaluate(f.manifest,{flywheel:f.executable,output:path.join(f.dir,"report")});
  assert.equal(report.complete,false);assert.match(report.stopReason,/configuration changed during model execution/);
  assert.equal(report.trials.length,1);assert.equal(report.calls,2);
});

for(const target of ["state.json","metrics.jsonl","new-contract.md","external-config"]){
  test("controlled injection cannot write engine evidence or experiment configuration: "+target,async t=>{
    const f=await fixture(t);
    const destination=target==="external-config"?"process.env.FLYWHEEL_CONFIG":"p.join(process.argv[3],"+JSON.stringify(target)+")";
    await fs.writeFile(path.join(f.repo,"inject.cjs"),"const fs=require('node:fs'),p=require('node:path');fs.writeFileSync("+destination+",'changed');\n");
    git(f.repo,"add","-A");git(f.repo,"commit","-qm","test: freeze invalid injector");
    f.manifest.source.commit=git(f.repo,"rev-parse","HEAD");
    f.manifest.cases[0].steps.push({kind:"inject",script:"inject.cjs",reason:"test evidence guard"},{kind:"run"});
    const report=await evaluate(f.manifest,{flywheel:f.executable,output:path.join(f.dir,"report")});
    assert.equal(report.complete,false);assert.match(report.stopReason,/injection changed/);
    assert.equal(report.trials.length,1);assert.equal(report.calls,2);
  });
}
