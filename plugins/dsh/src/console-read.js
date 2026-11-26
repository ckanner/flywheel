import { randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import { watch } from 'node:fs';
import { access, lstat, mkdir, readdir, readFile, realpath, stat, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { extname, isAbsolute, join, relative, sep } from 'node:path';
import { Service } from '@deepseek-ai/cordis';
import { Remote, bindTypertRemote } from '@deepseek-ai/dsh-typert-protocol';
import { ControlNotDispatchedError } from './control.js';
import { validPlanningCommand } from './planning-control.js';
import { validSessionAbandonCommand } from './session-control.js';
import { buildResumeInfo, readTaskTraceDetail, readTaskTraces, unwrapStreamingContractMarkdown } from './trace-parser.js';

export const consoleReadActions = new Set([
  'models', 'images', 'runs', 'status', 'config', 'config-edit',
  'metrics', 'history', 'node', 'delivery', 'doctor', 'planning',
  'design-candidates', 'ux-review', 'ui-prototype', 'read-artifact', 'traces', 'trace-detail',
]);

export const consoleActActions = new Set([
  'planning', 'investigate', 'launch', 'start', 'stop', 'step',
  'config-save', 'session', 'deliver', 'open-terminal', 'open-artifact', 'save-attachment',
]);

const IMAGE_EXTENSIONS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.gif']);
const SKIP_DIRS = new Set(['.git', 'node_modules', 'dist', 'dist-npm', '.next', 'build', 'coverage', '.cache']);
const MAX_IMAGE_RESULTS = 100;
const MAX_IMAGE_DEPTH = 6;
const MAX_ATTACHMENT_BYTES = 4 * 1024 * 1024;
const MAX_CANDIDATE_FILE_BYTES = 1024 * 1024;
const MAX_UX_FILE_BYTES = 512 * 1024;

function detectImageType(buf) {
  if (!Buffer.isBuffer(buf) || buf.length < 12) return null;
  if (buf[0] === 0x89 && buf[1] === 0x50 && buf[2] === 0x4e && buf[3] === 0x47) {
    return { mimeType: 'image/png', ext: '.png' };
  }
  if (buf[0] === 0xff && buf[1] === 0xd8 && buf[2] === 0xff) {
    return { mimeType: 'image/jpeg', ext: '.jpg' };
  }
  if (buf[0] === 0x47 && buf[1] === 0x49 && buf[2] === 0x46 && buf[3] === 0x38) {
    return { mimeType: 'image/gif', ext: '.gif' };
  }
  if (
    buf[0] === 0x52 && buf[1] === 0x49 && buf[2] === 0x46 && buf[3] === 0x46 &&
    buf[8] === 0x57 && buf[9] === 0x45 && buf[10] === 0x42 && buf[11] === 0x50
  ) {
    return { mimeType: 'image/webp', ext: '.webp' };
  }
  return null;
}

async function saveLocalAttachment(rootCwd, name, dataBase64) {
  if (typeof rootCwd !== 'string' || !isAbsolute(rootCwd) || rootCwd.includes('\0')) {
    throw new ControlNotDispatchedError('save-attachment requires an absolute cwd');
  }
  if (typeof dataBase64 !== 'string' || !dataBase64.trim()) {
    throw new ControlNotDispatchedError('save-attachment requires base64 image data');
  }
  const cleanB64 = dataBase64.replace(/^data:[^;]+;base64,/, '').replace(/\s+/g, '');
  if (cleanB64.length > Math.ceil(MAX_ATTACHMENT_BYTES / 3) * 4 + 16) {
    throw new ControlNotDispatchedError('Image attachment exceeds 4 MiB limit');
  }
  const bytes = Buffer.from(cleanB64, 'base64');
  if (bytes.length === 0 || bytes.length > MAX_ATTACHMENT_BYTES) {
    throw new ControlNotDispatchedError('Image attachment exceeds 4 MiB limit');
  }
  const detected = detectImageType(bytes);
  if (!detected) {
    throw new ControlNotDispatchedError('Unsupported image format; only PNG, JPEG, GIF, and WebP are allowed');
  }
  const root = await realpath(rootCwd);
  if (!(await stat(root)).isDirectory()) {
    throw new ControlNotDispatchedError('save-attachment target cwd must be a directory');
  }
  const attachDir = join(root, '.flywheel-attachments');
  await mkdir(attachDir, { recursive: true, mode: 0o755 });
  const resolvedAttachDir = await realpath(attachDir);
  if (!insideDir(root, resolvedAttachDir)) {
    throw new ControlNotDispatchedError('Attachment directory escapes workspace root');
  }
  const rawStem = typeof name === 'string' ? name.replace(/\.[^.]+$/, '') : 'image';
  const safeStem = rawStem.replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 48) || 'image';
  const stamp = new Date().toISOString().replace(/[-:T.Z]/g, '').slice(0, 14);
  const fileName = `${stamp}-${randomUUID().slice(0, 6)}-${safeStem}${detected.ext}`;
  const fullPath = join(resolvedAttachDir, fileName);
  await writeFile(fullPath, bytes, { mode: 0o644 });
  const rel = relative(root, fullPath).split(sep).join('/');
  return { protocol: 1, ok: true, cwd: root, path: rel, bytes: bytes.length, mimeType: detected.mimeType };
}

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
    if (kind === 'primary.md') bucket.primary = unwrapStreamingContractMarkdown(text)?.markdown ?? text;
    else if (kind === 'secondary.md') bucket.secondary = unwrapStreamingContractMarkdown(text)?.markdown ?? text;
    else if (kind === 'meta.json') {
      try { bucket.meta = JSON.parse(text); } catch { bucket.meta = null; }
    }
    byRev.set(rev, bucket);
  }
  const revisions = [...byRev.values()].sort((a, b) => a.revision - b.revision);
  return { protocol: 1, ok: true, taskDir: resolvedTask, revisions };
}

