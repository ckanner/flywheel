import { createInterface } from 'node:readline';
import { acquireGuiLease, settleGuiLease } from '../../src/native-resources.js';

const [directory, invocationId] = process.argv.slice(2);
const lines = createInterface({ input: process.stdin });
const send = value => process.stdout.write(JSON.stringify(value) + '\n');
try {
  const lease = await acquireGuiLease({ invocationId, runId: 'cross-process-run' }, {
    env: { FLYWHEEL_RESOURCE_DIR: directory }, signal: AbortSignal.timeout(10000),
    onWait: owner => send({ type: 'waiting', owner }),
  });
  send({ type: 'acquired', lease });
  for await (const line of lines) {
    if (line === 'release' || line === 'unknown') {
      send({ type: 'settled', receipt: await settleGuiLease(lease, line === 'release', { process: invocationId }) });
      break;
    }
  }
} catch (error) { send({ type: 'error', error: error.message }); process.exitCode = 1; }
finally { lines.close(); }
