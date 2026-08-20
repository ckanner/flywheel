// Shared test fixture: a minimal Flywheel task (mini spec + 2 packets +
// state.json + mock-driver config) inside a tmp directory.

import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export const SPEC_MD = `---
spec-version: 1
title: Mini fixture task
goal: Prove the orchestrator loop end to end.
non-goals: [anything real]
milestones:
  - id: M-01
    title: The only milestone
    gate: L1 green for all packets.
requirements:
  - id: R-01
    title: First thing works
    milestone: M-01
    verification: test -f build/M01-T01.txt
  - id: R-02
    title: Second thing works
    milestone: M-01
    verification: test -f build/M01-T02.txt
---

# Mini spec

R-01 and R-02 map to M-01.
`;

export function packetMd(id, requirement, extra = {}) {
  const maxRetries = extra.maxRetries ?? 3;
  const onFail = extra.onFail ?? 'return-to-planner';
  return `---
packet-id: ${id}
contract-version: 1
goal: Make ${id} produce its artifact.
requirements: [${requirement}]
route: default
needs-live-credentials: false
context-files: []
cache-scope: []
boundaries:
  must-not-touch: [forbidden/]
  max-files: 5
acceptance:
  L1:
    - {cmd: "test -f build/${id}.txt", expect: "exit 0"}
review-rubric: |
  Pass line: score >= 0.8. Artifact exists and is honest.
escalation: {max-retries: ${maxRetries}, on-fail: ${onFail}}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

Work instructions for ${id}.
`;
}

export function mockConfig(overrides = {}) {
  return {
    version: 1,
    drivers: { plan: 'mock', execute: 'mock', review: 'mock', judge: 'mock', intake: 'mock', garden: 'mock' },
    loops: {
      plan: { harness: 'kimi', model: 'kimi-code/k3', channel: 'kimi-subscription', thinking: 'max' },
      review: { harness: 'pi', model: 'opencode-go/deepseek-v4-pro', channel: 'opencode-go', thinking: 'max' },
      judge: { harness: 'pi', model: 'kimi-coding/k3', channel: 'kimi-coding', thinking: 'max' },
      intake: { harness: 'pi', model: 'opencode-go/deepseek-v4-flash', channel: 'opencode-go', thinking: 'max' },
      garden: { harness: 'kimi', model: 'kimi-code/k3', channel: 'kimi-subscription', thinking: 'max' },
    },
    routes: {
      frontend: { harness: 'kimi', model: 'kimi-code/k3', channel: 'kimi-subscription', thinking: 'max' },
      default: { harness: 'pi', model: 'opencode-go/deepseek-v4-flash', channel: 'opencode-go', thinking: 'max' },
      hard: { harness: 'pi', model: 'kimi-coding/k3', channel: 'kimi-coding', thinking: 'max' },
    },
    channels: {
      'opencode-go': { 'env-key': 'OPENCODE_API_KEY', fallback: 'opencode' },
      opencode: {
        'env-key': 'OPENCODE_API_KEY',
        models: { default: 'opencode/deepseek-v4-flash', hard: 'opencode/deepseek-v4-pro', review: 'opencode/deepseek-v4-pro', judge: 'opencode/deepseek-v4-pro' },
        fallback: null,
      },
      'kimi-subscription': { 'env-key': null, fallback: 'kimi-coding' },
      'kimi-coding': {
        'env-key': 'KIMI_API_KEY',
        harness: 'pi',
        models: { plan: 'kimi-coding/k3', frontend: 'kimi-coding/k3', hard: 'kimi-coding/k3', judge: 'kimi-coding/k3' },
        fallback: 'opencode',
      },
    },
    budget: { enforce: 'soft', 'daily-token-limit': 10_000_000 },
    watchdog: { 'idle-timeout-ms': 5000, 'wall-timeout-ms': 30_000, 'kill-grace-ms': 500 },
    retry: { 'base-delay-ms': 10, 'max-delay-ms': 40, jitter: 0, 'max-transport-retries': 2 },
    credentials: { 'poll-interval-ms': 10, 'timeout-ms': 200 },
    intake: {
      sources: [
        { name: 'ci-failures', type: 'command', cmd: 'printf \'{"title": "nightly CI red on main", "summary": "auth tests failed twice"}\n{"title": "upgrade lodash to 5.x", "summary": "CVE-2026-1234"}\n\'' },
      ],
    },
    'defect-injection': { 'every-n-units': 5, 'test-file-ratio-boost': 0.5 },
    verify: { 'command-timeout-ms': 10_000 },
    ...overrides,
  };
}

