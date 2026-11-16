#!/usr/bin/env node
import { appendFileSync, readFileSync } from 'node:fs';

const option = name => {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : '';
};
const raw = readFileSync(0, 'utf8');
const command = raw ? JSON.parse(raw) : {};
const log = process.env.TEST_SESSION_LOG;
if (log) {
  appendFileSync(log, JSON.stringify({
    command,
    proof: process.env.FLYWHEEL_SESSION_USER_ACTION ?? null,
    dsh: process.env.DSH_SESSION_ID ?? null,
  }) + '\n');
}
process.stdout.write(JSON.stringify({
  protocol: 1, ok: true, running: false,
  taskDir: option('--task-dir'), cwd: option('--cwd'),
  state: { slug: 'session-fixture', status: 'paused', revision: command.revision ?? 1 },
  'session-control': {
    key: command.key, action: command.action,
    applied: true, revision: command.revision ?? 1, status: 'abandoned',
  },
}));
