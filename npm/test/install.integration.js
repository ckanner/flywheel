"use strict";

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { assertSubscriptionCLIProfiles } = require('./support/installed-profiles.js');
const { spawn, execFileSync } = require('node:child_process');
const { pathToFileURL } = require('node:url');
const { shellQuote } = require('../install.js');
const { registry, environment, run, firstInstall, installedRoot, waitFile } = require('./support/install-harness.js');


// Desktop settings/UI peers must not become requirements of the independent
// machine worker installed by Flywheel. Assert absence before exercising the
// real public API below, so a leaked desktop import cannot pass accidentally.
function assertNoDesktopPeers(manifestPath) {
  const adapterRequire = require('node:module').createRequire(manifestPath);
  for (const name of ['@deepseek-ai/schemastery', '@deepseek-ai/dsh-home-paths', '@deepseek-ai/cordis', 'react']) {
    assert.throws(() => adapterRequire.resolve(name), { code: 'MODULE_NOT_FOUND' },
      name + ' must be absent from this clean machine-only installation');
  }
}

test('the complete npm candidate install and run from a clean agent environment', { timeout: 240000 }, async t => {
  const directory = path.resolve(process.env.FLYWHEEL_NPM_PACKAGES_DIR || 'dist-npm');
  const server = await registry(directory);
  const context = await environment(server.url, server.candidate.version);
  t.after(async () => { await server.close(); await context.clean(); });
  const first = await firstInstall(context, server.candidate.version);
  assert.equal(first.code, 0, first.stderr + '\n' + first.stdout);
  const report = JSON.parse(first.stdout);
  assert.equal(report.ok, true);
  assert.equal(report.authentication, 'unknown');
  assert.equal(report.skills.count, 11);
  const taskSkill = path.join(context.home, '.agents', 'skills', 'flywheel-task');
  const taskSkillText = await fs.readFile(path.join(taskSkill, 'SKILL.md'), 'utf8');
  assert.match(taskSkillText, /flywheel_control/);
  assert.match(taskSkillText, /"action": "planning"/);
  assert.match(await fs.readFile(path.join(taskSkill, 'references', 'contract.md'), 'utf8'), /scope-clear/);
  assert.equal(report.configuration.action, 'created');
  assert.equal(report.configuration.automaticMigration, false);
  assert.equal(report.steps.find(step => step.name === 'dsh-plugin').status, 'ok');
  assert.equal(report.steps.find(step => step.name === 'dsh-plugin').reloadRequired, true);
  assert.equal(report.ready, false, 'missing local harness/account prerequisites must remain visible');
  const root = await installedRoot(context);
  const cli = path.join(root, 'bin', 'flywheel.js');
  assert.equal(server.candidate.packages.length, 1);
  const native = JSON.parse(await fs.readFile(path.join(root, 'native/manifest.json'), 'utf8'));
  assert.equal(native.version, server.candidate.version);
  assert.deepEqual(Object.keys(native.platforms).sort(), ['darwin-amd64','darwin-arm64','linux-amd64','linux-arm64']);
  for (const id of Object.keys(native.platforms)) await fs.access(path.join(root, 'native', id, 'flywheel'), 1);
  const worker = path.join(context.prefix, 'bin', 'flywheel-dsh-worker');
  await fs.access(worker, 1);
  const codexProWorker = path.join(context.prefix, 'bin', 'flywheel-codex-pro-worker');
  await fs.access(codexProWorker, 1);
  const claudeCodeProWorker = path.join(context.prefix, 'bin', 'flywheel-claude-code-pro-worker');
  await fs.access(claudeCodeProWorker, 1);
  const kimiNativeWorker = path.join(context.prefix, 'bin', 'flywheel-kimi-native-worker');
  await fs.access(kimiNativeWorker, 1);
  const antigravityWorker = path.join(context.prefix, 'bin', 'flywheel-antigravity-worker');
  await fs.access(antigravityWorker, 1);
  const modelCatalogWorker = path.join(context.prefix, 'bin', 'flywheel-model-catalog');
  await fs.access(modelCatalogWorker, 1);
  assert.deepEqual(report.adapter, { package: 'dsh-subagent-codex-pro', version: '0.10.2', protocol: 1, inferenceTested: false });
  assert.equal((await run(process.execPath, [cli, 'version'], context.env, context.project)).stdout.includes(server.candidate.version), true);
  const template = await run(process.execPath, [cli, 'template', 'flywheel.profiles.example.json'], context.env, context.project);
  assert.equal(template.code, 0, template.stderr);
  assertSubscriptionCLIProfiles(JSON.parse(template.stdout));
  const config = await run(report.paths.launcher, ['config', 'inspect', '--cwd', context.project, '--json'], context.env, context.project);
  assert.equal(config.code, 0, config.stderr);
  const bindings = JSON.parse(config.stdout).bindings;
  assert.equal(bindings.length, 18);
  for (const id of ['loops.design-plan', 'loops.design-review', 'loops.plan-review']) {
    assert.ok(bindings.some(binding => binding.id === id), `effective configuration must expose ${id}`);
  }
  for (const [id, thinking] of [['loops.intake', 'low'], ['routes.default', 'high']]) {
    const binding = bindings.find(value => value.id === id);
    assert.ok(binding, `installed configuration must include ${id}`);
    assert.equal(binding.harness, 'antigravity');
    assert.equal(binding.channel, 'google-subscription');
    assert.equal(binding.model, 'gemini-4-hydrogen-' + thinking);
    assert.equal(binding.thinking, thinking);
    assert.equal(binding.permissionMode, 'danger-full-access');
  }
  for (const document of ['LICENSE', 'README.md', 'README.zh-CN.md', 'docs/README.md', 'docs/README.zh-CN.md', 'docs/zh-CN/model-policy.md', 'docs/en/model-policy.md', 'docs/zh-CN/claude-code-pro.md', 'docs/en/claude-code-pro.md', 'docs/en/antigravity.md', 'docs/zh-CN/antigravity.md', 'docs/en/image-inputs.md', 'docs/zh-CN/image-inputs.md']) {
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
  const workerPath = installedRequire.resolve('@ckanner/flywheel/worker');
  assert.equal(report.paths.claudeCodeProWorker, installedRequire.resolve('@ckanner/flywheel/claude-code-pro-worker'));
  assert.equal(report.paths.kimiNativeWorker, installedRequire.resolve('@ckanner/flywheel/kimi-native-worker'));
  assert.equal(report.paths.antigravityWorker, installedRequire.resolve('@ckanner/flywheel/antigravity-worker'));
  assert.equal(report.paths.modelCatalogWorker, installedRequire.resolve('@ckanner/flywheel/model-catalog-worker'));
  await fs.access(report.paths.modelCatalogWorker, 1);
  await fs.access(report.paths.kimiNativeWorker);
  const installedHost = await import(pathToFileURL(installedRequire.resolve('@ckanner/flywheel')));
  assert.ok(installedHost.inject.includes('sessionProjections'), 'the installed host imports its actual projection schema dependency');
  assert.equal(typeof installedHost.apply, 'function');
  assert.ok(server.requests.includes('/zod'));
  assert.ok(server.requests.includes('/tarballs/zod-4.3.6.tgz'));
  const installationModule = path.join(path.dirname(workerPath), 'installation.mjs');
  const { resolveFlywheelBinary } = await import(pathToFileURL(installationModule));
  assert.equal(resolveFlywheelBinary({}, context.env), report.paths.launcher, 'desktop uses the installed launcher, not terminal PATH');

  await t.test('the installed model catalog reads native metadata without inference, settings writes or task state', async () => {
    const kimi = await run(report.paths.launcher, ['control', 'models', '--cwd', context.project], context.env, context.project, 20000,
      JSON.stringify({ version: 1, harness: 'kimi', channel: 'kimi-subscription' }));
    assert.equal(kimi.code, 0, kimi.stderr);
    const unsupported = JSON.parse(kimi.stdout).modelCatalog;
    assert.equal(unsupported.status, 'unsupported');
    assert.deepEqual(unsupported.models, []);
    assert.equal(unsupported.inferencePerformed, false);
    assert.equal(unsupported.accountEntitlement, 'unknown');

    const fixture = path.join(context.fixtures, 'catalog agy');
    const observed = path.join(context.root, 'catalog-argv.json');
    await fs.writeFile(fixture, `#!${process.execPath}\nconst fs = require('node:fs');\n` +
      `const args = process.argv.slice(2); fs.writeFileSync(${JSON.stringify(observed)}, JSON.stringify(args));\n` +
      `if (args.length !== 1 || args[0] !== 'models') throw new Error('catalog must never dispatch a prompt');\n` +
      `console.log('gemini-4-hydrogen-medium\\ngemini-pro-orchestrator-neon');\n`, { mode: 0o755 });
    const agy = await run(report.paths.launcher, ['control', 'models', '--cwd', context.project],
      { ...context.env, FLYWHEEL_AGY_BIN: fixture }, context.project, 20000,
      JSON.stringify({ version: 1, harness: 'antigravity', channel: 'google-subscription' }));
    assert.equal(agy.code, 0, agy.stderr);
    const catalog = JSON.parse(agy.stdout).modelCatalog;
    assert.equal(catalog.status, 'available');
    assert.equal(catalog.source, 'agy models');
    assert.equal(catalog.scope, 'native-client-catalog');
    assert.deepEqual(catalog.models.map(model => model.id), ['gemini-4-hydrogen-medium', 'gemini-pro-orchestrator-neon']);
    assert.equal(catalog.models[1].defaultEffort, '');
    assert.equal(catalog.inferencePerformed, false);
    assert.equal(catalog.accountEntitlement, 'unknown');
    assert.deepEqual(JSON.parse(await fs.readFile(observed, 'utf8')), ['models']);
    assert.deepEqual(await fs.readFile(globalConfig), before);
    await assert.rejects(fs.access(path.join(context.project, 'state.json')), { code: 'ENOENT' });
  });

  await t.test('the bundled generator imports its public composition API without fetching a registry release', async () => {
    const compositionPath = installedRequire.resolve('dsh-preset-generator/composition');
    assert.equal(installedRequire.resolve('dsh-preset-generator'), compositionPath);
    const generator = await import(pathToFileURL(compositionPath));
    assert.equal(generator.COMPOSITION_PROTOCOL, 1);
    const composition = generator.createSdkPtcComposition({ runtimeProfile: 'sdk-install-check', preset: 'heavy' });
    assert.equal(composition.runtimeProfile, 'sdk-install-check');
    assert.equal(composition.presetPolicy, 'heavy');
    assert.deepEqual(composition.requirements.tools, ['run_code']);
    assert.equal(composition.requirements.executor, 'ptc');
    assert.equal(generator.validateSdkPtcAdmission(composition, {}).eligible, false,
      'an installed composition must still require observed runtime admission');
    assert.ok(!server.requests.some(value => value.includes('dsh-preset-generator')));
  });

  await t.test('the installed Kimi worker reports the absent CLI without inference', async () => {
    const doctor = await run(kimiNativeWorker, ['--doctor'], context.env, context.project);
    assert.equal(doctor.code, 1, doctor.stdout + doctor.stderr);
    assert.deepEqual(JSON.parse(doctor.stdout), { type: 'doctor', protocol: 1, available: false, cliVersion: null,
      backendProtocol: 'kimi-server/v1', inferenceTested: false,
      nativeAdmission: 'requires-actual-server-schema-and-effective-session-probe', recoverySupported: false });
  });

  await t.test('the bundled Antigravity worker executes the official CLI protocol from the installed package', async () => {
    const doctor = await run(antigravityWorker, ['--doctor'], context.env, context.project);
    assert.equal(doctor.code, 0, doctor.stdout + doctor.stderr);
    const facts = JSON.parse(doctor.stdout);
    assert.equal(facts.package, '@ckanner/dsh-subagent-antigravity');
    assert.equal(facts.version, '0.2.1');
    assert.equal(facts.inferenceTested, false);
    assert.equal(facts.accountEntitlement, 'unknown');
    const fixture = path.join(context.fixtures, 'offline-agy.mjs');
    await fs.copyFile(path.resolve(__dirname, '../../internal/drivers/testdata/fake-agy.js'), fixture);
    await fs.chmod(fixture, 0o755);
    const child = spawn(antigravityWorker, [], { cwd: context.project,
      env: { ...context.env, FLYWHEEL_AGY_BIN: fixture }, stdio: ['pipe', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
    child.stdout.on('data', value => { stdout += value; }); child.stderr.on('data', value => { stderr += value; });
    child.stdin.on('error', () => {});
    const done = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', (code, signal) => resolve({ code, signal })); });
    const timeout = setTimeout(() => child.kill('SIGTERM'), 15000);
    child.stdin.write(JSON.stringify({ protocol: 1, type: 'start', requestId: 'installed-antigravity', cwd: await fs.realpath(context.project),
      model: 'gemini-4-hydrogen-high', reasoningEffort: 'high', permissionMode: 'danger-full-access',
      subscription: { version: 1, mode: 'subscription-only' }, prompt: 'Offline installed subscription validation.', timeoutMs: 5000 }) + '\n');
    let exit;
    try { exit = await done; } finally { clearTimeout(timeout); }
    assert.equal(exit.code, 0, stderr + stdout);
    assert.equal(exit.signal, null);
    const result = stdout.trim().split('\n').map(line => JSON.parse(line)).at(-1);
    assert.deepEqual(result.adapter, { package: '@ckanner/dsh-subagent-antigravity', version: '0.2.1', protocol: 1 });
    assert.equal(result.receipt.ok, true);
    assert.equal(result.receipt.cleanup.confirmed, true);
    assert.equal(result.receipt.subscription.status, 'verified-local-controls');
    assert.equal(result.receipt.costStatus, 'unknown');
    assert.equal(result.receipt.costUSD, null);
    assert.throws(() => process.kill(result.receipt.execution.pid, 0), { code: 'ESRCH' });
    assert.ok(!server.requests.some(value => value.includes('@ckanner/dsh-subagent-antigravity')), 'the bundled adapter must not require a registry publication');
  });

  await t.test('a direct DSH installation resolves its own bundled CLI without a global record', async () => {
    const record = path.join(context.home, '.config/flywheel/installation.json');
    const saved = await fs.readFile(record);
    const stale=JSON.parse(saved);stale.version='0.32.1';await fs.writeFile(record,JSON.stringify(stale));
    assert.throws(()=>resolveFlywheelBinary({},context.env),/launcher version does not match/);
    assert.equal(resolveFlywheelBinary({flywheelBinary:'/explicit/user/binary'},context.env),'/explicit/user/binary');
    await fs.unlink(record);
    try {
      assert.equal(resolveFlywheelBinary({}, context.env), await fs.realpath(cli));
      const status=await run(process.execPath,[cli,'control','runs','--cwd',context.project],context.env,context.project);
      assert.equal(status.code,0,status.stderr+status.stdout);assert.equal(JSON.parse(status.stdout).ok,true);
    } finally {await fs.writeFile(record,saved,{mode:0o600});}
  });

  await t.test('the bundled DSH worker drives the offline SDK from the installed package', async () => {
    const fixture = path.join(context.fixtures, 'offline-dsh.mjs');
    await fs.copyFile(path.resolve(__dirname, '../../plugins/dsh/test/fixtures/fake-dsh.js'), fixture);await fs.chmod(fixture,0o755);
    const input={protocol:1,type:'start',requestId:'installed-single-dsh',cwd:await fs.realpath(context.project),provider:'deepseek-account',model:'fixture-model',reasoningEffort:'high',permissionMode:'danger-full-access',prompt:'Offline package check',timeoutMs:5000};
    const result=await new Promise((resolve,reject)=>{
      const child=spawn(worker,[],{cwd:context.project,env:{...context.env,FLYWHEEL_DSH_BIN:fixture},stdio:['pipe','pipe','pipe']});
      let stdout='',stderr='';child.stdout.on('data',b=>stdout+=b);child.stderr.on('data',b=>stderr+=b);child.on('error',reject);
      const timer=setTimeout(()=>child.kill('SIGTERM'),10000);child.on('close',(code,signal)=>{clearTimeout(timer);resolve({code,signal,stdout,stderr});});child.stdin.end(JSON.stringify(input)+'\n');
    });
    assert.equal(result.code,0,result.stderr+result.stdout);assert.equal(result.signal,null);
    const last=JSON.parse(result.stdout.trim().split('\n').at(-1));assert.equal(last.type,'result');assert.equal(last.status,'ok');assert.equal(last.settled,true);
  });

  await t.test('default installation succeeds with an absent optional Claude adapter' , async () => {
    const doctor = await run(claudeCodeProWorker, ['--doctor'], context.env, context.project);
    assert.equal(doctor.code, 1, doctor.stdout + doctor.stderr);
    const result = JSON.parse(doctor.stdout);
    assert.equal(result.receipt, null);
    assert.equal(result.error.code, 'ADAPTER_UNAVAILABLE');
    assert.deepEqual(result.admission, { dispatched: false, cleanupConfirmed: true });
    assert.deepEqual(await fs.readFile(globalConfig), before);
  });

  await t.test('installed worker imports the real pinned plugin and executes an offline app-server', async () => {
    assertNoDesktopPeers(installedRequire.resolve('dsh-subagent-codex-pro/package.json'));
    const doctor = await run(codexProWorker, ['--doctor'], context.env, context.project);
    assert.equal(doctor.code, 0, doctor.stderr + doctor.stdout);
    assert.deepEqual(JSON.parse(doctor.stdout), { protocol: 1, type: 'doctor', package: 'dsh-subagent-codex-pro', version: '0.10.2',
      machineProtocol: 1, nativeVersion: 1, available: true, inferenceTested: false });
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
      model: 'gpt-6.2-sol', reasoningEffort: 'high', permissionMode: 'workspace-write',
      subscription: { version: 1, mode: 'subscription-only' },
      prompt: 'Offline installed package validation.', timeoutMs: 5000 }) + '\n');
    let exit;
    try { exit = await done; } finally { clearTimeout(timeout); }
    assert.equal(exit.code, 0, stderr + stdout);
    assert.equal(exit.signal, null);
    const frames = stdout.trim().split('\n').map(line => JSON.parse(line));
    const result = frames.at(-1);
    assert.deepEqual(result.adapter, { package: 'dsh-subagent-codex-pro', version: '0.10.2', protocol: 1 });
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
    assert.equal(result.receipt.subscription.status, 'verified-native-controls');
    assert.equal(result.receipt.subscription.authentication, 'chatgpt');
    const calls = (await fs.readFile(log, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
    assert.deepEqual(calls[0].argv, ['app-server', '--stdio']);
    assert.equal(calls.find(call => call.method === 'turn/start').params.effort, 'high');
    const turnIndex = calls.findIndex(call => call.method === 'turn/start');
    for (const method of ['account/read', 'config/read']) {
      const admissionIndex = calls.findIndex(call => call.method === method);
      assert.ok(admissionIndex >= 0 && admissionIndex < turnIndex, method + ' must precede prompt dispatch');
    }
    assert.equal(JSON.stringify(result).includes('never-copy-account@example.invalid'), false);
  });

  await t.test('the optional source tarball runs through the installed Claude worker', {
    skip: process.env.FLYWHEEL_CLAUDE_CODE_PRO_TARBALL ? false : 'set FLYWHEEL_CLAUDE_CODE_PRO_TARBALL to the reviewed 0.7.0 source candidate',
  }, async () => {
    const tarball = path.resolve(process.env.FLYWHEEL_CLAUDE_CODE_PRO_TARBALL);
    const manifest = JSON.parse(execFileSync('tar', ['-xOf', tarball, 'package/package.json'], { encoding: 'utf8' }));
    assert.equal(manifest.name, 'dsh-subagent-claude-code-pro');
    assert.equal(manifest.version, '0.7.0');
    assert.ok(manifest.exports?.['./machine'], 'the candidate must expose its public machine API');
    assert.deepEqual(manifest.dependencies, { '@anthropic-ai/claude-agent-sdk': '0.3.326' },
      'the candidate must use the pinned official SDK as a runtime dependency');
    const [npm, args] = process.env.npm_execpath ? [process.execPath, [process.env.npm_execpath]] : ['npm', []];
    const installation = await run(npm, [...args, 'install', '--global', '--prefix', context.prefix,
      '--ignore-scripts', '--no-audit', '--no-fund', tarball], context.env, context.project);
    assert.equal(installation.code, 0, installation.stdout + installation.stderr);
    assertNoDesktopPeers(installedRequire.resolve('dsh-subagent-claude-code-pro/package.json'));
    const doctor = await run(claudeCodeProWorker, ['--doctor'], context.env, context.project);
    assert.equal(doctor.code, 0, doctor.stdout + doctor.stderr);
    assert.deepEqual(JSON.parse(doctor.stdout), { protocol: 1, type: 'doctor', package: manifest.name,
      version: manifest.version, machineProtocol: 1, nativeVersion: 1, available: true, inferenceTested: false });
    const fixture = path.join(context.fixtures, 'offline-claude.mjs');
    await fs.copyFile(path.resolve(__dirname, '../../plugins/dsh/test/fixtures/fake-claude-code-pro.js'), fixture);
    await fs.chmod(fixture, 0o755);
    const child = spawn(claudeCodeProWorker, [], { cwd: context.project,
      env: { ...context.env, FLYWHEEL_CLAUDE_BIN: fixture }, stdio: ['pipe', 'pipe', 'pipe'] });
    let stdout = '', stderr = '';
    child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
    child.stdout.on('data', value => { stdout += value; }); child.stderr.on('data', value => { stderr += value; });
    child.stdin.on('error', () => {});
    const done = new Promise((resolve, reject) => { child.once('error', reject); child.once('close', (code, signal) => resolve({ code, signal })); });
    const timeout = setTimeout(() => child.kill('SIGTERM'), 15000);
    const definitions = { scout: { description: 'Inspect the candidate', prompt: 'Report findings', model: 'claude-sonnet-5-5', effort: 'xhigh' } };
    const imageBytes = await fs.readFile(path.resolve(__dirname, '../../plugins/dsh/test/fixtures/multimodal-reference.png'));
    const image = { mimeType: 'image/png', data: imageBytes.toString('base64') };
    child.stdin.write(JSON.stringify({ protocol: 2, type: 'start', requestId: 'actual-claude-install', cwd: await fs.realpath(context.project),
      model: 'claude-opus-5-5', reasoningEffort: 'high', permissionMode: 'danger-full-access', threadMode: 'persistent',
      subscription: { version: 1, mode: 'subscription-only' }, images: [image],
      native: { version: 1, agents: { enabled: true, definitions } },
      prompt: 'Offline installed Claude package validation.', timeoutMs: 5000 }) + '\n');
    let exit;
    try { exit = await done; } finally { clearTimeout(timeout); }
    assert.equal(exit.code, 0, stderr + stdout);
    assert.equal(exit.signal, null);
    const frames = stdout.trim().split('\n').map(line => JSON.parse(line));
    const result = frames.at(-1);
    assert.deepEqual(result.adapter, { package: manifest.name, version: manifest.version, protocol: 1 });
    assert.equal(result.receipt.ok, true);
    assert.equal(result.receipt.configuration.observed.cwd, await fs.realpath(context.project));
    assert.equal(result.receipt.requested.model, 'claude-opus-5-5');
    assert.equal(result.receipt.requested.effort, 'high');
    assert.deepEqual(result.receipt.requested.inputImages, { version: 1, images: [{ mimeType: image.mimeType,
      sha256: createHash('sha256').update(imageBytes).digest('hex'), bytes: imageBytes.length }] });
    assert.equal(result.receipt.subscription.extraUsageEnabled, false);
    assert.equal(result.receipt.configuration.modelEvidence.rootMatched, true);
    assert.equal(result.receipt.session.resumable, true);
    assert.equal(result.receipt.cleanup.confirmed, true);
    assert.equal(result.receipt.usage.totalTokens, 22);
    assert.equal(result.receipt.costUSD, null);
    assert.equal(result.receipt.reportedCostUSD, 0.12);
    assert.throws(() => process.kill(-result.receipt.execution.processGroupId, 0), { code: 'ESRCH' });
    const calls = (await fs.readFile(path.join(context.project, 'claude-fixture.jsonl'), 'utf8')).trim().split('\n').map(JSON.parse);
    assert.equal(calls.length, 1);
    assert.ok(calls[0].argv.includes('--allow-dangerously-skip-permissions'));
    assert.ok(calls[0].argv.includes('--permission-mode=bypassPermissions'));
    assert.deepEqual(calls[0].initialize.agents, definitions);
    assert.ok(calls[0].argv.includes('--effort=high'));
    assert.deepEqual(result.receipt.native.requested.agents.definitions, definitions);
    assert.deepEqual(calls[0].userContent[1], { type: 'image', source: {
      type: 'base64', media_type: image.mimeType, data: image.data } });
    const probes = (await fs.readFile(path.join(context.project, 'claude-preflight.jsonl'), 'utf8')).trim().split('\n').map(JSON.parse);
    assert.equal(probes.length, 1);
    assert.equal(probes[0].pid, calls[0].pid, 'installed subscription probe and image prompt use the same owned CLI');
    assert.deepEqual(await fs.readFile(globalConfig), before, 'opting into an adapter must not select or rewrite any loop');
  });

  await t.test('installed console edits an explicit project config with revision checking', async () => {
    const invoke = (action, editor) => JSON.parse(execFileSync(process.execPath,
      [cli, 'control', action, '--cwd', context.project], {
        env: context.env, cwd: context.project, input: JSON.stringify(editor), encoding: 'utf8', timeout: 10000,
      }));
    const read = invoke('config-edit', { scope: 'project' });
    assert.equal(read.ok, true);
    const saved = invoke('config-save', { scope: 'project', revision: read.editor.revision,
      values: { sessions: { mode: 'fresh', 'policy-version': 1 } } });
    assert.equal(saved.editor.applied, true);
    assert.equal(saved.editor.values.sessions.mode, 'fresh');
    assert.throws(() => invoke('config-save', { scope: 'project', revision: read.editor.revision, values: {} }));
    await fs.unlink(saved.editor.target);
  });

  await t.test('same-version setup is repeatable and preserves configuration', async () => {
    const second = await run(process.execPath, [cli, 'setup', '--no-global', '--prefix', context.prefix, '--dsh-bin', context.dsh, '--json'], context.env, context.project);
    assert.equal(second.code, 0, second.stderr + second.stdout);
    const result = JSON.parse(second.stdout);
    assert.equal(result.skills.count, 11);
    assert.equal(result.paths.launcher, report.paths.launcher);
    assert.deepEqual(await fs.readFile(globalConfig), before);
    assert.equal(result.configuration.preserved, true);
    assert.equal(result.configuration.path, globalConfig);
    assert.match(result.configuration.message, /升级不会替换旧模型/);
    assert.deepEqual(result.configuration.migration.args.slice(0, 4), ['config', 'migrate', '--preset', 'subscription-cli']);
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
    assert.equal(failure.steps.at(-1).summary.results.length, 11);
    assert.deepEqual(await fs.readFile(skill), userVersion);
  });
  const calls = (await fs.readFile(context.env.FLYWHEEL_DSH_FIXTURE_LOG, 'utf8')).trim().split('\n').map(JSON.parse);
  assert.ok(calls.some(args => args[3] === 'add' && args.at(-1) === `@ckanner/flywheel@${server.candidate.version}`));
  assert.ok(!server.requests.some(request => /flywheel-(darwin|linux)-/.test(request)), 'native engines must come from the single tarball');
  for (const bundled of ['dsh-subagent-codex-pro', 'dsh-preset-generator', '@ckanner/dsh-subagent-antigravity']) {
    assert.ok(!server.requests.some(request => request.includes(bundled)),
      `${bundled} must install from the reviewed bundle without fetching a registry release`);
  }
  assert.equal(await fs.stat(path.join(context.project, 'state.json')).then(() => true, () => false), false);
});

// This environment has its own home/profile and registry; it never touches the user's desktop.
test('split desktop installation migrates to the same single package and can repeat', {timeout:240000}, async t => {
  const server=await registry(path.resolve(process.env.FLYWHEEL_NPM_PACKAGES_DIR || 'dist-npm'));
  const context=await environment(server.url,server.candidate.version);
  t.after(async()=>{await server.close();await context.clean();});
  context.env.FLYWHEEL_DSH_FIXTURE_LEGACY='0.32.1';
  const first=await firstInstall(context,server.candidate.version);
  assert.equal(first.code,0,first.stdout+first.stderr);
  const report=JSON.parse(first.stdout),step=report.steps.find(s=>s.name==='dsh-plugin');
  assert.equal(step.migration.status,'migrated');
  const state=JSON.parse(await fs.readFile(context.env.FLYWHEEL_DSH_FIXTURE_STATE,'utf8'));
  assert.deepEqual(Object.keys(state),['@ckanner/flywheel']);
  assert.equal(state['@ckanner/flywheel'].version,server.candidate.version);
  const calls=(await fs.readFile(context.env.FLYWHEEL_DSH_FIXTURE_LOG,'utf8')).trim().split('\n').map(JSON.parse);
  assert.ok(calls.findIndex(a=>a[3]==='add') < calls.findIndex(a=>a[3]==='remove'));
  const root=await installedRoot(context);const config=path.join(context.home,'.config/flywheel/flywheel.config.json');const before=await fs.readFile(config);
  const second=await run(process.execPath,[path.join(root,'bin/flywheel.js'),'setup','--no-global','--prefix',context.prefix,'--dsh-bin',context.dsh,'--json'],context.env,context.project);
  assert.equal(second.code,0,second.stderr+second.stdout);assert.deepEqual(await fs.readFile(config),before);
  assert.equal(JSON.parse(second.stdout).steps.find(s=>s.name==='dsh-plugin').migration,undefined);
});
