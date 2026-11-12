import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { evaluatePluginCompatibility } from './fixtures/dsh-rc1-plugin-compatibility.ts';

const manifest = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));

test('the shipped manifest passes the real DSH gate for every checked runtime without exemptions', () => {
  for (const runtime of ['0.2.1-rc.5', '0.2.2-rc.1', '0.2.2-rc.2']) {
    assert.equal(evaluatePluginCompatibility(manifest, {}, runtime), undefined, runtime);
  }
  for (const runtime of ['0.2.1-rc.4', '0.2.2-alpha.3', '0.2.2-rc.3', '0.2.2', '0.3.0']) {
    const result = evaluatePluginCompatibility(manifest, {}, runtime);
    assert.ok(result, `unverified runtime ${runtime} must stay blocked`);
    assert.equal(result.exempted, false);
    assert.deepEqual(Object.keys(result.requirements), ['engines.dsh']);
  }
});
