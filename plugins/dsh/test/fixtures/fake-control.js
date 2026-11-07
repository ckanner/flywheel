#!/usr/bin/env node
import { appendFileSync } from 'node:fs';
import { spawn } from 'node:child_process';
const args = process.argv.slice(2);
if (process.env.TEST_CONTROL_LOG) appendFileSync(process.env.TEST_CONTROL_LOG, JSON.stringify(args) + '\n');
const mode = process.env.TEST_CONTROL_MODE;
const taskDir = args.includes('--task-dir') ? args[args.indexOf('--task-dir') + 1] : '';
const cwd = args[args.indexOf('--cwd') + 1];
const configuration = { source: `${cwd}/flywheel.config.json`, scope: 'project', bindings: [{
  id: 'routes.default', role: 'execute', route: 'default', profile: 'fast', harness: 'dsh',
  provider: 'deepseek-official', channel: 'deepseek-main', model: 'deepseek-flash', thinking: 'high',
  residentSession: false, permissionMode: 'workspace-write', inScope: true,
}] };
const running = ['start', 'launch'].includes(args[1]);
const response = { protocol: 1, ok: true, running, pid: 42, taskDir, cwd,
  state: { slug: 'example', status: running ? 'running' : 'pending', model: 'deepseek-flash', packets: {} } };
if (args[1] === 'config') response.configuration = configuration;
if (args[1] === 'doctor') {
  response.diagnostic = { kind: 'doctor', ok: true, cwd, taskDir, probeRequested: args.includes('--probe'),
    inferenceTested: false, configuration,
    checks: [{ id: 'account', status: 'unknown', summary: '账号尚未验证，不发送推理请求。' }] };
}
if (['doctor-fail', 'unexpected-exit', 'fail-exit-zero'].includes(mode)) {
  response.ok = false;
  response.error = 'required preflight check failed';
  response.diagnostic.ok = false;
  response.diagnostic.checks.push({ id: 'binary.kimi', status: 'fail', summary: 'Kimi CLI is missing',
    code: 'INVALID_REQUEST', remedy: 'Set the configured CLI to an existing executable.' });
  process.exitCode = mode === 'unexpected-exit' ? 2 : mode === 'fail-exit-zero' ? 0 : 1;
}
if (mode === 'wrong-task') response.taskDir += '/wrong';
if (mode === 'wrong-cwd') response.cwd += '/wrong';
if (mode === 'wrong-protocol') response.protocol = 2;
if (mode === 'wrong-diagnostic-task') response.diagnostic.taskDir += '/wrong';
if (mode === 'wrong-probe') response.diagnostic.probeRequested = !response.diagnostic.probeRequested;
if (mode === 'claims-inference') response.diagnostic.inferenceTested = true;
if (mode === 'bad-check') response.diagnostic.checks[0].status = 'maybe';
if (mode === 'missing-report') { delete response.configuration; delete response.diagnostic; }
if (mode === 'bad-binding') response.configuration.bindings[0].inScope = 'yes';
if (mode === 'legacy-binding') Object.assign(response.configuration.bindings[0], { model: '', channel: '', provider: '' });
if (mode === 'disagree') response.diagnostic.ok = false;
if (mode === 'cleanup' || mode === 'ignore-term') {
  const log = event => appendFileSync(process.env.TEST_CONTROL_LIFECYCLE_LOG,
    JSON.stringify({ event, pid: process.pid, child: owned?.pid ?? null }) + '\n');
  // Model Go owning a separate watchdog group. Killing this leader too early
  // leaves the child alive, which the lifecycle test detects and cleans up.
  const owned = mode === 'cleanup' ? spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], {
    detached: true, stdio: 'ignore',
  }) : null;
  const closed = owned && new Promise(resolve => owned.once('close', resolve));
  const keepAlive = setInterval(() => {}, 1000);
  const terminate = async () => {
    log('term');
    if (mode === 'ignore-term') return;
    // Deliberately exceeds both the old 500ms and an insufficient 6s grace.
    await new Promise(resolve => setTimeout(resolve, 6500));
    process.kill(-owned.pid, 'SIGTERM');
    await closed;
    log('cleanup-complete');
    clearInterval(keepAlive);
    process.stdout.write(JSON.stringify(response) + '\n');
  };
  if (mode === 'cleanup') process.once('SIGTERM', terminate);
  else process.on('SIGTERM', terminate);
  log('ready');
} else if (mode === 'hang') {
  setInterval(() => {}, 1000);
} else if (mode === 'invalid-json') {
  process.stdout.write('not-json\n');
} else if (mode === 'split-utf8') {
  const bytes = Buffer.from(JSON.stringify(response) + '\n');
  const boundary = bytes.indexOf(Buffer.from('未')) + 1;
  process.stdout.write(bytes.subarray(0, boundary));
  await new Promise(resolve => setTimeout(resolve, 20));
  process.stdout.write(bytes.subarray(boundary));
} else {
  process.stdout.write(JSON.stringify(response) + '\n');
}
