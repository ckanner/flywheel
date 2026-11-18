'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { PLATFORMS, META } = require('../../../scripts/release-preflight.js');
function manifests(version) {
  const meta = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../../package.json'), 'utf8'));
  meta.version = version;
  return { [META]: meta };
}
function writeContents(directory, manifest) {
  const put = (relative, contents) => {
    const file = path.join(directory, relative);fs.mkdirSync(path.dirname(file), {recursive:true});fs.writeFileSync(file, contents);
  };
  const native = { protocol:1, version:manifest.version, platforms:{} };
  for (const platform of PLATFORMS) {
    const bytes=Buffer.from('fixture native binary '+platform);
    put(`native/${platform}/flywheel`,bytes);
    native.platforms[platform]={bytes:bytes.length,sha256:crypto.createHash('sha256').update(bytes).digest('hex')};
  }
  put('native/manifest.json',JSON.stringify(native));
  const real = manifests(manifest.version)[META];
  for (const file of [...Object.values(real.bin),...Object.values(real.exports),'cordis.patch.yml','LICENSE','THIRD_PARTY_NOTICES.md']) {
    if(file.replace(/^\.\//,'')!=='package.json')put(file,'// packed fixture '+file+'\n');
  }
  put('node_modules/dsh-subagent-codex-pro/package.json',JSON.stringify({name:'dsh-subagent-codex-pro',version:'0.10.2'}));
  put('node_modules/dsh-preset-generator/package.json',JSON.stringify({name:'dsh-preset-generator',version:'0.4.0'}));
  const vendor = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../../plugins/dsh/vendor/chatgpt.json'), 'utf8'));
  put('plugins/dsh/vendor/chatgpt.json', JSON.stringify(vendor));
  const reader = Buffer.from('export const readWebInvocationRecovery = () => { throw new Error("fixture"); };\n');
  const licenses = Buffer.from('Fixture Web reader license inventory.\n');
  const fingerprint = (file, bytes) => ({ file, bytes: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex') });
  put('dist/web-recovery-reader.mjs', reader);
  put('dist/web-recovery-reader.LICENSES.md', licenses);
  put('dist/web-recovery-reader.manifest.json', JSON.stringify({ version: 1, contract: 'chatgpt-web/read-only-recovery/1',
    publicEntry: 'dsh-plugin-chatgpt/web-recovery', source: { package: vendor.package, version: vendor.version,
      commit: vendor.sourceCommit, tree: vendor.sourceTree, archiveSHA256: vendor.sha256 }, imports: [],
    ...fingerprint('web-recovery-reader.mjs', reader), licenses: fingerprint('web-recovery-reader.LICENSES.md', licenses) }));
  put('package.json',JSON.stringify(manifest));
}
module.exports={manifests,writeContents};
