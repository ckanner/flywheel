// Trusted workspace transport. Tower receives a separate .git; Go still owns
// validation, protected-path decisions, final commits and task acceptance.
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, mkdir, lstat, readFile, writeFile, readlink, symlink, chmod, readdir, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';

const execute = promisify(execFile);
const sha = value => createHash('sha256').update(value).digest('hex');
const MAX_BYTES = 256 * 1024 * 1024;

function fail(message) { return Object.assign(new Error(message), { code: 'INVALID_REQUEST' }); }

export async function towerGit(cwd, args) {
  // A host shell may itself be inside a temporary index/worktree operation.
  // Those variables must not redirect the trusted copy or transfer commands.
  const inherited = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('GIT_')));
  const { stdout } = await execute('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'core.autocrlf=false', ...args], {
    cwd, encoding: 'buffer', maxBuffer: MAX_BYTES,
    env: { ...inherited, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: '/dev/null',
      GIT_AUTHOR_NAME: 'Flywheel', GIT_AUTHOR_EMAIL: 'flywheel@localhost',
      GIT_COMMITTER_NAME: 'Flywheel', GIT_COMMITTER_EMAIL: 'flywheel@localhost' },
  });
  return stdout;
}

/** Only the actual task's changing engine records are outside the candidate. */
function runtimeTaskPath(name) {
  if (['.sessions/', '.control/', 'reports/native/'].some(prefix => name.startsWith(prefix))) return true;
  return /^(?:metrics\.jsonl|orchestrator\.(?:log|pid)|\.state\.lock|\.run\.lock|\.stop-request)$/.test(name)
    || /^state\.json(?:\.tmp(?:[.-].*)?)?$/.test(name);
}

export function towerExcluded(path, cwd, taskDir) {
  const parts = path.split('/');
  if (parts.includes('.git') || parts[0] === '.tower') return true;
  const ledger = relative(cwd, taskDir).split(sep).join('/');
  if (ledger === '..' || ledger.startsWith('../') || isAbsolute(ledger) || (ledger && !path.startsWith(ledger + '/'))) return false;
  return runtimeTaskPath(ledger ? path.slice(ledger.length + 1) : path);
}

async function externalTaskSnapshot(taskDir, byteLimit) {
  const entries = []; let total = 0;
  const walk = async (directory, prefix = '') => {
    for (const name of (await readdir(directory)).sort()) {
      const path = prefix + name, absolute = join(directory, name);
      // Skip known runtime names before lstat: an atomic state/session rename
      // may remove a temporary entry between readdir and metadata inspection.
      if (runtimeTaskPath(path) || runtimeTaskPath(path + '/')) continue;
      const stat = await lstat(absolute);
      if (stat.isDirectory()) await walk(absolute, path + '/');
      else if (stat.isFile()) {
        total += stat.size;
        if (total > byteLimit) throw fail('Tower source and external contract exceed the 256 MiB snapshot limit');
        entries.push({ path, kind: 'file', executable: Boolean(stat.mode & 0o111), digest: sha(await readFile(absolute)) });
      } else {
        // A frozen external contract cannot be faithfully snapshotted by
        // following an unconstrained symlink tree or a special-file stream.
        throw fail('Tower external task contracts must contain regular files and directories');
      }
    }
  };
  await walk(taskDir);
  return { path: resolve(taskDir), entries, digest: sha(JSON.stringify(entries)) };
}

async function noSymlinkAncestor(cwd, path) {
  let cursor = cwd;
  for (const segment of path.split('/').slice(0, -1)) {
    cursor = join(cursor, segment);
    try { if ((await lstat(cursor)).isSymbolicLink()) throw fail('Tower input traverses a symlink directory'); }
    catch (error) { if (error.code === 'ENOENT') return; throw error; }
  }
}

