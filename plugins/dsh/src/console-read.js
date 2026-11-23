import { randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import { lstat, readdir, readFile, realpath, stat } from 'node:fs/promises';
import { extname, isAbsolute, join, relative, sep } from 'node:path';
import { Service } from '@deepseek-ai/cordis';
import { Remote, bindTypertRemote } from '@deepseek-ai/dsh-typert-protocol';
import { ControlNotDispatchedError } from './control.js';
import { validPlanningCommand } from './planning-control.js';
import { validSessionAbandonCommand } from './session-control.js';
import { buildResumeInfo, readTaskTraceDetail, readTaskTraces } from './trace-parser.js';

export const consoleReadActions = new Set([
  'models', 'images', 'runs', 'status', 'config', 'config-edit',
  'metrics', 'history', 'node', 'delivery', 'doctor', 'planning',
  'design-candidates', 'ux-review', 'traces', 'trace-detail',
]);

export const consoleActActions = new Set([
  'planning', 'investigate', 'launch', 'start', 'stop', 'step',
  'config-save', 'session', 'deliver', 'open-terminal',
]);

const IMAGE_EXTENSIONS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.gif']);
const SKIP_DIRS = new Set(['.git', 'node_modules', 'dist', 'dist-npm', '.next', 'build', 'coverage', '.cache']);
const MAX_IMAGE_RESULTS = 100;
const MAX_IMAGE_DEPTH = 6;
const MAX_CANDIDATE_FILE_BYTES = 1024 * 1024;
const MAX_UX_FILE_BYTES = 256 * 1024;

function validHostSessionId(value) {
  return typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$/.test(value) ? value : 'flywheel-ui';
}

function insideDir(parent, child) {
  return child === parent || child.startsWith(parent + sep);
}

async function readDesignCandidates(taskDir) {
  if (typeof taskDir !== 'string' || !isAbsolute(taskDir) || taskDir.includes('\0')) {
    throw new ControlNotDispatchedError('design-candidates requires an absolute taskDir');
  }
  const resolvedTask = await realpath(taskDir);
  if (!(await stat(resolvedTask)).isDirectory()) {
    throw new ControlNotDispatchedError('taskDir must be an existing directory');
  }
  const candidatesPath = join(resolvedTask, 'design-candidates');
  let resolvedDir;
  try {
    resolvedDir = await realpath(candidatesPath);
  } catch {
    return { protocol: 1, ok: true, taskDir: resolvedTask, revisions: [] };
  }
  if (!insideDir(resolvedTask, resolvedDir) || !(await stat(resolvedDir)).isDirectory()) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, revisions: [] };
  }
  const entries = (await readdir(resolvedDir)).sort();
  const byRev = new Map();
  let count = 0;
  for (const name of entries) {
    const match = /^rev-(\d{6})-(primary\.md|secondary\.md|meta\.json)$/.exec(name);
    if (!match || count >= 30) continue;
    const filePath = await realpath(join(resolvedDir, name));
    if (!insideDir(resolvedDir, filePath)) continue;
    const info = await stat(filePath);
    if (!info.isFile() || info.size > MAX_CANDIDATE_FILE_BYTES) continue;
    count++;
    const rev = Number.parseInt(match[1], 10);
    const kind = match[2];
    const bucket = byRev.get(rev) ?? { revision: rev, primary: '', secondary: '', meta: null };
    const text = await readFile(filePath, 'utf8');
    if (kind === 'primary.md') bucket.primary = text;
    else if (kind === 'secondary.md') bucket.secondary = text;
    else if (kind === 'meta.json') {
      try { bucket.meta = JSON.parse(text); } catch { bucket.meta = null; }
    }
    byRev.set(rev, bucket);
  }
  const revisions = [...byRev.values()].sort((a, b) => a.revision - b.revision);
  return { protocol: 1, ok: true, taskDir: resolvedTask, revisions };
}

