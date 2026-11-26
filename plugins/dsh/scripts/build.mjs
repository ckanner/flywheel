import { chmod, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join, resolve } from 'node:path';
import { transform } from 'lightningcss';
import { build as esbuild } from 'esbuild';

const root = fileURLToPath(new URL('../', import.meta.url));
const packageRoot = process.argv[2] === '--package-root' && process.argv[3] ? resolve(process.argv[3]) : undefined;
if (process.argv.length !== 2 && (process.argv.length !== 4 || !packageRoot)) {
  throw new Error('usage: build.mjs [--package-root <directory>]');
}

const outputs = [
  { directory: join(root, 'dist'), moduleExtension: '.js' },
  ...(packageRoot ? [{ directory: join(packageRoot, 'dist'), moduleExtension: '.mjs' }] : []),
];
for (const output of outputs) await mkdir(output.directory, { recursive: true });

const moduleFiles = [
  'task-directory.js', 'image-contract.js', 'subscription-controls.js',
  'codex-subscription-proxy.js', 'dsh-activity.js', 'kimi-print-session.js',
  'planning-control.js', 'session-control.js', 'native-doctor.js',
  'kimi-native-worker.js', 'kimi-native-bridge.js', 'kimi-native-runtime.js',
  'kimi-tower-candidate.js', 'native-jobs.js', 'managed-lcu.js', 'native.js',
  'native-policy.js', 'native-resources.js', 'console-read.js', 'trace-parser.js', 'workspaces.js',
  'index.js', 'control.js', 'task-association.js', 'worker.js', 'prewarm.js',
  'bridge.js', 'policy.js', 'protocol.js', 'installation.js',
  'codex-pro-worker.js', 'codex-pro-bridge.js', 'claude-code-pro-worker.js',
  'claude-code-pro-bridge.js', 'machine-worker.js', 'antigravity-bridge.js',
  'antigravity-worker.js', 'model-catalog.js', 'model-catalog-contract.js',
  'model-catalog-process.js', 'model-catalog-worker.js',
];

for (const name of moduleFiles) {
  const source = await readFile(join(root, 'src', name), 'utf8');
  for (const output of outputs) {
    const body = output.moduleExtension === '.mjs'
      ? source.replace(/(['"])(\.\/[^'"]+)\.js\1/g, (whole, quote, relative) =>
          moduleFiles.includes(relative.slice(2) + '.js') ? quote + relative + '.mjs' + quote : whole)
      : source;
    await writeFile(join(output.directory, name.replace(/\.js$/, output.moduleExtension)), body);
  }
}

// Compile plugin-owned CSS Modules with lightningcss. No global document.head writes at import time.
const cssPath = join(root, 'src', 'client', 'styles', 'flywheel.module.css');
const stylesheet = transform({
  filename: 'flywheel.module.css',
  code: await readFile(cssPath),
  cssModules: { pattern: 'flywheel_[local]_[hash]' },
  minify: true,
});
const classMap = Object.fromEntries(
  Object.keys(stylesheet.exports).sort().map(name => [name, stylesheet.exports[name].name]),
);
const cssText = stylesheet.code.toString();

const stylesPlugin = {
  name: 'flywheel-virtual-styles',
  setup(build) {
    build.onResolve({ filter: /^virtual:flywheel-styles$/ }, () => ({
      path: 'virtual:flywheel-styles',
      namespace: 'flywheel-styles',
    }));
    build.onLoad({ filter: /.*/, namespace: 'flywheel-styles' }, () => ({
      contents: `export const styles = ${JSON.stringify(classMap)};\nexport const cssText = ${JSON.stringify(cssText)};\n`,
      loader: 'js',
    }));
  },
};

const reactJsxShimPlugin = {
  name: 'flywheel-react-jsx-shim',
  setup(build) {
    build.onResolve({ filter: /^react\/jsx-(dev-)?runtime$/ }, () => ({
      path: 'react-jsx-runtime-shim',
      namespace: 'flywheel-jsx-shim',
    }));
    build.onResolve({ filter: /^react-dom$/ }, () => ({
      path: 'react-dom-shim',
      namespace: 'flywheel-react-dom-shim',
    }));
    build.onLoad({ filter: /.*/, namespace: 'flywheel-jsx-shim' }, () => ({
      contents: [
        'import React from "react";',
        'export const Fragment = React.Fragment;',
        'export function jsx(type, props, key) {',
        '  const nextProps = key !== undefined ? Object.assign({}, props, { key }) : props;',
        '  return React.createElement(type, nextProps);',
        '}',
        'export const jsxs = jsx;',
        'export const jsxDEV = jsx;',
      ].join('\n'),
      loader: 'js',
    }));
    build.onLoad({ filter: /.*/, namespace: 'flywheel-react-dom-shim' }, () => ({
      contents: [
        'export function createPortal(children) { return children; }',
        'export default { createPortal };',
      ].join('\n'),
      loader: 'js',
    }));
  },
};

// Bundle TypeScript + TSX + Streamdown + highlight.js into a single classic DSH web bundle.
// Use classic React.createElement transform + JSX runtime shim so the bundle depends ONLY
// on `react` (compatible with both DSH 0.2.1-rc.5 and 0.2.2-rc.3+ module loaders).
const bundled = await esbuild({
  entryPoints: [join(root, 'src', 'client', 'index.tsx')],
  bundle: true,
  format: 'cjs',
  platform: 'browser',
  target: 'es2022',
  jsx: 'transform',
  jsxFactory: 'React.createElement',
  jsxFragment: 'React.Fragment',
  banner: { js: 'var React = require("react");' },
  external: ['react'],
  plugins: [stylesPlugin, reactJsxShimPlugin],
  write: false,
  logLevel: 'warning',
  legalComments: 'none',
});

const clientBody = bundled.outputFiles[0]?.text;
if (!clientBody) throw new Error('esbuild produced no client bundle output');

const clientBundle =
  'window.__ModuleLoader__.load({id:"@ckanner/flywheel",factory:function(require){\n' +
  'var module={exports:{}};var exports=module.exports;\n' +
  '(function(module,exports,require){\n' +
  clientBody +
  '\n})(module,exports,require);\n' +
  'return module.exports;\n' +
  '}});\n';

for (const output of outputs) {
  await writeFile(join(output.directory, 'client.js'), clientBundle, 'utf8');
  for (const worker of [
    'codex-subscription-proxy', 'worker', 'codex-pro-worker',
    'claude-code-pro-worker', 'kimi-native-worker', 'antigravity-worker',
    'model-catalog-worker',
  ]) {
    await chmod(join(output.directory, worker + output.moduleExtension), 0o755);
  }
}

console.log('Built Flywheel host, v2 4-slot UI client and machine workers.');
