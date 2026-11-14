import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const root = process.cwd();
const vendor = join(root, 'plugins/dsh/vendor');
const hash = (bytes, algorithm = 'sha256', encoding = 'hex') => createHash(algorithm).update(bytes).digest(encoding);
const run = (cmd, args, cwd) => execFileSync(cmd, args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'], maxBuffer: 32 * 1024 * 1024 });
const specs = [
  { directory: '_companion/codex', package: 'dsh-subagent-codex-pro', version: '0.10.0', commit: process.env.CODEX_SOURCE_SHA, pin: 'codex-pro.json' },
  { directory: '_companion/claude', package: 'dsh-subagent-claude-code-pro', version: '0.5.0', commit: process.env.CLAUDE_SOURCE_SHA, pin: 'claude-code-pro.json' },
];
if (process.versions.node !== '24.22.1' || run('npm', ['--version'], root).trim() !== '12.2.1') throw new Error('Wrong release toolchain');
for (const item of specs) {
  if (!/^[a-f0-9]{40}$/.test(item.commit || '')) throw new Error('An immutable source commit is required');
  const cwd = join(root, item.directory);
  const actual = run('git', ['rev-parse', 'HEAD'], cwd).trim();
  if (actual !== item.commit) throw new Error('Wrong companion source commit');
  const tree = run('git', ['rev-parse', 'HEAD^{tree}'], cwd).trim();
  if (existsSync(join(cwd, 'package-lock.json'))) process.stdout.write(run('npm', ['ci', '--ignore-scripts', '--no-audit', '--no-fund'], cwd));
  process.stdout.write(run('npm', ['run', 'build', '--if-present'], cwd));
  const manifest = JSON.parse(readFileSync(join(cwd, 'package.json'), 'utf8'));
  if (manifest.name !== item.package || manifest.version !== item.version || manifest.license !== 'MIT') throw new Error('Unexpected companion identity');
  const packed = JSON.parse(run('npm', ['pack', '--ignore-scripts', '--json'], cwd));
  const pack = Array.isArray(packed) ? packed[0] : packed[item.package];
  if (!pack || pack.name !== item.package || pack.version !== item.version) throw new Error('Unexpected npm pack output');
  const tarball = readFileSync(join(cwd, pack.filename));
  const integrity = 'sha512-' + hash(tarball, 'sha512', 'base64');
  if (pack.integrity !== integrity) throw new Error('npm integrity does not identify the packed bytes');
  copyFileSync(join(cwd, pack.filename), join(vendor, pack.filename));
  const pin = {
    package: item.package, version: item.version,
    repository: 'https://github.com/ckanner/' + item.package,
    sourceCommit: item.commit, sourceTree: tree, tarball: pack.filename,
    sha256: hash(tarball), integrity, license: manifest.license,
    registryReleaseVerified: false, toolchain: { node: process.versions.node, npm: '12.2.1' },
  };
  writeFileSync(join(vendor, item.pin), JSON.stringify(pin, null, 2) + '\n');
  item.tarball = pack.filename;
  item.manifest = manifest;
  item.pinValue = pin;
  console.log('COMPANION_PACK_METADATA ' + JSON.stringify({ ...pin, fileCount: pack.files.length, size: pack.size }));
}

// Resolve real package metadata from the newly packed files. Keep the public
// exact-version declarations while pinning the reviewed unpublished bytes in
// the lock, then prove the restored manifests with ordinary npm ci.
for (const relative of ['.', 'plugins/dsh']) {
  const cwd = resolve(root, relative);
  const filename = join(cwd, 'package.json');
  const original = readFileSync(filename, 'utf8');
  const manifest = JSON.parse(original);
  const temporary = structuredClone(manifest);
  const prefix = relative === '.' ? 'plugins/dsh/vendor/' : 'vendor/';
  temporary.dependencies['dsh-subagent-codex-pro'] = 'file:' + prefix + specs[0].tarball;
  temporary.dependencies['dsh-preset-generator'] = 'file:' + prefix + 'dsh-preset-generator-0.4.0.tgz';
  const lockPath = join(cwd, 'package-lock.json');
  const prior = JSON.parse(readFileSync(lockPath, 'utf8'));
  for (const item of specs) delete prior.packages['node_modules/' + item.package];
  writeFileSync(lockPath, JSON.stringify(prior, null, 2) + '\n');
  writeFileSync(filename, JSON.stringify(temporary, null, 2) + '\n');
  try {
    process.stdout.write(run('npm', ['install', '--package-lock-only', '--ignore-scripts', '--no-audit', '--no-fund'], cwd));
  } finally {
    writeFileSync(filename, original);
  }
  const lock = JSON.parse(readFileSync(lockPath, 'utf8'));
  lock.packages[''].dependencies = manifest.dependencies;
  writeFileSync(lockPath, JSON.stringify(lock, null, 2) + '\n');
  for (const item of specs) {
    const entry = lock.packages['node_modules/' + item.package];
    if (!entry) { if (relative === '.' && item.package.includes('claude')) continue; throw new Error('Missing companion from resolved lock'); }
    if (entry.integrity !== item.pinValue.integrity || entry.version !== item.version) throw new Error('Stale companion lock');
    for (const field of ['dependencies', 'peerDependencies', 'peerDependenciesMeta']) {
      const normalize = value => JSON.stringify(Object.fromEntries(Object.entries(value || {}).sort()));
      if (normalize(entry[field]) !== normalize(item.manifest[field])) throw new Error('Resolved metadata mismatch: ' + item.package + '/' + field);
    }
  }
  process.stdout.write(run('npm', ['ci', '--ignore-scripts', '--no-audit', '--no-fund'], cwd));
}
process.stdout.write(run('node', ['--input-type=module', '-e', "await import('dsh-subagent-codex-pro/machine'); console.log('Independent Codex machine import passed')"], root));

// Transfer only public, reproducible source-candidate bytes and lockfiles.
// Each part has an index; the receiver verifies total length and SHA-256.
const outputs = [
  'package-lock.json', 'plugins/dsh/package-lock.json',
  ...specs.flatMap(item => ['plugins/dsh/vendor/' + item.tarball, 'plugins/dsh/vendor/' + item.pin]),
];
for (const path of outputs) {
  const bytes = readFileSync(join(root, path));
  const encoded = bytes.toString('base64');
  const parts = encoded.match(/.{1,4096}/g) || [];
  const blobSha = hash(Buffer.concat([Buffer.from('blob ' + bytes.length + '\0'), bytes]), 'sha1');
  console.log('COMPANION_FILE_BEGIN ' + JSON.stringify({ path, bytes: bytes.length, sha256: hash(bytes), blobSha, parts: parts.length }));
  for (let index = 0; index < parts.length; index++) console.log('COMPANION_FILE_PART ' + JSON.stringify({ path, index, data: parts[index] }));
  console.log('COMPANION_FILE_END ' + JSON.stringify({ path }));
}