export function initialState(slug = 'minitask') {
  return {
    'schema-version': 1,
    slug,
    revision: 0,
    status: 'pending',
    'current-milestone': 'M-01',
    'plan-session': null,
    milestones: { 'M-01': { status: 'pending', gate: null, 'closed-at': null } },
    packets: {
      'M01-T01': packetState(),
      'M01-T02': packetState(),
    },
    budget: {
      enforce: 'soft',
      'daily-token-limit': 10_000_000,
      daily: {},
      totals: { input: 0, output: 0, reasoning: 0, 'cache-read': 0, 'cache-write': 0, tokens: 0, 'cost-usd': 0 },
      cache: { 'hit-rate': 0, 'by-provider': {} },
      warnings: [],
    },
    'last-progress-at': null,
    inbox: [],
  };
}

function packetState() {
  return {
    milestone: 'M-01',
    status: 'pending',
    route: 'default',
    attempts: 0,
    'failed-attempts': 0,
    'plan-revisions': 0,
    interrupted: false,
    'needs-live-credentials': false,
    artifacts: [],
    exit: null,
    'base-ref': null,
    commit: null,
    evidence: { executor: null, l1: null, review: null, 'defect-injection': null },
    'last-failure-signature': null,
    'retry-history': [],
    'updated-at': null,
  };
}

/**
 * @returns {{root: string, taskDir: string}} root = fake repo root (cwd), taskDir = docs/minitask
 */
export function makeFixture({ slug = 'minitask', withState = true, config = mockConfig() } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'flywheel-fixture-'));
  const taskDir = join(root, 'docs', slug);
  mkdirSync(taskDir, { recursive: true });
  writeFileSync(join(taskDir, 'spec.md'), SPEC_MD, 'utf8');
  writeFileSync(join(taskDir, 'task-packet-M01-T01.md'), packetMd('M01-T01', 'R-01'), 'utf8');
  writeFileSync(join(taskDir, 'task-packet-M01-T02.md'), packetMd('M01-T02', 'R-02'), 'utf8');
  writeFileSync(join(taskDir, 'flywheel.config.json'), JSON.stringify(config, null, 2), 'utf8');
  if (withState) {
    writeFileSync(join(taskDir, 'state.json'), `${JSON.stringify(initialState(slug), null, 2)}\n`, 'utf8');
  }
  return { root, taskDir, slug };
}

function git(root, args) {
  return execFileSync('git', ['-C', root, '-c', 'user.name=test', '-c', 'user.email=test@localhost', ...args], {
    encoding: 'utf8',
  });
}

/**
 * Git-backed fixture: the whole fixture (contracts included) is committed,
 * like a real target repository after $flywheel-new. Extra files (e.g. an
 * existing test suite) can be added before the initial commit.
 *
 * @param {string[]} [extraFiles] repo-relative paths to create and commit
 */
export function makeGitFixture({ slug = 'minitask', config = mockConfig(), extraFiles = [] } = {}) {
  const fx = makeFixture({ slug, withState: true, config });
  for (const rel of extraFiles) {
    mkdirSync(join(fx.root, rel.split('/').slice(0, -1).join('/')), { recursive: true });
    writeFileSync(join(fx.root, rel), `// ${rel}\n`, 'utf8');
  }
  git(fx.root, ['init', '-q']);
  git(fx.root, ['add', '-A']);
  git(fx.root, ['commit', '-q', '-m', 'chore: initial contracts']);
  return fx;
}
