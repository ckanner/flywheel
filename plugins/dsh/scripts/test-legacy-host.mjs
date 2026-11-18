import { mkdtemp, copyFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../', import.meta.url));
const fixture = fileURLToPath(new URL('../test/fixtures/host-rc5/', import.meta.url));
const directory = await mkdtemp(join(tmpdir(), 'flywheel-rc5-host-'));
function checked(command, args, options) {
  const result = spawnSync(command, args, { stdio: 'inherit', timeout: 180000, killSignal: 'SIGTERM', ...options });
  if (result.error || result.signal || result.status !== 0) throw new Error(`Legacy host check failed: ${result.error?.code ?? result.signal ?? result.status}`);
}
try {
  for (const name of ['package.json', 'package-lock.json']) await copyFile(join(fixture, name), join(directory, name));
  checked('npm', ['ci', '--ignore-scripts', '--no-audit', '--no-fund', '--registry=https://registry.npmjs.org/'], { cwd: directory });
  for (const name of ['web-policy.js', 'planning-control.js', 'session-control.js', 'console-read.js', 'control.js', 'protocol.js', 'native.js', 'task-association.js']) await copyFile(join(root, 'src', name), join(directory, name));
  checked(process.execPath, ['--test', 'test/console-read.test.js', 'test/client-host.test.js', 'test/host-association.test.js', 'test/tool-runtime.test.js'], { cwd: root,
    env: { ...process.env, FLYWHEEL_TEST_DSH_HOST_DIR: directory } });
} finally {
  await rm(directory, { recursive: true, force: true });
}
