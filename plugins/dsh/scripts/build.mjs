import { chmod, copyFile, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { transform } from 'lightningcss';

const root = fileURLToPath(new URL('../', import.meta.url));
await mkdir(join(root, 'dist'), { recursive: true });
for (const name of ['index.js', 'control.js', 'task-association.js', 'worker.js', 'bridge.js', 'policy.js', 'protocol.js', 'installation.js', 'codex-pro-worker.js', 'codex-pro-bridge.js', 'claude-code-pro-worker.js', 'claude-code-pro-bridge.js', 'machine-worker.js']) {
  await copyFile(join(root, 'src', name), join(root, 'dist', name));
}
// Compile plugin-owned CSS Modules. The rendered component owns its style tag;
// external plugins must not write outside their React slot subtree.
const stylesheet = transform({
  filename: 'client.module.css',
  code: await readFile(join(root, 'src/client.module.css')),
  cssModules: { pattern: 'flywheel_[local]_[hash]' },
});
const classMap = Object.fromEntries(Object.keys(stylesheet.exports).sort()
  .map(name => [name, stylesheet.exports[name].name]));
// The web loader consumes a lazy factory, not an ES module or a global React.
const client = ((await readFile(join(root, 'src/client.js'), 'utf8')) + '\n' + (await readFile(join(root, 'src/client-console.js'), 'utf8')) + '\n' + (await readFile(join(root, 'src/client-tasks.js'), 'utf8')))
  .replace('__FLYWHEEL_CSS_CLASSES__', JSON.stringify(classMap))
  .replace('__FLYWHEEL_CSS_TEXT__', JSON.stringify(stylesheet.code.toString()));
await writeFile(join(root, 'dist/client.js'),
  'window.__ModuleLoader__.load({id:"@ckanner/dsh-plugin-flywheel",factory:(require)=>{\n' +
  'const module={exports:{}};const exports=module.exports;\n' +
  client + '\nreturn module.exports;\n}});\n');
await chmod(join(root, 'dist/worker.js'), 0o755);
await chmod(join(root, 'dist/codex-pro-worker.js'), 0o755);
await chmod(join(root, 'dist/claude-code-pro-worker.js'), 0o755);
console.log('Built host plugin, native Conversation View, DSH SDK, Codex Pro and Claude Code Pro workers.');