function runGitQuiet(cwd, args) {
  return new Promise((resolve, reject) => {
    execFile('git', args, { cwd, timeout: 5000, maxBuffer: 512 * 1024 }, (err, stdout) => {
      if (err) reject(err);
      else resolve(String(stdout ?? '').trim());
    });
  });
}

async function readLocalDeliveryFallback(rootCwd, taskDir, remoteError) {
  const cwd = await realpath(rootCwd).catch(() => null);
  if (!cwd) throw remoteError;
  const head = await runGitQuiet(cwd, ['rev-parse', 'HEAD']).catch(() => '');
  if (!head) throw remoteError;
  const localBranch = await runGitQuiet(cwd, ['symbolic-ref', '--quiet', '--short', 'HEAD']).catch(() => 'HEAD');
  const origin = await runGitQuiet(cwd, ['remote', 'get-url', 'origin']).catch(() => '');
  const statusOut = await runGitQuiet(cwd, ['status', '--porcelain']).catch(() => '');
  const changedFiles = statusOut ? statusOut.split('\n').map(l => l.trim()).filter(Boolean) : [];
  const logOut = await runGitQuiet(cwd, ['log', '-n', '8', '--pretty=format:%h\t%s\t%cI']).catch(() => '');
  const recentCommits = logOut
    ? logOut.split('\n').filter(Boolean).map(line => {
        const [sha, subject, at] = line.split('\t');
        return { sha: sha || '', subject: subject || '', ...(at ? { at } : {}) };
      })
    : [];
  const remoteStatus = !origin ? 'no-origin' : 'gh-unavailable';
  const remoteMessage = !origin
    ? '当前为纯本地 Git 仓库，未配置 GitHub 远程地址（origin）；以下展示本地交付清单。'
    : '无法连接 GitHub 远程或 gh CLI 未登录；以下展示本地交付清单。';
  return {
    protocol: 1,
    ok: true,
    taskDir,
    cwd,
    delivery: {
      mode: 'local',
      remoteStatus,
      remoteMessage,
      endpoint: 'local',
      localBranch,
      branch: localBranch,
      head,
      workingTreeClean: changedFiles.length === 0,
      recentCommits,
      changedFiles,
      pulls: [],
    },
  };
}

