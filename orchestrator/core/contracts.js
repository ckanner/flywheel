// Contract file parsing (DESIGN.md §5): spec.md and task-packet-<id>.md are
// markdown files whose YAML frontmatter is the machine-readable contract.
// Validation here is intentionally a small hand-rolled check against the same
// field list as templates/*.schema.json (no extra dependencies allowed).

import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import YAML from 'yaml';

export class ContractError extends Error {}

export function parseFrontmatter(content) {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(content);
  if (!m) throw new ContractError('missing YAML frontmatter');
  let data;
  try {
    data = YAML.parse(m[1]);
  } catch (err) {
    throw new ContractError(`frontmatter is not valid YAML: ${err.message}`);
  }
  if (data === null || typeof data !== 'object' || Array.isArray(data)) {
    throw new ContractError('frontmatter must be a mapping');
  }
  return { data, body: content.slice(m[0].length) };
}

function requireFields(data, fields, file) {
  for (const f of fields) {
    if (!(f in data)) throw new ContractError(`${file}: missing required field "${f}"`);
  }
}

const EXPECT_RE = /^exit (\d+)$/;

/**
 * Parse an acceptance `expect` field. Contracts are validated at LOAD time —
 * a bad expect must fail fast in `validatePacket`, never crash the
 * orchestrator mid-run after the executor already finished (P1: an uncaught
 * throw here would turn one contract typo into a spawn-crash loop).
 */
export function parseExpect(expect) {
  const m = EXPECT_RE.exec(String(expect).trim());
  if (!m) throw new ContractError(`unsupported expect: ${expect} (want "exit N")`);
  return parseInt(m[1], 10);
}

const PACKET_ID_RE = /^M(\d{2,})-T(\d{2,})$/;

// Exactly the §5.1 field set — anything else is rejected (schema
// additionalProperties:false, 铁律 6).
const PACKET_ALLOWED_FIELDS = new Set([
  'packet-id', 'contract-version', 'goal', 'requirements', 'route',
  'needs-live-credentials', 'context-files', 'cache-scope', 'boundaries',
  'acceptance', 'review-rubric', 'escalation', 'done-report-format',
]);

/** M02-T03 -> M-02 (milestone id derivation convention, §5.1). */
export function milestoneOfPacket(packetId) {
  const m = PACKET_ID_RE.exec(packetId);
  if (!m) throw new ContractError(`invalid packet-id "${packetId}" (want M<NN>-T<NN>)`);
  return `M-${m[1]}`;
}

