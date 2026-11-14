import { accessSync, constants, lstatSync, readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { homedir } from 'node:os';
import { isAbsolute, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const ownPackage = JSON.parse(readFileSync(fileURLToPath(new URL('../package.json', import.meta.url)), 'utf8'));

function bundledLauncher() {
  // The private source harness has no complete distribution. Installed packages
  // own their CLI and must never silently borrow a different global version.
  const root = new URL('../', import.meta.url);
  if (ownPackage.private === true) return 'flywheel';
  const launcher = fileURLToPath(new URL('bin/flywheel.js', root));
  if (ownPackage.name !== '@ckanner/flywheel') throw new Error('Invalid bundled Flywheel package identity');
  const stat = lstatSync(launcher);
  if (!stat.isFile() || stat.isSymbolicLink()) throw new Error('Bundled Flywheel launcher is missing or invalid; reinstall the package');
  accessSync(launcher, constants.X_OK);
  return launcher;
}

// The npm installer owns this small receipt. A desktop host can find the exact
// launcher without relying on the login shell's PATH or editing a user patch.
export function resolveFlywheelBinary(config = {}, environment = process.env) {
  if (config.flywheelBinary !== undefined) {
    if (typeof config.flywheelBinary !== 'string' || !config.flywheelBinary.trim()) throw new Error('flywheelBinary must be a nonempty command or path');
    return config.flywheelBinary;
  }
  const filename = join(environment.HOME || homedir(), '.config', 'flywheel', 'installation.json');
  let receipt;
  try {
    const stat = lstatSync(filename);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 16384) throw new Error('invalid installation record');
    receipt = JSON.parse(readFileSync(filename, 'utf8'));
  } catch (error) {
    if (error.code === 'ENOENT') return bundledLauncher();
    throw new Error('Flywheel installation record is invalid; rerun flywheel setup or set flywheelBinary explicitly');
  }
  const launcher = receipt?.launcher;
  if (receipt?.protocol !== 1 || receipt?.owner !== '@ckanner/flywheel' || typeof launcher?.path !== 'string'
    || !isAbsolute(launcher.path) || !/^[a-f0-9]{64}$/.test(launcher.sha256)) {
    throw new Error('Flywheel installation record has an unsupported contract; rerun flywheel setup');
  }
  try {
    const stat = lstatSync(launcher.path);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 65536) throw new Error('invalid launcher');
    accessSync(launcher.path, constants.X_OK);
    if (createHash('sha256').update(readFileSync(launcher.path)).digest('hex') !== launcher.sha256) throw new Error('launcher was modified');
  } catch {
    throw new Error('Flywheel launcher is missing or changed; preserve custom edits and rerun setup or set flywheelBinary explicitly');
  }
  if (ownPackage.private !== true && receipt.version !== ownPackage.version) {
    throw new Error('Flywheel launcher version does not match this package; rerun flywheel setup for this version');
  }
  return launcher.path;
}
