// Independent native jobs preserve their own receipt and recovery semantics.
// They never return a Flywheel Review/Judge verdict or mutate task state.
import { open } from 'node:fs/promises';
import { constants } from 'node:fs';
import { isAbsolute } from 'node:path';
import { loadAdapter as loadCodex } from './codex-pro-bridge.js';
import { loadAdapter as loadClaude } from './claude-code-pro-bridge.js';

const OPERATIONS = new Set(['codex-review', 'claude-review', 'claude-team', 'claude-recover', 'claude-control']);
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);

export function nativeJobExitCode(receipt) {
  const outcome = receipt.receipt ?? receipt;
  return ['pending', 'unknown'].includes(receipt.status) || outcome.ok === false ||
    outcome.cleanup?.confirmed === false || outcome.native?.cleanup?.confirmed === false ? 3 : 0;
}

/** Input is the public package request plus an explicit journal directory. */
export async function runNativeJob(operation, input, { signal, loaders = { codex: loadCodex, claude: loadClaude } } = {}) {
  if (!OPERATIONS.has(operation) || !object(input)) throw new Error('Choose an independent native job and provide a JSON object');
  if (typeof input.recoveryDirectory !== 'string' || !isAbsolute(input.recoveryDirectory) || input.recoveryDirectory.includes('\0')) {
    throw new Error('A private absolute recoveryDirectory is required for every native job');
  }
  const api = await loaders[operation === 'codex-review' ? 'codex' : 'claude']();
  signal?.throwIfAborted();
  if (operation === 'claude-recover') return api.recoverNativeJob(input);
  if (operation === 'claude-control') return api.controlInteractiveTeam(input);
  const { recoveryDirectory, ...request } = input;
  const runtime = { recoveryDirectory, signal };
  if (operation === 'codex-review') {
    if (!request.native?.review || request.threadMode !== 'ephemeral' || request.threadId !== undefined) {
      throw new Error('Native Codex review requires its own fresh ephemeral request and explicit review target');
    }
    return api.runMachine(api.parseMachineRequest(request), runtime);
  }
  if (operation === 'claude-review') {
    api.validateNativeReviewRequest(request);
    return api.runNativeReview(request, runtime);
  }
  api.validateInteractiveTeamRequest(request);
  return api.runInteractiveTeam(request, runtime);
}

export async function main(args, { stdout = process.stdout, stderr = process.stderr } = {}) {
  if (args.length === 0 || (args.length === 1 && args[0] === '--help')) {
    stdout.write('Usage: flywheel native <codex-review|claude-review|claude-team|claude-recover|claude-control> --request /absolute/request.json\n' +
      'Requests use the pinned plugin public API plus recoveryDirectory. These jobs produce native evidence, never a formal Flywheel verdict.\n');
    return 0;
  }
  const controller = new AbortController();
  const cancel = () => controller.abort(new Error('Native job cancelled'));
  for (const name of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(name, cancel);
  try {
    if (args.length !== 3 || !OPERATIONS.has(args[0]) || args[1] !== '--request' || !isAbsolute(args[2])) throw new Error('Use flywheel native --help for the request-file interface');
    const file = await open(args[2], constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    let input;
    try {
      const stat = await file.stat();
      if (!stat.isFile() || stat.size < 2 || stat.size > 1024 * 1024) throw new Error('Native request must be a regular JSON file of at most 1 MiB');
      input = JSON.parse(await file.readFile('utf8'));
    } finally { await file.close(); }
    const receipt = await runNativeJob(args[0], input, { signal: controller.signal });
    stdout.write(JSON.stringify(receipt) + '\n');
    if (controller.signal.aborted) return 130;
    return nativeJobExitCode(receipt);
  } catch (error) {
    stderr.write(`flywheel native: ${error instanceof Error ? error.message : 'native operation failed'}\n`);
    return controller.signal.aborted ? 130 : 1;
  } finally {
    for (const name of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.off(name, cancel);
  }
}
