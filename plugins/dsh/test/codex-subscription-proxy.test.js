import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { chmod, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const proxy = fileURLToPath(new URL('../src/codex-subscription-proxy.js', import.meta.url));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const payload = JSON.stringify({ jsonrpc: '2.0', method: 'fixture/backpressure', params: { data: 'x'.repeat(7 * 1024 * 1024) } }) + '\n';

async function fixture(t, mode) {
  const cwd = await realpath(await mkdtemp(join(tmpdir(), 'flywheel proxy close $() ')));
  const command = join(cwd, 'native-peer.mjs'), log = join(cwd, 'native.jsonl');
  await writeFile(command, mode.startsWith('signal-') ? `#!/bin/sh
printf '%s\\n' '{"jsonrpc":"2.0","method":"fixture/ready","params":{}}'
kill -${mode.slice('signal-'.length)} "$$"
exit 24
` : `#!/usr/bin/env node
import {close,openSync,read,writeSync} from 'node:fs';
const mode=process.env.TEST_PROXY_MODE;
const logFD=openSync(process.env.TEST_PROXY_LOG,'a');
const log=value=>writeSync(logFD,JSON.stringify(value)+'\\n');
const notify=method=>process.stdout.write(JSON.stringify({jsonrpc:'2.0',method,params:{pid:process.pid}})+'\\n');
let stopping=false,closePromise,replacement,readDone;
setInterval(()=>{},1000);
const readSettled=new Promise(resolve=>{readDone=resolve;});
// Own fd 0 directly, and finish its single read before the asynchronous close.
const closeInput=()=>closePromise??=readSettled.then(()=>new Promise((resolve,reject)=>{
  close(0,error=>{if(error){reject(error);return;}log({event:'input-closed'});resolve();});
}));
process.on('exit',code=>log({event:'exit',code}));
process.on('SIGTERM',()=>{
  log({event:'stop'});if(stopping)return;stopping=true;
  void closeInput().then(()=>{
    if(replacement!==undefined){writeSync(replacement,'still open');log({event:'replacement-open',fd:replacement});}
    setTimeout(()=>process.exit(0),50);
  });
});
read(0,Buffer.alloc(65536),0,65536,null,(error,bytes)=>{
  if(error)throw error;
  if(bytes>0)log({event:'input',bytes});
  readDone();
  if(mode==='backpressure'&&bytes>0)notify('fixture/backpressured');
  if(mode==='closed-input'&&bytes>0){
    void closeInput().then(()=>{
      replacement=openSync(process.env.TEST_PROXY_LOG+'.replacement','w');
      log({event:'replacement',fd:replacement});
    });
  }
});
notify('fixture/ready');
`);
  await chmod(command, 0o755);
  const child = spawn(process.execPath, [proxy, 'app-server', '--stdio'], { cwd, detached: true,
    env: { PATH: process.env.PATH, HOME: cwd, FLYWHEEL_CODEX_SUBSCRIPTION_COMMAND: command,
      TEST_PROXY_MODE: mode, TEST_PROXY_LOG: log }, stdio: ['pipe', 'pipe', 'pipe'] });
  const run = { child, frames: [], stderr: '', closed: false };
  let buffer = '';
  child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
  child.stdout.on('data', text => {
    buffer += text;
    for (let newline; (newline = buffer.indexOf('\n')) >= 0;) {
      run.frames.push(JSON.parse(buffer.slice(0, newline))); buffer = buffer.slice(newline + 1);
    }
  });
  child.stderr.on('data', text => { run.stderr += text; });
  child.stdin.on('error', () => {});
  run.done = new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('close', (code, signal) => { run.closed = true; run.terminal = { code, signal }; resolve(run.terminal); });
  });
  run.wait = async method => {
    const until = Date.now() + 5000;
    while (!run.frames.some(frame => frame.method === method)) {
      assert.equal(run.closed, false, 'proxy exited before ' + method + ': ' + run.stderr +
        (run.closed ? JSON.stringify({ terminal: run.terminal, native: await run.records() }) : ''));
      assert.ok(Date.now() < until, 'proxy did not observe ' + method);
      await delay(10);
    }
  };
  run.records = async () => (await readFile(log, 'utf8').catch(() => '')).trim().split('\n').filter(Boolean).map(JSON.parse);
  t.after(async () => {
    try { process.kill(-child.pid, 'SIGKILL'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
    await run.done;
    await rm(cwd, { recursive: true, force: true });
  });
  await run.wait('fixture/ready');
  return run;
}

test('backpressured native input tolerates duplicate EOF/group TERM only after owned shutdown', { timeout: 10000 }, async t => {
  const run = await fixture(t, 'backpressure');
  run.child.stdin.write(payload);
  await run.wait('fixture/backpressured');
  run.child.stdin.end();
  process.kill(-run.child.pid, 'SIGTERM');
  run.child.kill('SIGTERM');
  assert.deepEqual(await run.done, { code: 0, signal: null }, run.stderr);
  const records = await run.records();
  assert.ok(records.some(record => record.event === 'stop'));
  const readBytes = records.filter(record => record.event === 'input').reduce((sum, record) => sum + record.bytes, 0);
  assert.ok(readBytes > 0 && readBytes < Buffer.byteLength(payload), 'native stdin must close with a genuinely pending write');
  assert.throws(() => process.kill(-run.child.pid, 0), { code: 'ESRCH' });
});

test('native EPIPE before any shutdown remains a transport failure even when cleanup exits zero', { timeout: 10000 }, async t => {
  const run = await fixture(t, 'closed-input');
  run.child.stdin.write(payload);
  assert.deepEqual(await run.done, { code: 1, signal: null }, run.stderr);
  const records = await run.records();
  assert.ok(records.some(record => record.event === 'stop'));
  const readBytes = records.filter(record => record.event === 'input').reduce((sum, record) => sum + record.bytes, 0);
  assert.ok(readBytes > 0 && readBytes < Buffer.byteLength(payload), 'native must close stdin with a genuinely pending write before any requested shutdown');
  assert.equal(records.filter(record => record.event === 'input-closed').length, 1, 'stdin has one asynchronous close owner');
  assert.deepEqual(records.find(record => record.event === 'replacement'), { event: 'replacement', fd: 0 });
  assert.deepEqual(records.find(record => record.event === 'replacement-open'), { event: 'replacement-open', fd: 0 }, 'cleanup must leave the reused descriptor open');
  assert.deepEqual(records.filter(record => record.event === 'exit'), [{ event: 'exit', code: 0 }], 'native cleanup exit 0 cannot hide the earlier transport failure');
  assert.throws(() => process.kill(-run.child.pid, 0), { code: 'ESRCH' });
});

for (const signal of ['PIPE', 'USR1']) {
  test(`native SIG${signal} cannot be ignored or start an inspector in the Node proxy`, { timeout: 10000 }, async t => {
    const run = await fixture(t, 'signal-' + signal);
    assert.deepEqual(await run.done, { code: 1, signal: null }, run.stderr);
    assert.throws(() => process.kill(-run.child.pid, 0), { code: 'ESRCH' });
  });
}

for (const [name, tail] of [['JSON', '{"jsonrpc":"2.0","id":1'], ['UTF-8', Buffer.from([0xe4, 0xb8])]]) {
  test(`parent EOF with incomplete ${name} is rejected without forwarding an RPC`, { timeout: 10000 }, async t => {
    const run = await fixture(t, 'plain');
    run.child.stdin.end(tail);
    assert.deepEqual(await run.done, { code: 1, signal: null }, run.stderr);
    assert.equal((await run.records()).some(record => record.event === 'input'), false);
    assert.throws(() => process.kill(-run.child.pid, 0), { code: 'ESRCH' });
  });
}
