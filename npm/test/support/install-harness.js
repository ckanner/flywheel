"use strict";

const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const http = require('node:http');
const crypto = require('node:crypto');
const { spawn, execFileSync } = require('node:child_process');
const { capture } = require('../../install.js');

/** Serve the actual published dependency, verified against the source lock. */
async function runtimeDependency() {
  const lock = JSON.parse(await fs.readFile(path.resolve(__dirname, '../../../plugins/dsh/package-lock.json'), 'utf8'));
  const entry = lock.packages?.['node_modules/dsh-subagent-codex-pro'];
  if (entry?.version !== '0.8.0' || !entry.integrity?.startsWith('sha512-') ||
      entry.resolved !== 'https://registry.npmjs.org/dsh-subagent-codex-pro/-/dsh-subagent-codex-pro-0.8.0.tgz') {
    throw new Error('the installation fixture requires the exact locked Codex Pro release');
  }
  const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'flywheel published dependency '));
  try {
    let filename = process.env.FLYWHEEL_CODEX_PRO_TARBALL;
    if (!filename) {
      const [npm, args] = npmInvocation();
      // npm 12 disables direct remote tarball specs by default. Use the exact
      // registry identity, then verify the downloaded bytes against the lock.
      const result = await capture(npm, [...args, 'pack', `dsh-subagent-codex-pro@${entry.version}`, '--registry=https://registry.npmjs.org/', '--ignore-scripts', '--json', '--pack-destination', temporary,
        '--fetch-timeout=30000', '--fetch-retries=0'], { environment: process.env, cwd: temporary, timeoutMs: 60000 });
      if (result.code !== 0 || result.signal || result.error) throw new Error('could not fetch the locked published Codex Pro tarball: ' + (result.error || result.stderr));
      filename = path.join(temporary, 'dsh-subagent-codex-pro-0.8.0.tgz');
    }
    const data = await fs.readFile(filename);
    if (`sha512-${crypto.createHash('sha512').update(data).digest('base64')}` !== entry.integrity) throw new Error('published Codex Pro tarball integrity mismatch');
    const manifest = JSON.parse(execFileSync('tar', ['-xOf', filename, 'package/package.json'], { encoding: 'utf8' }));
    if (manifest.name !== 'dsh-subagent-codex-pro' || manifest.version !== entry.version || Object.keys(manifest.dependencies || {}).length !== 0) {
      throw new Error('published Codex Pro dependency identity or dependency graph changed');
    }
    return { manifest, entry: { name: manifest.name, version: entry.version, integrity: entry.integrity,
      filename: 'dsh-subagent-codex-pro-0.8.0.tgz' }, data };
  } finally { await fs.rm(temporary, { recursive: true, force: true }); }
}