async function readUIPrototype(taskDir, htmlFile) {
  if (typeof taskDir !== 'string' || !isAbsolute(taskDir) || taskDir.includes('\0')) {
    throw new ControlNotDispatchedError('ui-prototype requires an absolute taskDir');
  }
  const resolvedTask = await realpath(taskDir);
  const protoDir = await realpath(join(resolvedTask, 'ui-prototypes')).catch(() => null);
  if (!protoDir || !insideDir(resolvedTask, protoDir)) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, html: '', htmlFile: '' };
  }
  let targetRel = typeof htmlFile === 'string' && htmlFile.trim() ? htmlFile.trim() : '';
  if (!targetRel) {
    const names = (await readdir(protoDir).catch(() => [])).filter(n => /^ui-prototype-\d{6}\.html$/.test(n)).sort();
    if (names.length === 0) {
      return { protocol: 1, ok: true, taskDir: resolvedTask, html: '', htmlFile: '' };
    }
    targetRel = `ui-prototypes/${names[names.length - 1]}`;
  }
  if (!/^ui-prototypes\/ui-prototype-\d{6}\.html$/.test(targetRel)) {
    throw new ControlNotDispatchedError('ui-prototype htmlFile must reference a ui-prototypes HTML file');
  }
  const resolvedFile = await realpath(join(resolvedTask, targetRel)).catch(() => null);
  if (!resolvedFile || !insideDir(protoDir, resolvedFile)) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, html: '', htmlFile: targetRel };
  }
  const info = await stat(resolvedFile).catch(() => null);
  if (!info || !info.isFile() || info.size > MAX_UX_FILE_BYTES) {
    return { protocol: 1, ok: true, taskDir: resolvedTask, html: '', htmlFile: targetRel };
  }
  const html = await readFile(resolvedFile, 'utf8');
  return { protocol: 1, ok: true, taskDir: resolvedTask, html, htmlFile: targetRel };
}

function classifyArtifactKind(relPath) {
  const ext = extname(relPath).toLowerCase();
  if (ext === '.html' || ext === '.htm') return 'html';
  if (ext === '.md' || ext === '.markdown') return 'markdown';
  if (ext === '.json' || ext === '.jsonl') return 'json';
  if (['.ts', '.tsx', '.js', '.jsx', '.mjs', '.css', '.go', '.py', '.sh', '.yml', '.yaml', '.toml'].includes(ext)) return 'code';
  return 'text';
}

async function resolveConfinedArtifact(taskDir, relPath, cwd) {
  if (typeof relPath !== 'string' || !relPath.trim() || relPath.includes('\0') || isAbsolute(relPath)) {
    throw new ControlNotDispatchedError('artifact path must be a non-empty relative path');
  }
  const cleanRel = relPath.trim().replace(/^\/+/, '');
  const roots = [];
  if (typeof taskDir === 'string' && isAbsolute(taskDir) && !taskDir.includes('\0')) {
    const resolvedTask = await realpath(taskDir).catch(() => null);
    if (resolvedTask) roots.push(resolvedTask);
  }
  if (typeof cwd === 'string' && isAbsolute(cwd) && !cwd.includes('\0')) {
    const resolvedCwd = await realpath(cwd).catch(() => null);
    if (resolvedCwd && !roots.includes(resolvedCwd)) roots.push(resolvedCwd);
  }
  if (roots.length === 0) {
    throw new ControlNotDispatchedError('artifact access requires an existing taskDir or cwd');
  }
  for (const root of roots) {
    const candidate = await realpath(join(root, cleanRel)).catch(() => null);
    if (candidate && insideDir(root, candidate)) {
      const info = await stat(candidate).catch(() => null);
      if (info?.isFile()) {
        return { resolvedFile: candidate, root, relPath: cleanRel, size: info.size };
      }
    }
  }
  throw new ControlNotDispatchedError(`artifact not found within task or workspace: ${cleanRel}`);
}