async function readUXReview(taskDir, packet) {
  if (typeof taskDir !== 'string' || !isAbsolute(taskDir) || taskDir.includes('\0')) {
    throw new ControlNotDispatchedError('ux-review requires an absolute taskDir');
  }
  if (typeof packet !== 'string' || !/^M\d+-T\d+$/.test(packet)) {
    throw new ControlNotDispatchedError('ux-review requires an exact packet ID');
  }
  const resolvedTask = await realpath(taskDir);
  const reportsPath = join(resolvedTask, 'reports');
  let resolvedReports;
  try {
    resolvedReports = await realpath(reportsPath);
  } catch {
    return { protocol: 1, ok: true, taskDir: resolvedTask, packet, uxReview: null };
  }
  if (!insideDir(resolvedTask, resolvedReports)) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, packet, uxReview: null };
  }
  const filePath = join(resolvedReports, `${packet}.ux-review.json`);
  let resolvedFile;
  try {
    resolvedFile = await realpath(filePath);
  } catch {
    return { protocol: 1, ok: true, taskDir: resolvedTask, packet, uxReview: null };
  }
  if (!insideDir(resolvedReports, resolvedFile)) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, packet, uxReview: null };
  }
  const info = await stat(resolvedFile);
  if (!info.isFile() || info.size > MAX_UX_FILE_BYTES) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, packet, uxReview: null };
  }
  const parsed = JSON.parse(await readFile(resolvedFile, 'utf8'));
  return { protocol: 1, ok: true, taskDir: resolvedTask, packet, uxReview: parsed };
}

async function openResumeInTerminal(harness, sessionId, cwd) {
  const info = buildResumeInfo(harness, sessionId, cwd);
  if (!info || info.kind !== 'cli' || !info.command) {
    throw new ControlNotDispatchedError('open-terminal requires a valid CLI harness and sessionId');
  }
  if (info.cwd) {
    const resolvedCwd = await realpath(info.cwd).catch(() => null);
    if (!resolvedCwd || !(await stat(resolvedCwd).catch(() => null))?.isDirectory()) {
      throw new ControlNotDispatchedError('open-terminal cwd must be an existing directory');
    }
  }
  if (process.platform !== 'darwin') {
    return { protocol: 1, ok: true, launched: false, command: info.command, providerLabel: info.providerLabel };
  }
  await new Promise((resolve, reject) => {
    execFile(
      'osascript',
      [
        '-e', 'on run argv',
        '-e', 'tell application "Terminal"',
        '-e', 'do script (item 1 of argv)',
        '-e', 'activate',
        '-e', 'end tell',
        '-e', 'end run',
        '--',
        info.command,
      ],
      { timeout: 10000 },
      error => (error ? reject(error) : resolve()),
    );
  });
  return { protocol: 1, ok: true, launched: true, command: info.command, providerLabel: info.providerLabel };
}

async function walkRepoImages(rootCwd, signal) {
  if (typeof rootCwd !== 'string' || !isAbsolute(rootCwd) || rootCwd.includes('\0')) {
    throw new ControlNotDispatchedError('listRepoImages requires an absolute directory path');
  }
  const root = await realpath(rootCwd);
  if (!(await stat(root)).isDirectory()) {
    throw new ControlNotDispatchedError('listRepoImages target must be a directory');
  }
  const files = [];
  const walk = async (dir, depth) => {
    if (signal?.aborted || depth > MAX_IMAGE_DEPTH || files.length >= MAX_IMAGE_RESULTS) return;
    let entries;
    try { entries = await readdir(dir, { withFileTypes: true }); } catch { return; }
    entries.sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      if (signal?.aborted || files.length >= MAX_IMAGE_RESULTS) break;
      if (entry.name.startsWith('.') && entry.name !== '.github') continue;
      if (SKIP_DIRS.has(entry.name)) continue;
      const full = join(dir, entry.name);
      let info;
      try { info = await lstat(full); } catch { continue; }
      if (info.isSymbolicLink()) continue;
      if (info.isDirectory()) {
        await walk(full, depth + 1);
      } else if (info.isFile()) {
        const ext = extname(entry.name).toLowerCase();
        if (IMAGE_EXTENSIONS.has(ext)) {
          const rel = relative(root, full).split(sep).join('/');
          files.push({ path: rel, bytes: info.size, ext: ext.slice(1) });
        }
      }
    }
  };
  await walk(root, 0);
  return { protocol: 1, ok: true, cwd: root, files };
}

const initializers = [];

// Direct Typert Remote service powering Flywheel UI v2 across all 4 native DSH
// slots without requiring an active chat conversation or LLM agent session.
// Model tools (flywheel_launch / flywheel_control) never have access to act().
export class ConsoleReadService extends Service {
  constructor(ctx, control, failure, defaultCwd) {
    super(ctx, 'flywheelConsole');
    this.typertRemote = bindTypertRemote(this, 'flywheelConsole');
    this.control = control;
    this.failure = failure;
    this.defaultCwd = defaultCwd;
    for (const initialize of initializers) initialize.call(this);
  }

