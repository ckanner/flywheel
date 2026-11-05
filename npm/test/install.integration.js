"use strict";

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { pathToFileURL } = require('node:url');
const { shellQuote } = require('../install.js');
const { registry, environment, run, firstInstall, installedRoot, waitFile } = require('./support/install-harness.js');

test('the six packaged npm candidates install and run from a clean agent environment', { timeout: 240000 }, async t => {
  const directory = path.resolve(process.env.FLYWHEEL_NPM_PACKAGES_DIR || 'dist-npm');
  const server = await registry(directory);
  const context = await environment(server.url, server.candidate.version);
  t.after(async () => { await server.close(); await context.clean(); });
  const first = await firstInstall(context, server.candidate.version);
  assert.equal(first.code, 0, first.stderr + '\n' + first.stdout);
  const report = JSON.parse(first.stdout);
  assert.equal(report.ok, true);
  assert.equal(report.authentication, 'unknown');
  assert.equal(report.skills.count, 10);
  assert.equal(report.configuration.action, 'created');
  assert.equal(report.configuration.automaticMigration, false);
  assert.equal(report.steps.find(step => step.name === 'dsh-plugin').status, 'ok');
  assert.equal(report.steps.find(step => step.name === 'dsh-plugin').reloadRequired, true);
  assert.equal(report.ready, false, 'missing local harness/account prerequisites must remain visible');
  const root = await installedRoot(context);
  const cli = path.join(root, 'bin', 'flywheel.js');
  const worker = path.join(context.prefix, 'bin', 'flywheel-dsh-worker');
  await fs.access(worker, 1);
  const codexProWorker = path.join(context.prefix, 'bin', 'flywheel-codex-pro-worker');
  await fs.access(codexProWorker, 1);
  assert.deepEqual(report.adapter, { package: 'dsh-subagent-codex-pro', version: '0.8.0', protocol: 1, inferenceTested: false });
  assert.equal((await run(process.execPath, [cli, 'version'], context.env, context.project)).stdout.includes(server.candidate.version), true);
  const template = await run(process.execPath, [cli, 'template', 'flywheel.profiles.example.json'], context.env, context.project);
  assert.equal(template.code, 0, template.stderr);
  assert.equal(JSON.parse(template.stdout).profiles.flash.provider, 'deepseek-account');
  const config = await run(report.paths.launcher, ['config', 'inspect', '--cwd', context.project, '--json'], context.env, context.project);
  assert.equal(config.code, 0, config.stderr);
  const bindings = JSON.parse(config.stdout).bindings;
  assert.equal(bindings.length, 8);
  for (const [id, thinking] of [['loops.intake', 'low'], ['routes.default', 'high']]) {
    const binding = bindings.find(value => value.id === id);
    assert.ok(binding, `installed configuration must include ${id}`);
    assert.equal(binding.provider, 'deepseek-account');
    assert.equal(binding.channel, 'deepseek-account');
    assert.equal(binding.model, 'deepseek-flash');
    assert.equal(binding.thinking, thinking);
    assert.equal(binding.permissionMode, 'danger-full-access');
  }
  for (const document of ['docs/model-policy.md', 'docs/en/model-policy.md']) {
    await fs.access(path.join(root, document));
  }
  const demo = path.join(context.root, 'packaged tutorial demo');
  const example = await run('bash', [path.join(root, 'docs', 'examples', 'create-first-task.sh'), demo],
    { ...context.env, FLYWHEEL_BIN: report.paths.launcher }, context.project);
  assert.equal(example.code, 0, example.stderr);
  assert.match(await fs.readFile(path.join(demo, 'docs', 'add', 'task-packet-M01-T01.md'), 'utf8'), /packet-id: M01-T01/);
  assert.equal(await fs.stat(path.join(demo, 'docs', 'add', 'state.json')).then(() => true, () => false), false);
  const globalConfig = path.join(context.home, '.config', 'flywheel', 'flywheel.config.json');
  const originalConfig = await fs.readFile(globalConfig, 'utf8');
  const customized = JSON.parse(originalConfig);
  customized._note_installer_test = 'user-owned configuration must survive setup';
  await fs.writeFile(globalConfig, JSON.stringify(customized, null, 2));
  const before = await fs.readFile(globalConfig);

  const installedRequire = require('node:module').createRequire(path.join(root, 'package.json'));
  const workerPath = installedRequire.resolve('@ckanner/dsh-plugin-flywheel/worker');
  const installationModule = path.join(path.dirname(workerPath), 'installation.js');
  const { resolveFlywheelBinary } = await import(pathToFileURL(installationModule));
  assert.equal(resolveFlywheelBinary({}, context.env), report.paths.launcher, 'desktop uses the installed launcher, not terminal PATH');

  await t.test('installed worker imports the real pinned plugin and executes an offline app-server', async () => {
    const doctor = await run(codexProWorker, ['--doctor'], context.env, context.project);
    assert.equal(doctor.code, 0, doctor.stderr + doctor.stdout);
    assert.deepEqual(JSON.parse(doctor.stdout), { protocol: 1, type: 'doctor', package: 'dsh-subagent-codex-pro', version: '0.8.0',
      machineProtocol: 1, available: true, inferenceTested: false });
    const fixture = path.join(context.fixtures, 'offline-codex.mjs');
    await fs.copyFile(path.resolve(__dirname, '../../plugins/dsh/test/fixtures/fake-codex-pro.js'), fixture);
    await fs.chmod(fixture, 0o755);
    const log = path.join(context.root, 'installed-codex-rpc.jsonl');
    const child = spawn(codexProWorker, [], { cwd: context.project,
      env: { ...context.env, FLYWHEEL_CODEX_BIN: fixture, TEST_CODEX_PRO_LOG: log }, stdio: ['pipe', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
    child.stdout.on('data', value => { stdout += value; }); child.stderr.on('data', value => { stderr += value; });
    child.stdin.on('error', () => {});
    const done = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', (code, signal) => resolve({ code, signal })); });
    const timeout = setTimeout(() => child.kill('SIGTERM'), 15000);
    child.stdin.write(JSON.stringify({ protocol: 1, type: 'start', requestId: 'actual-npm-install', cwd: await fs.realpath(context.project),
      model: 'fixture-model', reasoningEffort: 'high', permissionMode: 'workspace-write', prompt: 'Offline installed package validation.', timeoutMs: 5000 }) + '\n');
    let exit;
    try { exit = await done; } finally { clearTimeout(timeout); }
    assert.equal(exit.code, 0, stderr + stdout);
    assert.equal(exit.signal, null);
    const frames = stdout.trim().split('\n').map(line => JSON.parse(line));
    const result = frames.at(-1);
    assert.deepEqual(result.adapter, { package: 'dsh-subagent-codex-pro', version: '0.8.0', protocol: 1 });
    assert.equal(result.receipt.ok, true);
    assert.equal(result.receipt.outcome.cleanup.confirmed, true);
    for (const pid of [result.receipt.outcome.execution.pid, result.receipt.outcome.execution.supervisorPid]) {
      assert.ok(Number.isSafeInteger(pid) && pid > 0);
      assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
    }
    assert.throws(() => process.kill(-result.receipt.outcome.execution.supervisorPid, 0), { code: 'ESRCH' });
    assert.equal(result.receipt.outcome.turnConfiguration.effectiveEffort, null);
    assert.equal(result.receipt.outcome.turnConfiguration.accepted, true);
    assert.equal(result.receipt.outcome.usage.status, 'complete');
    assert.equal(result.receipt.costStatus, 'unknown');
    assert.equal(result.receipt.costUSD, null);
    const calls = (await fs.readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
    assert.deepEqual(calls[0].argv, ['app-server', '--stdio']);
    assert.equal(calls.find(call => call.method === 'turn/start').params.effort, 'high');
    assert.ok(server.requests.includes('/dsh-subagent-codex-pro'));
    assert.ok(server.requests.includes('/tarballs/dsh-subagent-codex-pro-0.8.0.tgz'));
  });

  await t.test('same-version setup is repeatable and preserves configuration', async () => {
    const second = await run(process.execPath, [cli, 'setup', '--no-global', '--prefix', context.prefix, '--dsh-bin', context.dsh, '--json'], context.env, context.project);
    assert.equal(second.code, 0, second.stderr + second.stdout);
    const result = JSON.parse(second.stdout);
    assert.equal(result.skills.count, 10);
    assert.equal(result.paths.launcher, report.paths.launcher);
    assert.deepEqual(await fs.readFile(globalConfig), before);
    assert.equal(result.configuration.preserved, true);
    assert.equal(result.configuration.path, globalConfig);
    assert.match(result.configuration.message, /升级不会替换旧模型/);
    assert.deepEqual(result.configuration.migration.args.slice(0, 4), ['config', 'migrate', '--preset', 'subscriptions']);
    assert.equal(await fs.readFile(path.join(context.home, '.agents', 'skills', 'flywheel-run', 'SKILL.md'), 'utf8').then(v => v.includes('flywheel run')), true);
  });

  await t.test('installed npm shim forwards cancellation through the real Go watchdog', async () => {
    const marker = path.join(context.root, 'git-ready');
    const cleaned = path.join(context.root, 'git-cleaned');
    const fakeGit = path.join(context.fixtures, 'git');
    await fs.writeFile(fakeGit, '#!/bin/sh\ntrap ' + shellQuote('printf cleaned > ' + shellQuote(cleaned) + '; exit 0')
      + ' TERM INT\nprintf "%s" "$$" > ' + shellQuote(marker) + '\nwhile :; do sleep 0.1; done\n', { mode: 0o755 });
    const env = { ...context.env, PATH: context.fixtures + path.delimiter + context.env.PATH };
    const child = spawn(process.execPath, [cli, 'config', 'inspect', '--cwd', context.project, '--json'], { env, cwd: context.project, stdio: ['ignore', 'pipe', 'pipe'] });
    child.stdout.resume(); child.stderr.resume();
    const finished = new Promise((resolve, reject) => { child.once('close', (code, signal) => resolve({ code, signal })); child.once('error', reject); });
    t.after(() => { if (child.exitCode === null) child.kill('SIGKILL'); });
    const pid = Number(await waitFile(marker));
    child.kill('SIGTERM');
    const outcome = await Promise.race([finished, new Promise((_, reject) => {
      const timer = setTimeout(() => reject(new Error('installed wrapper did not await Go cleanup')), 10000); timer.unref();
    })]);
    assert.notEqual(outcome.code, 0);
    assert.equal(outcome.signal, null, 'npm shim must finish after forwarding the signal');
    assert.equal(await fs.readFile(cleaned, 'utf8'), 'cleaned');
    assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
    await fs.rm(fakeGit);
  });

  await t.test('DSH registration failure is reported and a retry repairs that step', async () => {
    const args = [cli, 'setup', '--no-global', '--prefix', context.prefix, '--dsh-bin', context.dsh, '--json'];
    const failed = await run(process.execPath, args, { ...context.env, FLYWHEEL_DSH_FIXTURE_FAIL: '1' }, context.project);
    assert.equal(failed.code, 1);
    const failure = JSON.parse(failed.stdout);
    assert.equal(failure.ok, false);
    assert.equal(failure.steps.find(step => step.name === 'skills').status, 'ok');
    assert.equal(failure.steps.at(-1).name, 'dsh-plugin');
    assert.equal(failure.steps.at(-1).status, 'failed');
    const recovered = await run(process.execPath, args, context.env, context.project);
    assert.equal(recovered.code, 0, recovered.stdout + recovered.stderr);
    assert.equal(JSON.parse(recovered.stdout).ok, true);
  });

  await t.test('explicit CLI-only installation keeps DSH skipped', async () => {
    const result = await run(process.execPath, [cli, 'setup', '--no-global', '--prefix', context.prefix, '--no-dsh', '--json'], context.env, context.project);
    assert.equal(result.code, 0, result.stdout + result.stderr);
    assert.equal(JSON.parse(result.stdout).steps.find(step => step.name === 'dsh-plugin').status, 'skipped');
  });

  await t.test('user-edited skill content is preserved and prevents false installation success', async () => {
    const skill = path.join(context.home, '.agents', 'skills', 'flywheel-run', 'SKILL.md');
    await fs.appendFile(skill, '\nUser installation customization.\n');
    const userVersion = await fs.readFile(skill);
    const result = await run(process.execPath, [cli, 'setup', '--no-global', '--prefix', context.prefix, '--no-dsh', '--json'], context.env, context.project);
    assert.equal(result.code, 1);
    const failure = JSON.parse(result.stdout);
    assert.equal(failure.ok, false);
    assert.equal(failure.steps.at(-1).name, 'skills');
    assert.equal(failure.steps.at(-1).summary.results.length, 10);
    assert.deepEqual(await fs.readFile(skill), userVersion);
  });
  const calls = (await fs.readFile(context.env.FLYWHEEL_DSH_FIXTURE_LOG, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.ok(calls.some(args => args[3] === 'add' && args.at(-1) === `@ckanner/dsh-plugin-flywheel@${server.candidate.version}`));
  assert.ok(server.requests.some(request => request.includes('flywheel-' + process.platform + '-' + (process.arch === 'x64' ? 'amd64' : process.arch))));
  assert.equal(await fs.stat(path.join(context.project, 'state.json')).then(() => true, () => false), false);
});
