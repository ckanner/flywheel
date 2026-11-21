import { basename, dirname, isAbsolute, join, normalize } from 'node:path';
import { directory, nonempty } from './protocol.js';

// Only preparation may name a new ledger. Resolve the nearest existing parent
// without creating anything; the Go engine owns creation and all task state.
export async function preparationDirectory(value) {
  nonempty(value, 'taskDir', 4096);
  if (!isAbsolute(value) || normalize(value) !== value || /[\0\r\n]/.test(value)) throw new Error('taskDir must be a normalized absolute directory');
  const missing = [];
  let parent = value;
  for (;;) {
    try { return join(await directory(parent, 'taskDir parent'), ...missing.reverse()); }
    catch (error) {
      if (error.code !== 'ENOENT' || dirname(parent) === parent) throw error;
      missing.push(basename(parent));
      parent = dirname(parent);
    }
  }
}
