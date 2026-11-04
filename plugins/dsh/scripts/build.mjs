import { chmod, copyFile, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

const root = fileURLToPath(new URL('../', import.meta.url));
await mkdir(join(root, 'dist'), { recursive: true });
for (const name of ['index.js', 'control.js', 'worker.js', 'bridge.js', 'policy.js', 'protocol.js', 'installation.js', 'codex-pro-worker.js', 'codex-pro-bridge.js']) {
  await copyFile(join(root, 'src', name), join(root, 'dist', name));
}
// The rc.5 web loader consumes this factory format, rather than ES module imports.
const client = await readFile(join(root, 'src/client.js'), 'utf8');
await writeFile(join(root, 'dist/client.js'),
  `window.__ModuleLoader__.load({id:"@ckanner/dsh-plugin-flywheel",factory:(require)=>{\n` +
  `const module={exports:{}};const exports=module.exports;\n${client}\nreturn module.exports;\n}});\n`);
await chmod(join(root, 'dist/worker.js'), 0o755);
await chmod(join(root, 'dist/codex-pro-worker.js'), 0o755);
console.log('Built host plugin, rc.5 client module, DSH SDK and Codex Pro workers.');
