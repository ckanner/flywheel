import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import semver from 'semver';
import { evaluatePluginCompatibility } from './fixtures/dsh-rc1-plugin-compatibility.ts';

const manifest = JSON.parse(await readFile(new URL('../../../package.json', import.meta.url), 'utf8'));

test('the shipped manifest admits compatible-series upgrades without a per-release exemption', () => {
  for (const runtime of ['0.2.1-rc.5', '0.2.1-rc.99', '0.2.1', '0.2.2-rc.1',
    '0.2.2-rc.2', '0.2.2-rc.3', '0.2.2-rc.99', '0.2.2', '0.2.3-rc.1', '0.2.99']) {
    assert.equal(evaluatePluginCompatibility(manifest, {}, runtime), undefined, runtime);
    // Older hosts use the DSH peer ranges; these must not retain an exact allowlist.
    const { engines, ...peerManifest } = manifest;
    assert.equal(evaluatePluginCompatibility(peerManifest, {}, runtime), undefined, runtime);
  }
});

test('the compatibility floor and next minor including its prereleases remain blocked', () => {
  for (const runtime of ['0.2.0', '0.2.1-rc.4', '0.3.0-0', '0.3.0-alpha.1', '0.3.0-rc.1', '0.3.0', '1.0.0']) {
    const result = evaluatePluginCompatibility(manifest, {}, runtime);
    assert.ok(result, `runtime outside the compatible series ${runtime} must stay blocked`);
    assert.equal(result.exempted, false);
    assert.deepEqual(Object.keys(result.requirements), ['engines.dsh']);
    const { engines, ...peerManifest } = manifest;
    assert.ok(evaluatePluginCompatibility(peerManifest, {}, runtime), runtime);
  }
});

test('npm peer ranges admit the supported prerelease lines without includePrerelease overrides', () => {
  for (const [name, range] of Object.entries(manifest.peerDependencies)) {
    if (!name.startsWith('@deepseek-ai/dsh-')) continue;
    for (const version of ['0.2.1-rc.5', '0.2.1-rc.99', '0.2.2-rc.3', '0.2.2-rc.99', '0.2.2', '0.2.99']) {
      assert.ok(semver.satisfies(version, range), `${name} must accept ${version} during npm resolution`);
    }
  }
});
