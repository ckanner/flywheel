#!/usr/bin/env node
"use strict";

// Build an executable, pinned repair corpus without contacting a model. The
// supplied binding file is copied, never changed. Accounts and credentials are
// not copied into the source repository or manifest.
const fs = require("node:fs/promises");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { SCENARIOS, validateManifest } = require("./evaluate-sessions.js");

const core = `function expected(values) {
  const a = [...new Set(values)].sort((x,y) => x-y), result=[];
  for (const n of a) { const tail=result[result.length-1]; if(tail && n===tail[1]+1)tail[1]=n;else result.push([n,n]); }
  return result;
}
const cases=[[],[1],[3,2,1,1],[0,-2,-1,2,4],[-5,-3,-4,7,8,7],[10,2,1,9,0]];
for(let i=0;i<60;i++) cases.push(Array.from({length:i%13},(_,j)=>((i*17+j*7)%41)-20));
`;
const acceptance = `"use strict";
const assert=require("node:assert/strict"),path=require("node:path");
${core}
const fn=require(path.resolve("lib",process.argv[2]+".cjs"));
for(const values of cases)assert.deepEqual(fn([...values]),expected(values));
console.log("all frozen output checks passed; input purity remains an explicit independent Review obligation");
`;
const oracle = `"use strict";
const fs=require("node:fs"),path=require("node:path");
${core}
const id=path.basename(process.argv[3]),file=path.join(process.argv[2],"lib",id+".cjs");
const omissions=[],regressions=[],evidence=[];
try {
  const fn=require(file);
  for(let i=0;i<cases.length;i++) {
    const input=[...cases[i]],before=JSON.stringify(input);
    let actual;
    try {actual=fn(input);}catch(error){omissions.push("case-"+i+": exception");continue;}
    if(JSON.stringify(actual)!==JSON.stringify(expected(cases[i])))omissions.push("case-"+i+": incorrect interval output");
    if(JSON.stringify(input)!==before)regressions.push("case-"+i+": caller input mutated");
  }
  evidence.push("frozen independent oracle executed "+cases.length+" examples and checked caller input preservation");
}catch(error){omissions.push("R-01: implementation could not be loaded");evidence.push("independent module loading failed");}
console.log(JSON.stringify({version:1,pass:omissions.length===0&&regressions.length===0,omissions,regressions,evidence}));
`;
const injectL1 = `const fs=require("node:fs"),path=require("node:path");
const id=path.basename(process.argv[3]);
fs.writeFileSync(path.join(process.argv[2],"lib",id+".cjs"),"module.exports = function compactRanges(values) { return []; };\\n");
`;
const injectReview = `const fs=require("node:fs"),path=require("node:path");
const id=path.basename(process.argv[3]);
fs.writeFileSync(path.join(process.argv[2],"lib",id+".cjs"),
"module.exports = function compactRanges(values) {\\n  const a = [...new Set(values.sort((x,y) => x-y))], result = [];\\n  for (const n of a) { const tail=result[result.length-1]; if(tail && n===tail[1]+1)tail[1]=n;else result.push([n,n]); }\\n  return result;\\n};\\n");
`;