  _signal(signal) {
    return signal ?? this.ctx.invocation?.peer?.signal;
  }

  async query(input, signal) {
    try {
      if (!input || typeof input !== 'object' || Array.isArray(input) || !consoleReadActions.has(input.action)) {
        throw new ControlNotDispatchedError('Unsupported Flywheel console query action');
      }
      if (input.action === 'planning' && (input.planning?.action !== 'status' || !validPlanningCommand(input.planning))) {
        throw new ControlNotDispatchedError('Console planning queries accept status only');
      }
      if (input.action === 'design-candidates') {
        const value = await readDesignCandidates(input.taskDir);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'ux-review') {
        const value = await readUXReview(input.taskDir, input.packet);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'traces') {
        const value = await readTaskTraces(input.taskDir);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'trace-detail') {
        const value = await readTaskTraceDetail(input.taskDir, input.traceId);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      const { hostSessionId, ...payload } = input;
      if (payload.cwd == null) payload.cwd = this.defaultCwd ?? process.cwd();
      const value = await this.control(payload, undefined, this._signal(signal), undefined, undefined, undefined, validHostSessionId(hostSessionId));
      return { result: { kind: 'success', text: JSON.stringify(value) } };
    } catch (error) {
      return { result: { kind: 'error', text: JSON.stringify(this.failure(input, error)) } };
    }
  }

  async act(input, signal) {
    try {
      if (!input || typeof input !== 'object' || Array.isArray(input) || !consoleActActions.has(input.action)) {
        throw new ControlNotDispatchedError('Unsupported Flywheel console mutation action');
      }
      if (input.action === 'open-terminal') {
        const value = await openResumeInTerminal(input.harness, input.sessionId, input.cwd);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      const { hostSessionId: rawSessionId, ...payload } = input;
      if (payload.cwd == null) payload.cwd = this.defaultCwd ?? process.cwd();
      const sessionId = validHostSessionId(rawSessionId);
      const commandId = 'ui-' + randomUUID();
      let humanPlanningAction;
      let humanSessionAction;
      if (payload.action === 'planning' && payload.planning?.action === 'approve') {
        if (!validPlanningCommand(payload.planning)) {
          throw new ControlNotDispatchedError('Design approval requires a valid planning command');
        }
        humanPlanningAction = {
          kind: 'dsh',
          sessionId,
          commandId,
          'request-id': payload.planning['request-id'],
          revision: payload.planning.revision,
          design: { ...payload.planning.design },
          start: payload.planning.start ?? false,
        };
      }
      if (payload.action === 'session' && payload.session?.action === 'abandon') {
        if (!validSessionAbandonCommand(payload.session) || typeof payload.taskDir !== 'string' || !payload.taskDir) {
          throw new ControlNotDispatchedError('Session abandonment requires a valid session command and taskDir');
        }
        humanSessionAction = {
          kind: 'dsh',
          sessionId,
          commandId,
          taskDir: payload.taskDir,
          key: payload.session.key,
          invocation: payload.session.invocation,
          revision: payload.session.revision,
          action: 'abandon',
          reason: payload.session.reason,
        };
      }
      const value = await this.control(payload, undefined, this._signal(signal), 'ui:' + commandId, humanPlanningAction, humanSessionAction, sessionId);
      return { result: { kind: 'success', text: JSON.stringify(value) } };
    } catch (error) {
      return { result: { kind: 'error', text: JSON.stringify(this.failure(input, error)) } };
    }
  }

  async listRepoImages(cwd, signal) {
    try {
      const value = await walkRepoImages(cwd ?? this.defaultCwd ?? process.cwd(), this._signal(signal));
      return { result: { kind: 'success', text: JSON.stringify(value) } };
    } catch (error) {
      return { result: { kind: 'error', text: JSON.stringify(this.failure({ action: 'images' }, error)) } };
    }
  }
}

for (const name of ['query', 'act', 'listRepoImages']) {
  Remote(ConsoleReadService.prototype[name], {
    kind: 'method',
    name,
    static: false,
    private: false,
    addInitializer: initialize => initializers.push(initialize),
  });
}
