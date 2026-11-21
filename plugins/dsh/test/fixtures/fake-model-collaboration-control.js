#!/usr/bin/env node
import { appendFile } from 'node:fs/promises';
const args = process.argv.slice(2);
const option = key => args.includes(key) ? args[args.indexOf(key) + 1] : '';
let raw = '';
for await (const chunk of process.stdin) raw += chunk;
const input = raw ? JSON.parse(raw) : null;
if (process.env.TEST_MODEL_COLLAB_LOG) await appendFile(process.env.TEST_MODEL_COLLAB_LOG, JSON.stringify({ args, input }) + '\n');
const response = { protocol: 1, ok: true, running: false, taskDir: option('--task-dir'), cwd: option('--cwd'), state: null };
if (args[1] === 'models') {
  const source = { antigravity: 'agy models', 'codex-pro': 'codex app-server model/list', 'claude-code-pro': 'Claude Agent SDK initialization.models' }[input.harness] ?? 'unsupported';
  response.modelCatalog = { ...input, status: source === 'unsupported' ? 'unsupported' : 'available', source, scope: 'native-client-catalog',
    models: source === 'unsupported' ? [] : [{ id: 'gemini-pro-orchestrator-neon', label: 'Orchestrator Neon', efforts: [''], defaultEffort: '' }],
    truncated: false, inferencePerformed: false, accountEntitlement: 'unknown', limitations: [], ...JSON.parse(process.env.TEST_MODEL_COLLAB_PATCH ?? '{}') };
} else if (args[1] === 'node') {
  response.state = { revision: 7, packets: {} };
  response.node = { id: option('--packet'), revision: 7, packet: {}, imageInputEditor: {
    packet: option('--packet'), revision: 7, contractFingerprint: 'a'.repeat(64), plan: { revision: 3, digest: 'b'.repeat(64) },
    status: 'known', editable: true, reason: 'Review required.', value: null, ...JSON.parse(process.env.TEST_MODEL_COLLAB_PATCH ?? '{}'),
  } };
} else {
  response.state = { revision: 8, status: 'paused', planning: { version: 1 } };
  if (process.env.TEST_MODEL_COLLAB_RECEIPT !== 'missing') response['planning-control'] = {
    action: input.action, 'request-id': input['request-id'], status: 'applied', ...JSON.parse(process.env.TEST_MODEL_COLLAB_PATCH ?? '{}'),
  };
}
process.stdout.write(JSON.stringify(response));
