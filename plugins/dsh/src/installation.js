import { accessSync, constants, lstatSync, readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { homedir } from 'node:os';
import { isAbsolute, join } from 'node:path';

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
    if (error.code === 'ENOENT') return 'flywheel';
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
  return launcher.path;
}