/** HEAD plus exact tracked/unignored bytes, deletions, executable bits and links. */
export async function towerSnapshot(cwd, taskDir, { externalContract = true } = {}) {
  cwd = resolve(cwd);
  const head = (await towerGit(cwd, ['rev-parse', '--verify', 'HEAD'])).toString().trim();
  const top = (await towerGit(cwd, ['rev-parse', '--show-toplevel'])).toString().trim();
  if (resolve(top) !== cwd) throw fail('Tower requires the repository root as its execution directory');
  const paths = [...new Set((await towerGit(cwd, ['ls-files', '-c', '-o', '--exclude-standard', '-z'])).toString().split('\0').filter(Boolean))].sort();
  const entries = [];
  let total = 0;
  for (const path of paths) {
    if (isAbsolute(path) || path.split('/').some(part => part === '..' || part === '')) throw fail('Invalid Tower source path');
    if (towerExcluded(path, cwd, taskDir)) continue;
    await noSymlinkAncestor(cwd, path);
    let stat;
    try { stat = await lstat(join(cwd, path)); }
    catch (error) { if (error.code === 'ENOENT') { entries.push({ path, kind: 'missing' }); continue; } throw error; }
    if (stat.isSymbolicLink()) {
      entries.push({ path, kind: 'link', target: await readlink(join(cwd, path)) });
    } else if (stat.isFile()) {
      total += stat.size;
      if (total > MAX_BYTES) throw fail('Tower source exceeds the 256 MiB tracked/unignored snapshot limit');
      entries.push({ path, kind: 'file', executable: Boolean(stat.mode & 0o111), digest: sha(await readFile(join(cwd, path))) });
    } else {
      throw fail('Tower input contains a submodule, directory entry or unsupported special file');
    }
  }
  const location = relative(cwd, taskDir);
  const outside = location === '..' || location.startsWith('..' + sep) || isAbsolute(location);
  const contract = externalContract && outside ? await externalTaskSnapshot(taskDir, MAX_BYTES - total) : undefined;
  return { version: 1, head, entries, ...(contract ? { externalContract: contract } : {}), digest: sha(JSON.stringify({ head, entries, externalContract: contract })) };
}

export async function createTowerCandidate(input) {
  const source = await towerSnapshot(input.cwd, input.managed.taskDir);
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'flywheel-kimi-tower-')));
  await chmod(directory, 0o700);
  const path = join(directory, 'repository');
  const branch = input.native.tower?.branch ?? `flywheel-tower-${sha(input.requestId).slice(0, 16)}`;
  await towerGit(input.cwd, ['check-ref-format', '--branch', branch]);
  // --no-local and --no-hardlinks avoid shared objects/admin data even when the
  // source itself is a linked worktree. Nothing writes the source's .git.
  await towerGit(input.cwd, ['clone', '--quiet', '--no-local', '--no-hardlinks', '--no-checkout', '--', input.cwd, path]);
  await towerGit(path, ['remote', 'remove', 'origin']);
  await towerGit(path, ['reset', '--mixed', source.head]);
  for (const entry of source.entries) {
    if (entry.kind === 'missing') continue;
    const target = join(path, entry.path);
    await mkdir(dirname(target), { recursive: true });
    if (entry.kind === 'link') await symlink(entry.target, target);
    else {
      await writeFile(target, await readFile(join(input.cwd, entry.path)));
      await chmod(target, entry.executable ? 0o755 : 0o644);
    }
  }
  if ((await towerSnapshot(input.cwd, input.managed.taskDir)).digest !== source.digest) {
    throw fail(`Tower source changed while copying; partial candidate retained at ${path}`);
  }
  await towerGit(path, ['add', '-A', '--', '.']);
  await towerGit(path, ['commit', '--quiet', '--allow-empty', '-m', 'Snapshot Flywheel candidate for isolated Tower execution']);
  const baseHead = (await towerGit(path, ['rev-parse', 'HEAD'])).toString().trim();
  await towerGit(path, ['checkout', '-B', branch]);
  const candidate = { kind: 'tower-repository', path, branch, baseHead, sourceHead: source.head,
    sourceSnapshotDigest: source.digest, patchPath: join(directory, 'candidate.patch'), source };
  await writeFile(join(directory, 'source-snapshot.json'), JSON.stringify(source), { mode: 0o600 });
  return candidate;
}

/** Freeze the candidate only after the owned runtime is quiescent. */
export async function freezeTowerCandidate(candidate, input) {
  // Exclude Tower's administrative worktrees/comms, while retaining ordinary
  // contract edits so Go's existing boundary guard can reject them explicitly.
  const paths = (await towerGit(candidate.path, ['ls-files', '-c', '-o', '--exclude-standard', '-z'])).toString().split('\0').filter(Boolean);
  const excluded = [...new Set(paths.filter(path => towerExcluded(path, input.cwd, input.managed.taskDir)))];
  await towerGit(candidate.path, ['add', '-A', '--', '.', ':(exclude).tower', ...excluded.map(path => `:(exclude,literal)${path}`)]);
  const patch = await towerGit(candidate.path, ['diff', '--cached', '--binary', '--full-index', candidate.baseHead, '--', '.', ':(exclude).tower', ...excluded.map(path => `:(exclude,literal)${path}`)]);
  await writeFile(candidate.patchPath, patch, { mode: 0o600 });
  const tree = (await towerGit(candidate.path, ['write-tree'])).toString().trim();
  const snapshot = await towerSnapshot(candidate.path, join(candidate.path, relative(input.cwd, input.managed.taskDir)), { externalContract: false });
  return { ...candidate, tree, patchDigest: sha(patch), snapshot };
}

