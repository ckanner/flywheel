// L1 verification (DESIGN.md §4.4): the orchestrator runs deterministic
// acceptance commands itself — they are the ground-truth oracle, cheap and
// impossible for the executor to argue with. The coverage gate is just one
// more L1 command in the packet, no special-casing here.
//
// Test/boundary integrity is judged AFTER the fact via git diff (v0.10):
// a test file or a must-not-touch path in the unit's diff fails the unit.

import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { milestoneOfPacket, parseExpect } from './contracts.js';

import { gateEnv } from './child-env.js';

export { gateEnv } from './child-env.js';

const execFileAsync = promisify(execFile);

export const DEFAULT_TEST_FILE_PATTERNS = [
  '(^|/)(tests?|__tests__|spec)/',
  '\\.(test|spec)\\.[cm]?[jt]sx?$',
];

export { parseExpect } from './contracts.js';


/**
 * Run one gate level's commands (L1/L2/L3) in the task workspace.
 * Every command runs; no early stop, so the report is complete.
 * Commands get a WHITELISTED env (P0-4) — they execute executor-written code
 * and must not see the orchestrator's keys; config verify.env-allowlist is
 * the only explicit escape.
 *
 * @param {Array<{cmd?:string, script?:string, expect:string}>} commands
 * @param {{cwd:string, env?:object, timeoutMs?:number, envAllowlist?:string[]}} opts
 */
export async function runGateCommands(commands = [], { cwd, env, timeoutMs = 300_000, envAllowlist = [] } = {}) {
  const results = [];
  for (const entry of commands) {
    const expected = parseExpect(entry.expect);
    const command = entry.cmd ?? entry.script;
    const startedAt = Date.now();
    let exitCode;
    let output = '';
    try {
      await execFileAsync('/bin/sh', ['-c', command], {
        cwd,
        env: gateEnv(env ?? process.env, { allow: envAllowlist }),
        timeout: timeoutMs,
        maxBuffer: 4 * 1024 * 1024,
      });
      exitCode = 0;
    } catch (err) {
      exitCode = typeof err.code === 'number' ? err.code : 1;
      output = `${err.stdout ?? ''}${err.stderr ?? ''}`.slice(0, 2000);
    }
    results.push({
      cmd: command,
      expect: entry.expect,
      exitCode,
      pass: exitCode === expected,
      durationMs: Date.now() - startedAt,
      output: output || undefined,
    });
  }
  return { pass: results.every((r) => r.pass), results };
}

/**
 * Dirty worktree paths OUTSIDE the task dir (repo-relative, status porcelain
 * parsed). Returns null when the workspace is not a git checkout.
 *
 * The task dir is orchestrator-owned and expected to change (contract and
 * ledger files); anything ELSE dirty at run start is pre-existing WIP the
 * scoped unit commit would sweep in (P1-1 caveat — the diff is "everything
 * since the base ref", not "everything the executor touched").
 */
export async function foreignDirtyFiles(cwd, taskDirPrefix) {
  try {
    const { stdout } = await execFileAsync('git', ['-C', cwd, 'status', '--porcelain'], { maxBuffer: 4 * 1024 * 1024 });
    return stdout
      .split('\n')
      .map((line) => line.slice(3).trim())
      .filter((f) => f && !f.startsWith(taskDirPrefix));
  } catch {
    return null;
  }
}

// Well-known empty-tree sha: diff base for repositories with zero commits.
export const EMPTY_TREE = '4b825dc642cb6eb9a060e54bf8d69288fbee4904';

/**
 * Base ref for the current unit: HEAD when the workspace is a git checkout
 * (empty tree when it has no commits yet), null when it is not a git repo.
 * The orchestrator records this BEFORE the executor runs; all boundary
 * judgement then diffs against it, so an executor committing mid-unit cannot
 * silently drain the diff (§4.2/§4.4).
 */
export async function getBaseRef(cwd) {
  try {
    await execFileAsync('git', ['-C', cwd, 'rev-parse', '--git-dir'], { maxBuffer: 1024 * 1024 });
  } catch {
    return null; // not a git checkout
  }
  try {
    const { stdout } = await execFileAsync('git', ['-C', cwd, 'rev-parse', 'HEAD'], { maxBuffer: 1024 * 1024 });
    return stdout.trim();
  } catch {
    return EMPTY_TREE;
  }
}

/**
 * Changed files of the current unit, judged from git diff against baseRef
 * (defaults to HEAD). Diffing against the unit's base ref covers both the
 * executor's own commits and uncommitted changes; untracked files are listed
 * explicitly, since git diff never shows them.
 * Returns null when the workspace is not a usable git checkout — the caller
 * records the check as skipped.
 */
export async function getChangedFiles(cwd, baseRef = null) {
  try {
    const { stdout } = await execFileAsync('git', ['-C', cwd, 'diff', '--name-only', baseRef ?? 'HEAD'], {
      maxBuffer: 4 * 1024 * 1024,
    });
    const untracked = await execFileAsync('git', ['-C', cwd, 'ls-files', '--others', '--exclude-standard'], {
      maxBuffer: 4 * 1024 * 1024,
    });
    const files = new Set();
    for (const line of `${stdout}\n${untracked.stdout}`.split('\n')) {
      const f = line.trim();
      if (f) files.add(f);
    }
    return [...files].sort();
  } catch {
    return null;
  }
}

