// Only the plugin evidence inspector is replaced. The original worker journal,
// read-only recovery function and downstream Go protocol parser are real.
import { readFileSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { recoverWebWorker, WEB_RECOVERY_VERSION } from '../../src/web-recovery.js';

const query = JSON.parse(readFileSync(0, 'utf8'));
const journal = JSON.parse(await readFile(join(query.recoveryDirectory, 'worker.json'), 'utf8'));
const scenario = process.argv[2];
if (!['completed','retired','unsent'].includes(scenario)) throw new Error('invalid offline recovery scenario');
const evidence = { version: 1, contract: WEB_RECOVERY_VERSION, identity: journal.checkpoint.identity, checkpointDigest: journal.checkpointDigest,
  revision: 'a'.repeat(64), disposition: scenario === 'completed' ? 'committed' : scenario === 'unsent' ? 'not-dispatched' : 'retired', binding: journal.ready.web,
  facts: { version: 1, ...journal.checkpoint.identity, dispatch: scenario === 'unsent' ? 'not-dispatched' : 'dispatched',
    outcome: scenario === 'completed' ? 'completed' : 'failed', remoteGeneration: 'idle', localCleanupConfirmed: true, usageStatus: 'unknown' },
  terminalMain: scenario === 'completed' ? [{ text: journal.business.text, hostCommitId: 'original-committed-main' }] : [] };
process.stdout.write(JSON.stringify(await recoverWebWorker(query, { inspect: async () => evidence })) + '\n');
