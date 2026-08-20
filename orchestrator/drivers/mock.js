// Mock driver: deterministic fake Plan/Execute/Review for tests and dry runs.
// Execute writes its deliverables (a build artifact + the done-report) exactly
// where the contract says, so the orchestrator's exit-contract, done-report
// parsing, L1 and state transitions all run for real against it.

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const MOCK_USAGE = { input: 120, output: 60, reasoning: 15, cacheRead: 40, cacheWrite: 10, totalTokens: 180 };

function mockResult(overrides = {}) {
  return {
    exitCode: 0,
    signal: null,
    timedOut: false,
    idleTimedOut: false,
    wallTimedOut: false,
    stdout: '',
    stderr: '',
    truncated: false,
    durationMs: 1,
    spawnError: null,
    harness: 'mock',
    provider: 'mock',
    model: 'mock-model',
    usage: { ...MOCK_USAGE },
    reasoningChars: 42,
    costUsd: 0.001,
    settled: true,
    text: '',
    ...overrides,
  };
}

/**
 * @param {object} [behaviors] optional per-packet overrides:
 *   { execute: { 'M01-T01': (ctx) => partialResult }, review: {...}, plan: {...} }
 */
export function createMockDriver(behaviors = {}) {
  return {
    name: 'mock',

    async plan({ milestone, packets }) {
      const behavior = behaviors.plan?.[milestone?.id];
      const base = mockResult({
        text: JSON.stringify({
          milestone: milestone?.id ?? null,
          packets: (packets ?? []).map((p) => p['packet-id']),
          action: 'issue',
        }),
      });
      return behavior ? { ...base, ...(await behavior({ milestone, packets })) } : base;
    },

    async execute({ packet, taskDir, cwd }) {
      const behavior = behaviors.execute?.[packet['packet-id']];
      const id = packet['packet-id'];
      // Deterministic deliverables: build artifact in the workspace root
      // (where L1 commands run), done-report inside the task directory.
      const workspace = cwd ?? taskDir;
      mkdirSync(join(workspace, 'build'), { recursive: true });
      mkdirSync(join(taskDir, 'reports'), { recursive: true });
      const artifact = `build/${id}.txt`;
      writeFileSync(join(workspace, artifact), `ok ${id}\n`, 'utf8');
      const doneReport = {
        'requirements-covered': packet.requirements,
        'changed-files': [artifact],
        'commands-run': (packet.acceptance?.L1 ?? []).map((c) => c.cmd ?? c.script),
        failures: [],
      };
      const reportPath = `reports/${id}.done.json`;
      writeFileSync(join(taskDir, reportPath), `${JSON.stringify(doneReport, null, 2)}\n`, 'utf8');
      const base = mockResult({ text: `mock executed ${id}` });
      return behavior ? { ...base, ...(await behavior({ packet, taskDir })) } : base;
    },

    async review({ packet }) {
      const behavior = behaviors.review?.[packet['packet-id']];
      const base = mockResult({
        text: JSON.stringify({ verdict: 'pass', score: 1, kind: 'ok', reasons: [] }),
      });
      return behavior ? { ...base, ...(await behavior({ packet })) } : base;
    },

    async judge({ milestone }) {
      const behavior = behaviors.judge?.[milestone?.id];
      const base = mockResult({
        text: JSON.stringify({ verdict: 'pass', score: 1, kind: 'ok', reasons: [] }),
      });
      return behavior ? { ...base, ...(await behavior({ milestone })) } : base;
    },

    async intake() {
      const base = mockResult({
        text: JSON.stringify({ verdict: 'recommend', reason: 'mock: looks actionable and verifiable' }),
      });
      return base;
    },

    async garden() {
      const base = mockResult({
        text: [
          '## GP-01 — Recurring transport failures need a retry-hardening note',
          '- type: failure-pattern',
          '- evidence: retro.md:"packet <packet>: INBOX — transport" (x2)',
          '- proposal: add a skill documenting the channel-fallback drill so future tasks reuse it',
        ].join('\n'),
      });
      return base;
    },
  };
}
