#!/usr/bin/env node
// flywheel CLI (DESIGN.md §12/§13): the bottom interface and the fallback path.
// Skills are just buttons on top of these commands.
//
//   flywheel run <docs/slug> [--only plan|execute|review|verify] [--packet ID|--unit ID] [--pause-before review]
//   flywheel status <docs/slug>
//   flywheel stop <docs/slug>

import { existsSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { resolve, basename } from 'node:path';
import { loadConfig } from '../orchestrator/core/config.js';
import { createDrivers } from '../orchestrator/drivers/index.js';
import { runTask, runSingleStep, pidFile } from '../orchestrator/core/machine.js';
import { loadState, saveState, stateExists } from '../orchestrator/core/state.js';
import { runIntake } from '../orchestrator/modules/intake.js';
import { runGarden } from '../orchestrator/modules/garden.js';
import { installSkills, repoRoot } from '../orchestrator/modules/install-skills.js';

const ONLY_STEPS = ['plan', 'execute', 'review', 'verify'];

function usage() {
  console.log(`flywheel — Loop Engineering 编排器（DESIGN.md v0.14）

用法:
  flywheel run <docs/slug> [--only plan|execute|review|verify] [--packet ID|--unit ID] [--pause-before review]
  flywheel status <docs/slug>
  flywheel stop <docs/slug>
  flywheel intake <docs/slug> [--source <name>]
  flywheel garden <docs/slug>
  flywheel install-skills [--force] [--copy] [--project <path>]

说明:
  run      以当前进程执行全自动循环；--only 只走一步状态转移（§12）
  status   只读：读 state.json/指标，讲进度
  stop     停编排器进程，状态定格
  intake   外挂：扫任务源 → 候选进 triage-inbox → LLM 分诊建议（不冻结需求，§4.5）
  garden   外挂：分析 retro/知识资产 → garden-proposals.md 提案（需人审，§4.6）
  install-skills  把 skills/ 安装到 ~/.agents/skills（软链默认；--copy 快照；--project 项目级）`);
}

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--force' || a === '--copy') {
      args[a.slice(2)] = true;
    } else if (a === '--only' || a === '--packet' || a === '--unit' || a === '--pause-before' || a === '--source' || a === '--project') {
      const key = a.slice(2);
      args[key] = argv[++i];
    } else if (a.startsWith('--')) {
      throw new Error(`未知参数: ${a}`);
    } else {
      args._.push(a);
    }
  }
  return args;
}

function resolveTaskDir(slugArg) {
  if (!slugArg) throw new Error('缺少任务路径（docs/<slug>）');
  const taskDir = resolve(process.cwd(), slugArg);
  if (!existsSync(taskDir)) throw new Error(`任务目录不存在: ${taskDir}`);
  return taskDir;
}

const log = (msg) => console.log(`[flywheel] ${msg}`);

async function cmdRun(args) {
  const taskDir = resolveTaskDir(args._[0]);
  if (args.only && !ONLY_STEPS.includes(args.only)) {
    throw new Error(`--only 只支持: ${ONLY_STEPS.join('|')}`);
  }
  if (args['pause-before'] && args['pause-before'] !== 'review') {
    throw new Error('--pause-before 只支持: review');
  }
  const packetId = args.packet ?? args.unit ?? null;
  const config = loadConfig(taskDir, process.cwd());
  const drivers = createDrivers(config);
  const opts = { taskDir, cwd: process.cwd(), config, drivers, log, packetId, pauseBefore: args['pause-before'] ?? null };
  const result = args.only
    ? await runSingleStep({ ...opts, only: args.only })
    : await runTask(opts);
  console.log(JSON.stringify(result, null, 2));
  if (result.status === 'escalated' || result.status === 'budget-stopped') process.exitCode = 3;
  if (result.step && result.step.ok === false) process.exitCode = 3; // manual step failed (#9)
}

function cmdStatus(args) {
  const taskDir = resolveTaskDir(args._[0]);
  if (!stateExists(taskDir)) throw new Error(`state.json 不存在于 ${taskDir}（任务尚未启动？）`);
  const state = loadState(taskDir);
  let alive = false;
  let pid = null;
  if (existsSync(pidFile(taskDir))) {
    try {
      pid = JSON.parse(readFileSync(pidFile(taskDir), 'utf8')).pid;
      process.kill(pid, 0);
      alive = true;
    } catch { /* not running */ }
  }

  const b = state.budget;
  console.log(`任务: ${state.slug}  状态: ${state.status}${alive ? `（编排器运行中, pid ${pid}）` : ''}`);
  console.log(`当前里程碑: ${state['current-milestone'] ?? '—'}  上次推进: ${state['last-progress-at'] ?? '—'}  revision: ${state.revision}`);
  console.log('');
  for (const [mid, m] of Object.entries(state.milestones)) {
    console.log(`里程碑 ${mid}: ${m.status}${m['closed-at'] ? `（关闭于 ${m['closed-at']}）` : ''}`);
    for (const [pid2, p] of Object.entries(state.packets)) {
      if (p.milestone !== mid) continue;
      const flags = [p.interrupted ? 'interrupted' : null].filter(Boolean).join(',');
      console.log(
        `  ${pid2}: ${p.status}${flags ? ` [${flags}]` : ''}  尝试 ${p.attempts}（失败 ${p['failed-attempts']}）` +
        `  证据: executor=${p.evidence.executor ? '✓' : '—'} L1=${p.evidence.l1 ? (p.evidence.l1.pass ? '✓' : '✗') : '—'} review=${p.evidence.review ? p.evidence.review.verdict : '—'}`,
      );
    }
  }
  console.log('');
  console.log(`token 累计: ${b.totals.tokens}（in ${b.totals.input} / out ${b.totals.output} / reasoning ${b.totals.reasoning}）  成本: $${b.totals['cost-usd']}`);
  console.log(`缓存命中率: ${(b.cache['hit-rate'] * 100).toFixed(1)}%  预算档: ${b.enforce}（日上限 ${b['daily-token-limit'] ?? '未设'}）`);
  if (b.warnings.length > 0) console.log(`预算告警: ${b.warnings[b.warnings.length - 1]}`);
  if (state.inbox.length > 0) {
    console.log('');
    console.log(`inbox（需要人处理）: ${state.inbox.length} 条，最近: ${state.inbox[state.inbox.length - 1].reason}`);
  }
}