async function readArtifactFile(taskDir, relPath, cwd) {
  const { resolvedFile, relPath: cleanRel, size } = await resolveConfinedArtifact(taskDir, relPath, cwd);
  if (size > MAX_CANDIDATE_FILE_BYTES) {
    throw new ControlNotDispatchedError(`artifact exceeds ${MAX_CANDIDATE_FILE_BYTES} byte preview limit`);
  }
  const content = await readFile(resolvedFile, 'utf8');
  return {
    protocol: 1,
    ok: true,
    relPath: cleanRel,
    absPath: resolvedFile,
    kind: classifyArtifactKind(cleanRel),
    content,
    bytes: size,
  };
}

async function openArtifactFile(taskDir, relPath, cwd) {
  const { resolvedFile, relPath: cleanRel } = await resolveConfinedArtifact(taskDir, relPath, cwd);
  const opener = process.platform === 'darwin' ? 'open' : process.platform === 'linux' ? 'xdg-open' : '';
  if (!opener) {
    return { protocol: 1, ok: true, opened: false, absPath: resolvedFile, relPath: cleanRel };
  }
  await new Promise((resolve, reject) => {
    execFile(opener, [resolvedFile], { timeout: 10000 }, error => (error ? reject(error) : resolve()));
  });
  return { protocol: 1, ok: true, opened: true, absPath: resolvedFile, relPath: cleanRel };
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

function yamlString(value) {
  return JSON.stringify(String(value ?? ''));
}

async function hasWarpApp() {
  try {
    await access('/Applications/Warp.app');
    return true;
  } catch {
    return false;
  }
}

async function openResumeInTerminal(harness, sessionId, cwd, sessionTitle, preferredTerminal) {
  const info = buildResumeInfo(harness, sessionId, cwd);
  if (!info || info.kind !== 'cli' || !info.command) {
    throw new ControlNotDispatchedError('open-terminal requires a valid CLI harness and sessionId');
  }
  let resolvedCwd = info.cwd || '';
  if (resolvedCwd) {
    resolvedCwd = await realpath(resolvedCwd).catch(() => '');
    if (!resolvedCwd || !(await stat(resolvedCwd).catch(() => null))?.isDirectory()) {
      throw new ControlNotDispatchedError('open-terminal cwd must be an existing directory');
    }
  }
  if (process.platform !== 'darwin') {
    return { protocol: 1, ok: true, launched: false, terminal: 'none', command: info.command, providerLabel: info.providerLabel };
  }
  const useWarp =
    preferredTerminal === 'warp' ||
    (preferredTerminal !== 'terminal' && (await hasWarpApp()));
  if (useWarp) {
    const cleanTitle =
      typeof sessionTitle === 'string' && sessionTitle.trim()
        ? sessionTitle.trim().slice(0, 80)
        : `Flywheel · ${info.providerLabel}`;
    // Strip leading `cd '...' && ` when Warp sets tab cwd natively; keep full command if no cwd.
    const execCmd = resolvedCwd && info.command.includes(' && ')
      ? info.command.slice(info.command.indexOf(' && ') + 4)
      : info.command;
    const launchFile = join(tmpdir(), `flywheel-warp-resume-${randomUUID().slice(0, 8)}.yaml`);
    const yaml = [
      '---',
      `name: ${yamlString(cleanTitle)}`,
      'active_window_index: 0',
      'windows:',
      '  - tabs:',
      `      - title: ${yamlString(cleanTitle)}`,
      '        layout:',
      `          cwd: ${yamlString(resolvedCwd || process.cwd())}`,
      '          commands:',
      `            - exec: ${yamlString(execCmd)}`,
      '',
    ].join('\n');
    await writeFile(launchFile, yaml, { mode: 0o600 });
    await new Promise((resolve, reject) => {
      execFile('open', [`warp://launch/${encodeURI(launchFile)}`], { timeout: 10000 }, error =>
        error ? reject(error) : resolve(),
      );
    });
    return { protocol: 1, ok: true, launched: true, terminal: 'warp', command: info.command, providerLabel: info.providerLabel };
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
  return { protocol: 1, ok: true, launched: true, terminal: 'terminal', command: info.command, providerLabel: info.providerLabel };
}

async function waitForLiveTraceGrowth(taskDir, traceId, sinceBytes, signal) {
  if (typeof sinceBytes !== 'number' || !Number.isFinite(sinceBytes) || sinceBytes < 0) return;
  if (typeof taskDir !== 'string' || !isAbsolute(taskDir) || typeof traceId !== 'string' || !/^[A-Za-z0-9_.-]{1,200}$/.test(traceId)) {
    return;
  }
  const resolvedTask = await realpath(taskDir).catch(() => null);
  if (!resolvedTask) return;
  const tracesDir = join(resolvedTask, '.traces');
  const fileName = `live-${traceId}.jsonl`;
  const filePath = join(tracesDir, fileName);
  const current = await stat(filePath).catch(() => null);
  if (!current || current.size !== sinceBytes) return;

  await new Promise(resolve => {
    let settled = false;
    let watcher = null;
    const finish = () => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      signal?.removeEventListener('abort', finish);
      try { watcher?.close(); } catch { /* ignore */ }
      resolve();
    };
    const timer = setTimeout(finish, 1500);
    signal?.addEventListener('abort', finish, { once: true });
    try {
      watcher = watch(tracesDir, (_event, changed) => {
        if (changed && changed !== fileName) return;
        void stat(filePath).then(
          info => {
            if (info.size !== sinceBytes) finish();
          },
          () => finish(),
        );
      });
      watcher.on('error', finish);
    } catch {
      finish();
    }
  });
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
      if (entry.name.startsWith('.') && entry.name !== '.github' && entry.name !== '.flywheel-attachments') continue;
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
      if (input.action === 'ui-prototype') {
        const value = await readUIPrototype(input.taskDir, input.htmlFile);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'read-artifact') {
        const value = await readArtifactFile(input.taskDir, input.relPath, input.cwd ?? this.defaultCwd);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'traces') {
        const value = await readTaskTraces(input.taskDir);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'trace-detail') {
        if (typeof input.waitSinceBytes === 'number') {
          await waitForLiveTraceGrowth(input.taskDir, input.traceId, input.waitSinceBytes, this._signal(signal));
        }
        const value = await readTaskTraceDetail(input.taskDir, input.traceId);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      const { hostSessionId, ...payload } = input;
      if (payload.cwd == null) payload.cwd = this.defaultCwd ?? process.cwd();
      try {
        const value = await this.control(payload, undefined, this._signal(signal), undefined, undefined, undefined, validHostSessionId(hostSessionId));
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      } catch (controlErr) {
        if (payload.action === 'delivery') {
          try {
            const localValue = await readLocalDeliveryFallback(payload.cwd, payload.taskDir, controlErr);
            return { result: { kind: 'success', text: JSON.stringify(localValue) } };
          } catch {
            // Fall through to original control error.
          }
        }
        throw controlErr;
      }
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
        const value = await openResumeInTerminal(input.harness, input.sessionId, input.cwd, input.sessionTitle, input.terminal);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'open-artifact') {
        const value = await openArtifactFile(input.taskDir, input.relPath, input.cwd ?? this.defaultCwd);
        return { result: { kind: 'success', text: JSON.stringify(value) } };
      }
      if (input.action === 'save-attachment') {
        const value = await saveLocalAttachment(input.cwd ?? this.defaultCwd ?? process.cwd(), input.name, input.dataBase64);
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
