"use strict";

// Populate the isolated install registry with the real, integrity-checked SDK
// dependency graph. The native provider remains an offline protocol fixture.
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const { capture } = require('../../install.js');
const { parsePackOutput } = require('../../../scripts/release-build.js');

async function loadClaudeSdkDependencies() {
  const lock = JSON.parse(await fs.readFile(path.resolve(__dirname, '../../../plugins/dsh/package-lock.json'), 'utf8'));
  const sdkKey = 'node_modules/@anthropic-ai/claude-agent-sdk';
  if (lock.packages?.[sdkKey]?.version !== '0.3.326') throw new Error('the Claude install gate requires the locked official SDK 0.3.326');
  const pending = [sdkKey], selected = new Map();
  const resolve = (parent, name) => {
    let cursor = parent;
    while (cursor !== '.') {
      const key = cursor + '/node_modules/' + name;
      if (lock.packages[key]) return key;
      cursor = path.posix.dirname(cursor);
    }
    const key = 'node_modules/' + name;
    if (!lock.packages[key]) throw new Error('missing SDK dependency lock entry: ' + name);
    return key;
  };
  const libc = process.platform === 'linux' && !process.report.getReport().header.glibcVersionRuntime ? 'musl' : 'glibc';
  const matches = (constraints, value) => !constraints || (!constraints.includes('!' + value) &&
    (constraints.every(item => item.startsWith('!')) || constraints.includes(value)));
  while (pending.length) {
    const key = pending.pop(), entry = lock.packages[key];
    if (selected.has(key)) continue;
    if (entry.optional && (!matches(entry.os, process.platform) || !matches(entry.cpu, process.arch) || !matches(entry.libc, libc))) continue;
    selected.set(key, entry);
    for (const name of new Set([...Object.keys(entry.dependencies || {}), ...Object.keys(entry.optionalDependencies || {}),
      ...Object.keys(entry.peerDependencies || {}).filter(name => !entry.peerDependenciesMeta?.[name]?.optional)])) {
      pending.push(resolve(key, name));
    }
  }
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'flywheel-sdk-registry-'));
  // Different nesting positions may resolve to the same archive. Fetch it once
  // so parallel npm pack processes never write the same destination file.
  const unique = new Map([...selected.entries()].map(([key, entry]) => [
    key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length) + '@' + entry.version, [key, entry],
  ]));
  const queue = [...unique.values()], results = [];
  const [npm, prefix] = process.env.npm_execpath ? [process.execPath, [process.env.npm_execpath]] : ['npm', []];
  try {
    const workers = await Promise.allSettled(Array.from({ length: 6 }, async () => {
      while (queue.length) {
        const [key, entry] = queue.pop();
        const name = key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length);
        if (!entry.resolved?.startsWith('https://registry.npmjs.org/') || !entry.integrity?.startsWith('sha512-')) {
          throw new Error('SDK install fixture requires an exact registry integrity: ' + name);
        }
        const packed = await capture(npm, [...prefix, 'pack', `${name}@${entry.version}`, '--registry=https://registry.npmjs.org/',
          '--ignore-scripts', '--json', '--prefer-offline', '--pack-destination', directory, '--fetch-timeout=30000', '--fetch-retries=0'],
        { environment: process.env, cwd: directory, timeoutMs: 60000 });
        if (packed.code !== 0 || packed.signal || packed.error) throw new Error('could not fetch locked SDK dependency ' + name + ': ' + (packed.error || packed.stderr));
        const { filename } = parsePackOutput(packed.stdout, name, entry.version);
        const file = path.join(directory, filename), data = await fs.readFile(file);
        if (`sha512-${crypto.createHash('sha512').update(data).digest('base64')}` !== entry.integrity) {
          throw new Error('SDK dependency integrity mismatch: ' + name);
        }
        const manifest = JSON.parse(execFileSync('tar', ['-xOf', file, 'package/package.json'], { encoding: 'utf8' }));
        if (manifest.name !== name || manifest.version !== entry.version) throw new Error('SDK dependency identity mismatch: ' + name);
        results.push({ manifest, entry: { name, version: entry.version, integrity: entry.integrity, filename }, data });
      }
    }));
    const failures = workers.filter(result => result.status === 'rejected');
    if (failures.length) throw new AggregateError(failures.map(result => result.reason), 'could not prepare the verified SDK install registry');
    return results;
  } finally { await fs.rm(directory, { recursive: true, force: true }); }
}

// Both isolated installation scenarios use the same immutable source lock.
// Share the verified bytes within this test process instead of fetching twice.
let prepared;
function claudeSdkDependencies() { return prepared ??= loadClaudeSdkDependencies(); }
module.exports = { claudeSdkDependencies };