async function cmdStop(args) {  const taskDir = resolveTaskDir(args._[0]);
  let stoppedPid = null;
  let confirmedDead = false;
  if (existsSync(pidFile(taskDir))) {
    try {
      const { pid } = JSON.parse(readFileSync(pidFile(taskDir), 'utf8'));
      stoppedPid = pid;
      process.kill(pid, 'SIGTERM');
      for (let i = 0; i < 50; i++) {
        try {
          process.kill(pid, 0);
          await new Promise((r) => setTimeout(r, 100));
        } catch {
          confirmedDead = true;
          break;
        }
      }
    } catch {
      confirmedDead = true; // already dead before we signalled
    }
    if (!confirmedDead) {
      // Never claim a stop we cannot confirm (#7).
      console.log(`SIGTERM 已发送给 pid ${stoppedPid}，但 5s 内未确认退出。state.json 保持原样；请检查进程后重试，或手动 kill。`);
      process.exitCode = 1;
      return;
    }
    rmSync(pidFile(taskDir), { force: true });
  }
  if (stateExists(taskDir)) {
    const state = loadState(taskDir);
    if (state.status !== 'completed') {
      state.status = 'stopped';
      saveState(taskDir, state, { expectedRevision: state.revision });
    }
  }
  console.log(stoppedPid ? `已停止编排器进程 (pid ${stoppedPid})，状态定格。` : '编排器未在运行，状态已定格为 stopped。');
}

async function cmdIntake(args) {
  const taskDir = resolveTaskDir(args._[0]);
  const config = loadConfig(taskDir, process.cwd());
  const drivers = createDrivers(config);
  const result = await runIntake({
    taskDir,
    cwd: process.cwd(),
    config,
    drivers,
    source: args.source ?? null,
    log,
  });
  console.log(JSON.stringify(result, null, 2));
  if (result.triaged?.failed > 0) process.exitCode = 3;
}

async function cmdGarden(args) {
  const taskDir = resolveTaskDir(args._[0]);
  const config = loadConfig(taskDir, process.cwd());
  const drivers = createDrivers(config);
  const result = await runGarden({ taskDir, cwd: process.cwd(), config, drivers, log });
  console.log(JSON.stringify(result, null, 2));
  if (!result.ok) process.exitCode = 3;
}

function cmdInstallSkills(args) {
  const { targetBase, scope, results } = installSkills({
    force: Boolean(args.force),
    copy: Boolean(args.copy),
    project: args.project ? resolve(process.cwd(), args.project) : null,
  });
  console.log(`安装范围: ${scope}  目标目录: ${targetBase}  方式: ${args.copy ? '复制（快照）' : '软链（默认）'}`);
  let errors = 0;
  for (const r of results) {
    if (r.action === 'error') errors += 1;
    const actionText = { linked: '已软链', copied: '已复制', skipped: '已安装，跳过', error: '错误' }[r.action];
    console.log(`  ${r.name}: ${actionText}${r.reason ? `（${r.reason}）` : ''} -> ${r.dest}`);
  }
  console.log('');
  console.log('验证方式: 在 kimi code 或 pi 中调用 /skill:<name>，例如 /skill:flywheel-run。');
  if (errors > 0) process.exitCode = 1;
}

async function main() {
  const [command, ...rest] = process.argv.slice(2);
  const args = parseArgs(rest);
  switch (command) {
    case 'run':
      return cmdRun(args);
    case 'status':
      return cmdStatus(args);
    case 'stop':
      return cmdStop(args);
    case 'intake':
      return cmdIntake(args);
    case 'garden':
      return cmdGarden(args);
    case 'install-skills':
      return cmdInstallSkills(args);
    case undefined:
    case 'help':
    case '--help':
      usage();
      return;
    default:
      throw new Error(`未知命令: ${command}`);
  }
}

main().catch((err) => {
  console.error(`[flywheel] 错误: ${err.message}`);
  process.exitCode = 1;
});
