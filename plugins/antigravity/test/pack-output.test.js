import assert from 'node:assert/strict'
import test from 'node:test'
import { parsePackOutput } from '../scripts/pack-output.js'

const name = 'dsh-subagent-antigravity', version = '0.1.0'
const packed = { name, version, filename: `${name}-${version}.tgz`, integrity: 'sha512-test', files: [{ path: 'lib/machine.js' }] }

test('npm 11 array and npm 12 name-keyed object retain the same complete package record', () => {
  for (const output of [[packed], { [name]: packed }]) {
    assert.deepEqual(parsePackOutput(JSON.stringify(output), name, version), packed)
  }
})

test('pack output must contain exactly one non-null object', () => {
  for (const output of [null, true, 1, 'packed', {}, [], packed, [packed, packed], [null], [[]], [[packed]],
    { wrong: packed }, { [name]: packed, extra: packed }, { [name]: null }, { [name]: [packed] }]) {
    assert.throws(() => parsePackOutput(JSON.stringify(output), name, version), /Unexpected npm pack output/)
  }
})

test('both npm output forms bind name, version and the exact safe tarball filename', () => {
  for (const edit of [
    { name: 'another-package' }, { name: undefined },
    { version: '0.2.0' }, { version: undefined },
    { filename: 'another-package-0.1.0.tgz' }, { filename: undefined },
    { filename: '../dsh-subagent-antigravity-0.1.0.tgz' },
    { filename: '/tmp/dsh-subagent-antigravity-0.1.0.tgz' },
    { filename: 'subdir\\dsh-subagent-antigravity-0.1.0.tgz' },
  ]) {
    const mismatched = { ...packed, ...edit }
    for (const output of [{ [name]: mismatched }, [mismatched]]) {
      assert.throws(() => parsePackOutput(JSON.stringify(output), name, version), /Unexpected npm pack output/)
    }
  }
  const scoped = { name: '@example/plugin', version: '1.2.3', filename: 'example-plugin-1.2.3.tgz' }
  assert.deepEqual(parsePackOutput(JSON.stringify({ [scoped.name]: scoped }), scoped.name, scoped.version), scoped)
})

test('malformed or mixed npm stdout is rejected rather than selecting a partial record', () => {
  for (const output of ['', '{', `notice\n${JSON.stringify(packed)}`, `${JSON.stringify(packed)}\n${JSON.stringify(packed)}`]) {
    assert.throws(() => parsePackOutput(output, name, version), /npm pack returned invalid JSON/)
  }
})