async function registry(directory) {
  const candidate = JSON.parse(await fs.readFile(path.join(directory, 'candidate.json'), 'utf8'));
  const packages = new Map();
  const tarballs = new Map();
  for (const entry of candidate.packages) {
    const filename = path.join(directory, entry.filename);
    const data = await fs.readFile(filename);
    if (`sha512-${crypto.createHash('sha512').update(data).digest('base64')}` !== entry.integrity) throw new Error(`candidate integrity mismatch: ${entry.name}`);
    const manifest = JSON.parse(execFileSync('tar', ['-xOf', filename, 'package/package.json'], { encoding: 'utf8' }));
    packages.set(manifest.name, { manifest, entry, data });
    tarballs.set('/tarballs/' + entry.filename, data);
  }
  const dependency = await runtimeDependency();
  if (packages.get('@ckanner/dsh-plugin-flywheel')?.manifest.dependencies?.[dependency.manifest.name] !== dependency.manifest.version) {
    throw new Error('the candidate plugin does not pin the verified runtime dependency');
  }
  packages.set(dependency.manifest.name, dependency);
  tarballs.set('/tarballs/' + dependency.entry.filename, dependency.data);
  const requests = [];
  let url;
  const server = http.createServer((request, response) => {
    const pathname = decodeURIComponent(new URL(request.url, 'http://localhost').pathname);
    requests.push(pathname);
    const tarball = tarballs.get(pathname);
    if (tarball) { response.writeHead(200, { 'content-type': 'application/octet-stream' }); response.end(tarball); return; }
    const entry = packages.get(pathname.slice(1));
    if (entry) {
      const version = { ...entry.manifest, dist: { tarball: url + '/tarballs/' + entry.entry.filename, integrity: entry.entry.integrity,
        shasum: crypto.createHash('sha1').update(entry.data).digest('hex') } };
      response.writeHead(200, { 'content-type': 'application/json' });
      response.end(JSON.stringify({ name: version.name, 'dist-tags': { latest: version.version }, versions: { [version.version]: version } }));
      return;
    }
    response.writeHead(404, { 'content-type': 'application/json' });
    response.end(JSON.stringify({ error: 'Not found', code: 'E404' }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  url = `http://127.0.0.1:${server.address().port}`;
  return { url, candidate, requests, close: () => new Promise(resolve => { server.close(resolve); server.closeAllConnections(); }) };
}

async function environment(registryURL, version) {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'flywheel npm $() '));
  const home = path.join(root, 'user home');
  const prefix = path.join(root, 'global prefix');
  const project = path.join(root, 'target project');
  const fixtures = path.join(root, 'commands');
  for (const dir of [home, prefix, project, fixtures]) await fs.mkdir(dir, { recursive: true });
  const npmrc = path.join(root, 'npmrc');
  await fs.writeFile(npmrc, '');
  const env = { ...process.env, HOME: home, DSH_HOME: path.join(home, '.dsh'), NO_PROXY: '127.0.0.1,localhost',
    npm_config_registry: registryURL, npm_config_cache: path.join(root, 'npm cache'), npm_config_userconfig: npmrc,
    npm_config_prefix: prefix, npm_config_update_notifier: 'false', npm_config_audit: 'false', npm_config_fund: 'false',
    FLYWHEEL_DSH_FIXTURE_LOG: path.join(root, 'dsh-calls.jsonl'), FLYWHEEL_DSH_FIXTURE_VERSION: version };
  for (const name of Object.keys(env)) if (/^(FLYWHEEL_(CONFIG|DSH_BIN|DSH_WORKER|CODEX_BIN|CODEX_PRO_WORKER|KIMI_BIN|INSTALLATION)|CODEX_SUBAGENT_LIVE|CODEX_HOME|NODE_OPTIONS)$/.test(name)) delete env[name];
  env.FLYWHEEL_KIMI_BIN = path.join(root, 'intentionally missing kimi');
  env.FLYWHEEL_CODEX_BIN = path.join(root, 'intentionally missing codex');
  env.CODEX_HOME = path.join(home, 'isolated codex');
  const dsh = path.join(fixtures, 'desktop dsh');
  await fs.writeFile(dsh, `#!${process.execPath}\n` + String.raw`
const fs = require('node:fs');
const args = process.argv.slice(2);
fs.appendFileSync(process.env.FLYWHEEL_DSH_FIXTURE_LOG, JSON.stringify(args)+'\n');
if (args.length === 1 && args[0] === '--version') { console.log('0.2.1-rc.5'); process.exit(0); }
if (args[0] !== 'plugin' || args[1] !== '--profile' || args[2] !== 'desktop') process.exit(19);
if (process.env.FLYWHEEL_DSH_FIXTURE_FAIL === '1') { console.error('fixture profile is locked'); process.exit(23); }
if (args[3] === 'add') {
  if (args.at(-1) !== '@ckanner/dsh-plugin-flywheel@'+process.env.FLYWHEEL_DSH_FIXTURE_VERSION) process.exit(24);
  process.exit(0);
}
if (args[3] === 'list') {
  console.log(JSON.stringify([{dependencies:{'@ckanner/dsh-plugin-flywheel':{version:process.env.FLYWHEEL_DSH_FIXTURE_VERSION}}}]));
  process.exit(0);
}
process.exit(25);
`, { mode: 0o755 });
  return { root, home, prefix, project, fixtures, dsh, env, clean: () => fs.rm(root, { recursive: true, force: true }) };
}

function run(command, args, env, cwd, timeoutMs = 180000) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { env, cwd, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
    child.stdout.on('data', value => { stdout += value; });
    child.stderr.on('data', value => { stderr += value; });
    const timer = setTimeout(() => { child.kill('SIGTERM'); reject(new Error(`command timed out: ${command}\n${stderr}`)); }, timeoutMs);
    child.on('error', error => { clearTimeout(timer); reject(error); });
    child.on('close', (code, signal) => { clearTimeout(timer); resolve({ code, signal, stdout, stderr }); });
  });
}

function npmInvocation() {
  return process.env.npm_execpath ? [process.execPath, [process.env.npm_execpath]] : ['npm', []];
}

async function firstInstall(context, version, extra = []) {
  const [npm, prefixArgs] = npmInvocation();
  return run(npm, [...prefixArgs, 'exec', '--yes', '--ignore-scripts', '--include=optional', '--package', `@ckanner/flywheel@${version}`,
    '--', 'flywheel', 'install', '--prefix', context.prefix, '--dsh-bin', context.dsh, '--json', ...extra], context.env, context.project);
}

async function installedRoot(context) {
  const [npm, prefixArgs] = npmInvocation();
  const result = await run(npm, [...prefixArgs, 'root', '--global', '--prefix', context.prefix], context.env, context.project);
  if (result.code !== 0) throw new Error(result.stderr);
  return path.join(result.stdout.trim(), '@ckanner', 'flywheel');
}

async function waitFile(filename, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try { return await fs.readFile(filename, 'utf8'); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw new Error(`fixture did not become ready: ${filename}`);
}

module.exports = { registry, environment, run, firstInstall, installedRoot, waitFile };
