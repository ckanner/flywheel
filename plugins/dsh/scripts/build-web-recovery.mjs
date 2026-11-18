import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { build, version as esbuildVersion } from 'esbuild';

const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const contract = 'chatgpt-web/read-only-recovery/1';
const publicEntry = 'dsh-plugin-chatgpt/web-recovery';
const allowedImports = new Set(['node:fs', 'node:fs/promises', 'node:crypto', 'node:path', 'node:os']);
const allowedPluginFiles = new Set(['lib/web/recovery.js', 'lib/web/config.js', 'lib/web/policy.js',
  'lib/web/protocol.js', 'lib/web/image-policy.js', 'lib/web/media.js', 'lib/web/store.js',
  'lib/web/browser/storage.js', 'lib/abort.js']);
const dependencies = new Map([['ajv', '8.17.1'], ['fast-uri', '3.1.9'], ['fast-deep-equal', '3.1.3'], ['json-schema-traverse', '1.0.0']]);

/** Bundle only the public recovery reader; never initialize the provider/SDK. */
export async function buildWebRecovery(root, outputs) {
  const vendor = JSON.parse(await readFile(join(root, 'vendor/chatgpt.json'), 'utf8'));
  const lock = JSON.parse(await readFile(join(root, 'package-lock.json'), 'utf8'));
  const archive = join(root, 'vendor', vendor.tarball);
  if (vendor.package !== 'dsh-plugin-chatgpt' || vendor.version !== '0.2.0-alpha.1' ||
      !/^[a-f0-9]{40}$/.test(vendor.sourceCommit) || !/^[a-f0-9]{40}$/.test(vendor.sourceTree) ||
      vendor.sha256 !== sha256(await readFile(archive)) ||
      lock.packages['node_modules/dsh-plugin-chatgpt']?.integrity !== vendor.integrity) {
    throw new Error('Web recovery source must match its exact reviewed vendor archive and lock');
  }
  const result = await build({ absWorkingDir: root, entryPoints: ['src/web-recovery-reader.js'],
    bundle: true, format: 'esm', platform: 'node', target: 'node22.18', treeShaking: true,
    write: false, metafile: true, legalComments: 'none', outfile: 'web-recovery-reader.mjs' });
  const [output] = Object.values(result.metafile.outputs);
  const imports = [...new Set(output.imports.map(item => item.path))].sort();
  if (result.outputFiles.length !== 1 || Object.keys(result.metafile.outputs).length !== 1 ||
      output.exports.length !== 1 || output.exports[0] !== 'readWebInvocationRecovery' ||
      output.imports.some(item => !item.external || !allowedImports.has(item.path))) {
    throw new Error('Web recovery bundle must expose only its reader and bounded Node filesystem/protocol imports');
  }
  const packages = new Map(), inputs = [];
  for (const name of Object.keys(result.metafile.inputs).sort()) {
    const bytes = await readFile(resolve(root, name));
    inputs.push({ path: name, bytes: bytes.length, sha256: sha256(bytes) });
    if (name === 'src/web-recovery-reader.js') continue;
    const match = /^(.*node_modules\/((?:@[^/]+\/)?[^/]+))\/(.+)$/.exec(name);
    if (!match) throw new Error('Unexpected Web recovery bundle source: ' + name);
    const [, directory, packageName, relative] = match;
    const manifest = JSON.parse(await readFile(join(root, directory, 'package.json'), 'utf8'));
    if (packageName === vendor.package) {
      if (!allowedPluginFiles.has(relative) || manifest.version !== vendor.version ||
          !bytes.equals(execFileSync('tar', ['-xOf', archive, 'package/' + relative], { maxBuffer: 8 * 1024 * 1024 }))) {
        throw new Error('Web recovery contains an unreviewed provider module: ' + relative);
      }
    } else if (dependencies.get(packageName) !== manifest.version || lock.packages[directory]?.version !== manifest.version) {
      throw new Error('Unexpected Web recovery schema dependency: ' + packageName);
    }
    packages.set(directory, { name: packageName, version: manifest.version, license: manifest.license });
  }
  if (packages.size !== dependencies.size + 1) throw new Error('Web recovery license/package inventory is incomplete');
  let notices = '# Bundled ChatGPT Web read-only recovery licenses\n\n' +
    'Built from the public `' + publicEntry + '` entry. Source identity and exact\n' +
    'input/output fingerprints are recorded in `web-recovery-reader.manifest.json`.\n\n';
  const bundledPackages = [];
  for (const [directory, manifest] of [...packages.entries()].sort((a, b) => a[1].name.localeCompare(b[1].name))) {
    const names = (await readdir(join(root, directory))).filter(name => /^(license|licence)(?:\.(?:txt|md))?$/i.test(name)).sort();
    if (!names.length) throw new Error('Web recovery dependency lacks its license: ' + manifest.name);
    notices += '## ' + manifest.name + '@' + manifest.version + ' (' + manifest.license + ')\n\n';
    for (const name of names) notices += (await readFile(join(root, directory, name), 'utf8')).trim() + '\n\n';
    bundledPackages.push(manifest);
  }
  const code = result.outputFiles[0].contents;
  for (const { directory, moduleExtension } of outputs) {
    const file = 'web-recovery-reader' + moduleExtension;
    const manifest = { version: 1, contract, publicEntry,
      source: { package: vendor.package, version: vendor.version, commit: vendor.sourceCommit, tree: vendor.sourceTree, archiveSHA256: vendor.sha256 },
      build: { tool: 'esbuild', version: esbuildVersion, target: 'node22.18' },
      file, bytes: code.length, sha256: sha256(code), imports, bundledPackages, inputs,
      licenses: { file: 'web-recovery-reader.LICENSES.md', bytes: Buffer.byteLength(notices), sha256: sha256(notices) } };
    await writeFile(join(directory, file), code);
    await writeFile(join(directory, manifest.licenses.file), notices);
    await writeFile(join(directory, 'web-recovery-reader.manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  }
}
