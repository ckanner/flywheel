import { chmod, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join, resolve } from 'node:path';
import { transform } from 'lightningcss';

const root = fileURLToPath(new URL('../', import.meta.url));
const packageRoot = process.argv[2] === '--package-root' && process.argv[3] ? resolve(process.argv[3]) : undefined;
if (process.argv.length !== 2 && (process.argv.length !== 4 || !packageRoot)) throw new Error('usage: build.mjs [--package-root <directory>]');
const outputs = [{ directory: join(root, 'dist'), moduleExtension: '.js' }, ...(packageRoot ? [{ directory: join(packageRoot, 'dist'), moduleExtension: '.mjs' }] : [])];
for (const output of outputs) await mkdir(output.directory, { recursive: true });
const moduleFiles = ['dsh-activity.js', 'kimi-print-session.js', 'planning-control.js', 'session-control.js', 'native-doctor.js', 'kimi-native-worker.js', 'kimi-native-bridge.js', 'kimi-native-runtime.js', 'kimi-tower-candidate.js', 'native-jobs.js', 'managed-lcu.js', 'native.js', 'native-policy.js', 'native-resources.js', 'console-read.js', 'workspaces.js', 'index.js', 'control.js', 'task-association.js', 'worker.js', 'prewarm.js', 'bridge.js', 'policy.js', 'protocol.js', 'installation.js', 'codex-pro-worker.js', 'codex-pro-bridge.js', 'claude-code-pro-worker.js', 'claude-code-pro-bridge.js', 'machine-worker.js'];
for (const name of moduleFiles) {
  const source = await readFile(join(root, 'src', name), 'utf8');
  for (const output of outputs) {
    const body = output.moduleExtension === '.mjs' ? source.replace(/(['"])(\.\/[^'"]+)\.js\1/g, (whole, quote, relative) => moduleFiles.includes(relative.slice(2) + '.js') ? quote + relative + '.mjs' + quote : whole) : source;
    await writeFile(join(output.directory, name.replace(/\.js$/, output.moduleExtension)), body);
  }
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
const clientSources = ['client.js', 'client-console.js', 'client-tasks.js', 'client-planning.js', 'client-workbench.js', 'client-config.js', 'client-runs.js'];
const client = (await Promise.all(clientSources.map(name => readFile(join(root, 'src', name), 'utf8')))).join('\n')
  .replace('__FLYWHEEL_CSS_CLASSES__', JSON.stringify(classMap))
  .replace('__FLYWHEEL_CSS_TEXT__', JSON.stringify(stylesheet.code.toString()));
for (const output of outputs) {
  await writeFile(join(output.directory, 'client.js'),
    'window.__ModuleLoader__.load({id:"@ckanner/flywheel",factory:(require)=>{\n' +
    'const module={exports:{}};const exports=module.exports;\n' +
    client + '\nreturn module.exports;\n}});\n');
  for (const worker of ['worker', 'codex-pro-worker', 'claude-code-pro-worker', 'kimi-native-worker']) {
    await chmod(join(output.directory, worker + output.moduleExtension), 0o755);
  }
}
console.log('Built Flywheel host, Conversation View and machine workers.');