function git(cwd, args) {
  const r = spawnSync("git", ["-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-C", cwd, ...args],
    { encoding: "utf8", timeout: 30000, env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" } });
  if (r.error || r.status !== 0) throw new Error(r.error?.message || r.stderr);
  return r.stdout.trim();
}
function spec(id) {
  return `---
spec-version: 1
title: Immutable integer interval compaction (${id})
goal: Implement compactRanges for arbitrary arrays of safe integers without mutating the caller input.
non-goals: [network access, publishing, new dependencies, changing the frozen evaluation checks]
workflow:
  version: 1
  mode: quick
  reason: One local pure function; deterministic output checks and independent final Review cover the requirement.
  scope-clear: true
  implementation-known: true
  verification-complete: true
  review-sufficient: true
  evidence: ["eval/acceptance.cjs checks outputs; final Review checks input purity; the independent evaluation oracle checks both"]
  risks: []
  not-applicable: {L2: "No integration service", L3: "No user interface"}
milestones:
  - id: M-01
    title: Correct pure range compaction
    gate: Frozen acceptance and independent final Review pass.
requirements:
  - id: R-01
    title: Return sorted maximal inclusive ranges with duplicates removed, negative integers supported, and the input array unchanged.
    milestone: M-01
    verification: node eval/acceptance.cjs ${id} plus independent final Review of input purity
---

# Task

Export a CommonJS function from lib/${id}.cjs. For [3,1,2,2,7] return [[1,3],[7,7]].
For [] return []. Preserve the original array and do not change any input element.
Only safe integers are in scope. Review must inspect the complete implementation and independently check purity.
`;
}
function packet(id) {
  const body = `Implement only lib/${id}.cjs. Read the current code even when a retained conversation says it was already correct: the experiment may introduce a controlled external regression between calls. Preserve the frozen checks and task contracts. Produce the normal done-report and report the exact commands actually run. The independent final Reviewer must check output behavior and caller-array preservation; output-only checks do not establish purity.\n`;
  return `---
packet-id: M01-T01
contract-version: 1
goal: Implement the current pure compactRanges requirement completely.
requirements: [R-01]
route: default
needs-live-credentials: false
context-files: [lib/${id}.cjs, eval/acceptance.cjs]
cache-scope: ${id === "long-context" ? "[reference/long-context.txt]" : "[]"}
boundaries:
  must-not-touch: [eval/, docs/, reference/, AGENTS.md, flywheel.config.json]
  max-files: 2
acceptance:
  L1:
    - {cmd: "node eval/acceptance.cjs ${id}", expect: "exit 0"}
review-rubric: |
  Pass only if all current R-01 behavior is correct. Independently verify that the caller array is unchanged, including unsorted arrays; in-place Array.sort on the argument is a regression even when all output checks pass. Return with concrete findings for a defect. Rerun the entire scope after a repair, not just the prior finding.
escalation: {max-retries: 3, on-fail: inbox}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

${body}`;
}
async function prepare(config, output, repetitions = 2) {
  if (!config || typeof config !== "object" || !config.routes?.default || !config.loops?.review) throw new Error("config needs explicit default Execute and Review bindings");
  const destination = path.resolve(output);
  await fs.mkdir(destination, { mode: 0o700 });
  const repo = path.join(destination, "source");
  await fs.mkdir(repo, { mode: 0o700 });
  await fs.mkdir(path.join(repo, "eval")); await fs.mkdir(path.join(repo, "lib"));
  await fs.mkdir(path.join(repo, "reference"));
  let reference = "Stable examples of the same pure integer range requirement. Preserve caller order and duplicates.\n\n";
  for (let i = 0; i < 1200; i++) {
    const n = i - 600;
    reference += `Reference ${i + 1}: input [${n + 2},${n},${n + 1},${n + 1}] produces [[${n},${n + 2}]]; the caller keeps its original ordering and duplicate entries.\n`;
  }
  await fs.writeFile(path.join(repo, "reference", "long-context.txt"), reference);
  await fs.writeFile(path.join(repo, "eval", "CORPUS"), "flywheel-session-evaluation-v1\n");
  // These are isolated experimental workspaces, so in-place execution is both
  // intentional and identical in every arm. Normal installation is unaffected.
  const copied = structuredClone(config);
  copied.worktree = { ...copied.worktree, enabled: false };
  copied.verify = { ...copied.verify, "protected-test-patterns": [...new Set([...(copied.verify?.["protected-test-patterns"] || []), "^eval/"])] };
  await fs.writeFile(path.join(repo, "flywheel.config.json"), JSON.stringify(copied, null, 2) + "\n");
  await fs.writeFile(path.join(repo, "AGENTS.md"), "# Evaluation workspace\n\nImplement only the current packet's lib file and its normal done-report. Preserve eval/, all task contracts, and configuration. The orchestrator owns state and performs Git commits. Do not change account configuration, publish, contact external services, or copy private history between roles.\n");
  await fs.writeFile(path.join(repo, ".gitignore"), "docs/**/metrics.jsonl\ndocs/**/orchestrator.log\ndocs/**/orchestrator.pid\ndocs/**/.sessions/\ndocs/**/.run.lock\ndocs/**/.state.lock\ndocs/**/.stop-request\ndocs/**/.control/\n");
  for (const [name, data] of Object.entries({ "acceptance.cjs": acceptance, "oracle.cjs": oracle, "inject-l1.cjs": injectL1, "inject-review.cjs": injectReview })) await fs.writeFile(path.join(repo, "eval", name), data);
  const cases = [];
  const run = { kind: "run" }, execute = { kind: "run", only: "execute", packet: "M01-T01" }, verify = { kind: "run", only: "verify", packet: "M01-T01" };
  for (const id of SCENARIOS) {
    const taskDir = `docs/${id}`;
    await fs.mkdir(path.join(repo, taskDir), { recursive: true });
    await fs.writeFile(path.join(repo, taskDir, "spec.md"), spec(id));
    await fs.writeFile(path.join(repo, taskDir, "task-packet-M01-T01.md"), packet(id));
    await fs.writeFile(path.join(repo, "lib", `${id}.cjs`), "module.exports = function compactRanges(values) { throw new Error('Implement the current R-01 contract'); };\n");
    const c = { id, scenario: id, taskDir, oracle: "eval/oracle.cjs", steps: [run] };
    if (id === "l1-repair") c.steps = [execute, { kind: "inject", script: "eval/inject-l1.cjs", reason: "Controlled external regression after a completed first implementation; real L1 must reject it" }, verify, run];
    if (id === "review-return") c.steps = [execute, { kind: "inject", script: "eval/inject-review.cjs", reason: "Controlled caller-array mutation regression that preserves all output checks; real independent Review must detect it" }, verify, run];
    if (id === "long-context") c.minRunInputTokens = 10000;
    if (id === "stop-resume") c.steps = [{ kind: "run", pauseBeforeReview: true }, run];
    if (id === "configuration-change") c.steps = [execute, { kind: "configure", section: "routes", name: "default", thinking: copied.routes.default.thinking === "medium" ? "high" : "medium" }, execute, run];
    cases.push(c);
  }
  git(repo, ["init", "-q"]); git(repo, ["config", "user.name", "Flywheel Session Corpus"]); git(repo, ["config", "user.email", "session-corpus@localhost"]);
  git(repo, ["add", "-A"]); git(repo, ["commit", "-qm", "test: freeze six paired session evaluation scenarios"]);
  const manifest = validateManifest({ version: 1, execution: "live", source: { repository: repo, commit: git(repo, ["rev-parse", "HEAD"]) },
    config: "flywheel.config.json", repetitions, limits: { maxCalls: 180, runTimeoutMs: 600000 }, cases });
  const file = path.join(destination, "manifest.json");
  await fs.writeFile(file, JSON.stringify(manifest, null, 2) + "\n", { mode: 0o600 });
  return { manifest: file, repository: repo, commit: manifest.source.commit, trials: cases.length * repetitions * 3 };
}

async function main(argv) {
  const options = {};
  for (let i=0;i<argv.length;i+=2) {
    if (!["--config", "--output", "--repetitions"].includes(argv[i]) || !argv[i+1]) throw new Error("usage: node scripts/prepare-session-evaluation.js --config CONFIG --output NEW_DIRECTORY [--repetitions 2]");
    options[argv[i].slice(2)] = argv[i+1];
  }
  if (!options.config || !options.output) throw new Error("--config and --output are required");
  const result = await prepare(JSON.parse(await fs.readFile(options.config,"utf8")),options.output,Number(options.repetitions||2));
  process.stdout.write(JSON.stringify(result)+"\n");
}
module.exports = { prepare, spec, packet };
if (require.main === module) main(process.argv.slice(2)).catch(error => { process.stderr.write(error.message+"\n"); process.exitCode=1; });