/** Apply no candidate after uncertainty, to another role, or to a changed source. */
export async function transferTowerCandidate(candidate, input, cleanupConfirmed) {
  if (!cleanupConfirmed) throw fail('Tower candidate transfer requires confirmed runtime cleanup');
  if (input.managed.role !== 'execute') return { applied: false, reason: 'role-does-not-own-workspace-transfer' };
  const current = await towerSnapshot(input.cwd, input.managed.taskDir);
  if (current.digest !== candidate.sourceSnapshotDigest) throw fail('Tower source or contract changed; candidate retained without transfer');
  const patch = await readFile(candidate.patchPath);
  if (sha(patch) !== candidate.patchDigest) throw fail('Tower candidate patch changed after freezing');
  if (patch.length === 0) return { applied: true, empty: true, sourceSnapshotDigest: current.digest, afterDigest: current.digest };
  await towerGit(input.cwd, ['apply', '--check', '--binary', '--', candidate.patchPath]);
  // A second read catches edits made while checking the patch. There is no
  // claim of an OS-wide lock over unrelated editors; the postcondition below
  // also rejects a concurrent write instead of reporting a matching candidate.
  if ((await towerSnapshot(input.cwd, input.managed.taskDir)).digest !== current.digest) throw fail('Tower source changed during transfer admission');
  await towerGit(input.cwd, ['apply', '--binary', '--', candidate.patchPath]);
  const after = await towerSnapshot(input.cwd, input.managed.taskDir);
  const existing = entries => entries.filter(entry => entry.kind !== 'missing');
  if (after.head !== current.head || after.externalContract?.digest !== candidate.source.externalContract?.digest
      || JSON.stringify(existing(after.entries)) !== JSON.stringify(existing(candidate.snapshot.entries))) {
    throw fail('Tower transfer postcondition differs from the frozen candidate; artifacts retained');
  }
  return { applied: true, empty: false, sourceSnapshotDigest: current.digest, afterDigest: after.digest };
}

export function towerCandidateReceipt(candidate, transfer) {
  const { kind, path, branch, baseHead, sourceHead, sourceSnapshotDigest, patchPath, patchDigest, tree } = candidate;
  return { kind, path, branch, baseHead, sourceHead, sourceSnapshotDigest, patchPath,
    ...(candidate.source.externalContract ? { externalContractDigest: candidate.source.externalContract.digest } : {}),
    ...(patchDigest ? { patchDigest, tree } : {}), ...(transfer ? { transfer } : {}) };
}

/** Remap exact project path references, never substrings in another path/URL. */
export function towerPrompt(input, candidate) {
  const source = input.cwd, target = candidate.path;
  let cursor = 0, mapped = '', replacements = 0;
  for (;;) {
    const index = input.prompt.indexOf(source, cursor);
    if (index < 0) { mapped += input.prompt.slice(cursor); break; }
    const before = index === 0 ? '' : input.prompt[index - 1];
    const after = input.prompt[index + source.length] || '';
    const startsPath = before === '' || /[\s"'`([{=:]/.test(before);
    const endsPrefix = after === '' || /[\/\s"'`)\]},;:#?]/.test(after);
    mapped += input.prompt.slice(cursor, index) + (startsPath && endsPrefix ? target : source);
    if (startsPath && endsPrefix) replacements++;
    cursor = index + source.length;
  }
  const prompt = `Flywheel Tower candidate directory: ${target}\n` +
    'Run project edits and Git operations in this independent repository. External task contract files are read-only evidence at their supplied paths. ' +
    'Flywheel transfers this candidate after cleanup and owns validation, review and final commits.\n\n' + mapped;
  return { prompt, mapping: { sourceCwd: source, candidateCwd: target, replacements,
    rule: 'bounded-literal-absolute-path-prefix', originalPromptDigest: sha(input.prompt), effectivePromptDigest: sha(prompt), filesystemSandbox: false } };
}
