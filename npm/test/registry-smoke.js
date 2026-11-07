"use strict";

const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { assertFlashAccountProfile } = require('./support/installed-profiles.js');
const { environment, firstInstall, installedRoot, run } = require('./support/install-harness.js');

(async () => {
  const version = process.env.FLYWHEEL_NPM_VERSION || require('../../package.json').version;
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error('an exact npm version is required');
  const context = await environment(process.env.FLYWHEEL_NPM_REGISTRY || 'https://registry.npmjs.org', version);
  try {
    const result = await firstInstall(context, version);
    assert.equal(result.code, 0, result.stdout + result.stderr);
    const report = JSON.parse(result.stdout);
    assert.equal(report.version, version);
    assert.equal(report.ok, true);
    assert.equal(report.skills.count, 11);
    assert.equal(report.steps.find(step => step.name === 'dsh-plugin').status, 'ok');
    const root = await installedRoot(context);
    const cli = path.join(root, 'bin', 'flywheel.js');
    const smoke = await run(process.execPath, [cli, 'template', 'flywheel.profiles.example.json'], context.env, context.project);
    assert.equal(smoke.code, 0, smoke.stderr);
    assertFlashAccountProfile(JSON.parse(smoke.stdout));
    assert.equal(JSON.parse(await fs.readFile(path.join(root, 'package.json'), 'utf8')).version, version);
    console.log(JSON.stringify({ ok: true, version, registry: process.env.FLYWHEEL_NPM_REGISTRY || 'https://registry.npmjs.org',
      skills: 11, actualNpmInstall: true, actualNativeCLI: true, desktopRegistration: 'CLI contract fixture',
      liveDesktopVerified: false, inferenceRequests: 0 }));
  } finally { await context.clean(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