const DIFF_TEXT_CAP = 100_000; // ~100KB

/**
 * Full unified diff text against baseRef (DESIGN.md §4.3 promises the
 * reviewer the git diff, not a file list). Truncated at ~100KB with a marker.
 * `exclude` pathspecs (repo-relative) drop the orchestrator's ledger files
 * from the diff; contract files (spec/packets) stay IN.
 * Returns null when the workspace is not a usable git checkout.
 */
export async function getDiffText(cwd, baseRef, { exclude = [] } = {}) {
  try {
    const pathspecs = ['--', '.', ...exclude.map((p) => `:(exclude)${p}`)];
    const { stdout } = await execFileAsync('git', ['-C', cwd, 'diff', baseRef ?? 'HEAD', ...pathspecs], {
      maxBuffer: 8 * 1024 * 1024,
    });
    if (stdout.length > DIFF_TEXT_CAP) {
      return `${stdout.slice(0, DIFF_TEXT_CAP)}\n... [diff truncated at ${DIFF_TEXT_CAP} chars]`;
    }
    return stdout;
  } catch {
    return null;
  }
}

/**
 * Unit-granularity commit (§4.2: one commit per execution unit, rollback-able
 * and auditable). The ORCHESTRATOR owns this — never the executor.
 * The commit is SCOPED to the unit's own changes: everything modified or
 * added since the unit base ref (recorded on the FIRST attempt — the same
 * anchor the boundary judgement uses), minus `excludePaths` (the
 * orchestrator's volatile runtime files). Never `git add -A` — that would
 * sweep unrelated user work and the live orchestrator.log into the unit
 * commit. Skips with a recorded reason when the workspace is not a git
 * checkout or nothing changed; commit failures are reported, not thrown
 * (bookkeeping must not destroy verified work).
 */
export async function commitUnitChanges(cwd, packet, { excludePaths = [], baseRef = null } = {}) {
  const id = packet['packet-id'];
  const milestone = milestoneOfPacket(id);
  const goal = String(packet.goal ?? '').replace(/\s+/g, ' ').trim().slice(0, 50);
  const message = `feat(${milestone}): ${id} ${goal}`.trim();
  try {
    const base = baseRef ?? (await getBaseRef(cwd));
    if (base === null) return { sha: null, skipped: 'not a git checkout', message, at: new Date().toISOString() };
    const changed = await getChangedFiles(cwd, base);
    if (changed === null) {
      return { sha: null, skipped: 'changed-file scan failed', message, at: new Date().toISOString() };
    }
    const exclude = new Set(excludePaths);
    const addList = changed.filter((f) => !exclude.has(f));
    if (addList.length === 0) return { sha: null, skipped: 'no changes', message, at: new Date().toISOString() };
    const identity = ['-c', 'user.name=flywheel-orchestrator', '-c', 'user.email=flywheel-orchestrator@localhost'];
    await execFileAsync('git', ['-C', cwd, ...identity, 'add', '--', ...addList], { maxBuffer: 4 * 1024 * 1024 });
    await execFileAsync('git', ['-C', cwd, ...identity, 'commit', '-m', message], { maxBuffer: 4 * 1024 * 1024 });
    const { stdout } = await execFileAsync('git', ['-C', cwd, 'rev-parse', 'HEAD'], { maxBuffer: 1024 * 1024 });
    return { sha: stdout.trim(), message, at: new Date().toISOString() };
  } catch (err) {
    return { sha: null, error: String(err.message ?? err), message, at: new Date().toISOString() };
  }
}

function globToRegExp(glob) {
  const escaped = glob.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*');
  return new RegExp(`^${escaped}`);
}

/**
 * Pure boundary check over a list of changed files.
 *
 * @param {string[]|null} changedFiles null = diff unavailable -> skipped
 * @param {object} [boundaries] packet boundaries {must-not-touch:[], max-files:n}
 * @param {string[]} [testFilePatterns] regex strings
 */
export function checkBoundaries(changedFiles, boundaries = {}, testFilePatterns = DEFAULT_TEST_FILE_PATTERNS) {
  if (changedFiles === null) {
    return { pass: true, skipped: true, reason: 'git diff unavailable (not a git checkout)', violations: [], changedFiles: [] };
  }
  const violations = [];
  const mustNotTouch = boundaries['must-not-touch'] ?? [];
  const matchers = mustNotTouch.map((entry) => {
    if (entry.includes('*')) return globToRegExp(entry);
    const prefix = entry.endsWith('/') ? entry : `${entry}/`;
    return { exact: entry, prefix };
  });
  const testRes = testFilePatterns.map((p) => new RegExp(p));

  for (const file of changedFiles) {
    for (const m of matchers) {
      const hit = m instanceof RegExp ? m.test(file) : file === m.exact || file.startsWith(m.prefix);
      if (hit) violations.push({ type: 'must-not-touch', path: file });
    }
    if (testRes.some((re) => re.test(file))) {
      violations.push({ type: 'test-file', path: file });
    }
  }
  const maxFiles = boundaries['max-files'];
  if (typeof maxFiles === 'number' && changedFiles.length > maxFiles) {
    violations.push({ type: 'max-files', count: changedFiles.length, limit: maxFiles });
  }
  return { pass: violations.length === 0, skipped: false, violations, changedFiles };
}