export function validatePacket(data, file = 'packet') {
  requireFields(data, [
    'packet-id', 'contract-version', 'goal', 'requirements', 'route',
    'context-files', 'boundaries',
    'acceptance', 'review-rubric', 'escalation', 'done-report-format',
  ], file);
  for (const key of Object.keys(data)) {
    if (!PACKET_ALLOWED_FIELDS.has(key)) {
      throw new ContractError(`${file}: unknown field "${key}" (allowed: ${[...PACKET_ALLOWED_FIELDS].join(', ')})`);
    }
  }
  milestoneOfPacket(data['packet-id']); // validates the pattern
  if (!['frontend', 'default', 'hard'].includes(data.route)) {
    throw new ContractError(`${file}: route must be frontend|default|hard`);
  }
  if (!Array.isArray(data.requirements) || data.requirements.length === 0) {
    throw new ContractError(`${file}: requirements must be a non-empty array`);
  }
  if ('needs-live-credentials' in data && typeof data['needs-live-credentials'] !== 'boolean') {
    throw new ContractError(`${file}: needs-live-credentials must be a boolean`);
  }
  if (!Array.isArray(data['context-files'])) {
    throw new ContractError(`${file}: context-files must be an array (use [] when none)`);
  }
  if (!data['context-files'].every((s) => typeof s === 'string')) {
    throw new ContractError(`${file}: context-files entries must be strings`);
  }
  if ('cache-scope' in data && (!Array.isArray(data['cache-scope']) || !data['cache-scope'].every((s) => typeof s === 'string'))) {
    throw new ContractError(`${file}: cache-scope must be an array of strings`);
  }
  if (typeof data.boundaries !== 'object' || data.boundaries === null || Array.isArray(data.boundaries)) {
    throw new ContractError(`${file}: boundaries must be a mapping ({must-not-touch: [...], max-files: n})`);
  }
  if ('max-files' in data.boundaries && (!Number.isInteger(data.boundaries['max-files']) || data.boundaries['max-files'] < 1)) {
    throw new ContractError(`${file}: boundaries.max-files must be an integer >= 1`);
  }
  if (typeof data['review-rubric'] !== 'string') {
    throw new ContractError(`${file}: review-rubric must be a string`);
  }
  if (typeof data['done-report-format'] !== 'object' || data['done-report-format'] === null || Array.isArray(data['done-report-format'])) {
    throw new ContractError(`${file}: done-report-format must be a mapping`);
  }
  const acc = data.acceptance;
  if (typeof acc !== 'object' || !Array.isArray(acc.L1) || acc.L1.length === 0) {
    throw new ContractError(`${file}: acceptance.L1 must be a non-empty array`);
  }
  // P1: validate every acceptance entry at load time. The orchestrator runs
  // these commands itself; a malformed expect must be caught when the
  // contract is read, not after a spawn (crash -> restart -> re-spawn loop).
  for (const level of ['L1', 'L2', 'L3']) {
    if (!(level in acc)) continue;
    if (!Array.isArray(acc[level])) throw new ContractError(`${file}: acceptance.${level} must be an array`);
    for (const entry of acc[level]) {
      if (typeof entry !== 'object' || entry === null || Array.isArray(entry)) {
        throw new ContractError(`${file}: acceptance.${level} entries must be {cmd|script, expect}`);
      }
      const command = entry.cmd ?? entry.script;
      if (typeof command !== 'string' || command.trim() === '') {
        throw new ContractError(`${file}: acceptance.${level} entry needs a non-empty cmd or script`);
      }
      parseExpect(entry.expect);
    }
  }
  const esc = data.escalation;
  if (typeof esc !== 'object' || !Number.isInteger(esc['max-retries']) || esc['max-retries'] < 1 || !['return-to-planner', 'inbox'].includes(esc['on-fail'])) {
    throw new ContractError(`${file}: escalation needs max-retries:int>=1 and on-fail:return-to-planner|inbox`);
  }
  return data;
}

export function validateSpec(data, file = 'spec.md') {
  requireFields(data, ['spec-version', 'title', 'goal', 'milestones', 'requirements'], file);
  if (!Array.isArray(data.milestones) || data.milestones.length === 0) {
    throw new ContractError(`${file}: milestones must be a non-empty array`);
  }
  if (!Array.isArray(data.requirements) || data.requirements.length === 0) {
    throw new ContractError(`${file}: requirements must be a non-empty array`);
  }
  const milestoneIds = new Set(data.milestones.map((m) => m.id));
  const reqIds = new Set();
  for (const r of data.requirements) {
    if (!r.id || !r.milestone) throw new ContractError(`${file}: every requirement needs id and milestone`);
    if (reqIds.has(r.id)) throw new ContractError(`${file}: duplicate requirement id ${r.id}`);
    reqIds.add(r.id);
    if (!milestoneIds.has(r.milestone)) {
      throw new ContractError(`${file}: requirement ${r.id} maps to unknown milestone ${r.milestone}`);
    }
  }
  return { ...data, _reqIds: reqIds, _milestoneIds: milestoneIds };
}

/** Load spec.md from a task directory. */
export function loadSpec(taskDir) {
  const file = join(taskDir, 'spec.md');
  if (!existsSync(file)) throw new ContractError(`spec.md not found in ${taskDir}`);
  const { data } = parseFrontmatter(readFileSync(file, 'utf8'));
  return validateSpec(data, file);
}

/** Load and validate all task-packet-*.md in a task directory, ordered by id. */
export function loadPackets(taskDir, spec = null) {
  const files = readdirSync(taskDir)
    .filter((f) => /^task-packet-.*\.md$/.test(f))
    .sort();
  if (files.length === 0) throw new ContractError(`no task-packet-*.md found in ${taskDir}`);
  const packets = files.map((f) => {
    const { data, body } = parseFrontmatter(readFileSync(join(taskDir, f), 'utf8'));
    validatePacket(data, f);
    return { ...data, _file: f, _body: body.trim() };
  });
  packets.sort((a, b) => a['packet-id'].localeCompare(b['packet-id']));
  if (spec) {
    for (const p of packets) {
      for (const r of p.requirements) {
        if (!spec._reqIds.has(r)) {
          throw new ContractError(`${p._file}: requirement ${r} not declared in spec.md`);
        }
      }
    }
  }
  return packets;
}
