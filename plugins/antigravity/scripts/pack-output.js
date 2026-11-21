// npm 11 returns an array; npm 12 keys packed records by package name.
// Normalize only these two shapes and bind the result to our expected package.
export function parsePackOutput(raw, name, version) {
  let parsed
  try { parsed = JSON.parse(raw) }
  catch (error) { throw new Error('npm pack returned invalid JSON', { cause: error }) }
  let packed
  if (Array.isArray(parsed) && parsed.length === 1) packed = parsed[0]
  else if (parsed && typeof parsed === 'object' && !Array.isArray(parsed) &&
           Object.keys(parsed).length === 1 && Object.hasOwn(parsed, name)) packed = parsed[name]
  const filename = `${name.replace(/^@/, '').replace('/', '-')}-${version}.tgz`
  if (!packed || typeof packed !== 'object' || Array.isArray(packed) ||
      packed.name !== name || packed.version !== version || packed.filename !== filename) {
    throw new Error(`Unexpected npm pack output: expected exactly ${name}@${version} as ${filename}`)
  }
  return packed
}
