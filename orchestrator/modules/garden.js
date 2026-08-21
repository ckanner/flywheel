// Garden Loop (DESIGN.md §4.6): low-frequency maintenance of the loop's own
// knowledge assets — distill recurring failure patterns from retro.md into
// skill/acceptance proposals, and flag decaying AGENTS.md/skills content.
// Garden only ever produces PROPOSALS for human review (garden-proposals.md);
// it never edits AGENTS.md or skills itself (改的是知识资产，需人审).

import { readFileSync, writeFileSync, existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { resolveBinding } from '../core/fallback.js';
import { createModuleContext, callAgent } from './module.js';

/**
 * Deterministic pre-pass over retro.md: normalized patterns that repeat
 * (>= 2). Packet ids, timestamps, and numbers are stripped so "the same"
 * failure counts together.
 */
export function extractFailurePatterns(retroContent, { minCount = 2 } = {}) {
  const buckets = new Map();
  for (const rawLine of retroContent.split('\n')) {
    const line = rawLine.trim();
    // Failure evidence only: "failed=0" success lines and summary lines like
    // "defects(review-failed)=0" are NOT failures (P1-14, user-verified).
    if (!/failed=[1-9]\d*\b|INBOX|escalated/.test(line)) continue;
    const normalized = line
      .replace(/\d{4}-\d{2}-\d{2}T[\d:.]+Z?/g, '')
      .replace(/M\d{2,}-T\d{2,}/g, '<packet>')
      .replace(/M-\d{2,}/g, '<milestone>')
      .replace(/\d+/g, '<n>')
      .replace(/\s+/g, ' ')
      .trim();
    if (!normalized) continue;
    const bucket = buckets.get(normalized) ?? { pattern: normalized, count: 0, lines: [] };
    bucket.count += 1;
    bucket.lines.push(line);
    buckets.set(normalized, bucket);
  }
  return [...buckets.values()].filter((b) => b.count >= minCount).sort((a, b) => b.count - a.count);
}

function collectInputs(taskDir, cwd) {
  const retroFile = join(taskDir, 'retro.md');
  const retro = existsSync(retroFile) ? readFileSync(retroFile, 'utf8') : '';
  const agentsFile = join(cwd, 'AGENTS.md');
  const agents = existsSync(agentsFile) ? readFileSync(agentsFile, 'utf8') : null;
  const skillsDir = join(cwd, 'skills');
  const skills = existsSync(skillsDir)
    ? readdirSync(skillsDir, { withFileTypes: true }).filter((d) => d.isDirectory()).map((d) => d.name)
    : [];
  return { retro, agents, skills };
}

function buildGardenPrompt({ patterns, retroTail, agents, skills }) {
  return [
    'You are the gardener of the Flywheel system (Garden Loop). You maintain the loop\'s OWN knowledge assets. You produce PROPOSALS ONLY — a human reviews and applies them. You never edit AGENTS.md or skills yourself.',
    '',
    '## Recurring failure patterns from retro.md (deterministic pre-pass)',
    patterns.length === 0
      ? '(none repeated >= 2 times)'
      : patterns.map((p) => `- x${p.count}: ${p.pattern}\n  evidence: ${p.lines.slice(0, 3).map((l) => `"${l}"`).join('; ')}`).join('\n'),
    '',
    '## retro.md (tail)',
    retroTail || '(empty)',
    '',
    '## Current AGENTS.md',
    agents ?? '(none in this repo)',
    '',
    '## Installed skills',
    skills.length > 0 ? skills.map((s) => `- ${s}`).join('\n') : '(none)',
    '',
    '## Produce garden-proposals.md content',
    '1. For every recurring failure pattern: a proposal to prevent it — a new skill, an acceptance rule, or a contract-template improvement.',
    '2. For suspected decay (AGENTS.md/skills entries that contradict the current implementation shown above): a cleanup proposal.',
    '3. Every proposal MUST cite evidence: quote the retro line or name the file and section it refers to. No evidence, no proposal.',
    '4. Keep it to the most valuable proposals (<= 5).',
    '',
    '## Output format (strict)',
    '## GP-01 — <short title>',
    '- type: failure-pattern | decay',
    '- evidence: <quoted retro line(s) or file reference>',
    '- proposal: <what a human should do>',
    '(repeat GP-02, GP-03, ...)',
  ].join('\n');
}

/**
 * Validate proposal structure: every GP block must carry an evidence line
 * citing retro.md, AGENTS.md, or skills/ (evidence-carrying is the whole
 * point — unsupported proposals are noise).
 */
export function validateGardenProposals(content) {
  const errors = [];
  const proposals = [];
  const blocks = content.split(/(?=^## GP-\d+ — )/m).filter((b) => b.startsWith('## GP-'));
  if (blocks.length === 0) {
    return { ok: false, proposals, errors: ['no GP blocks found'] };
  }
  for (const block of blocks) {
    const title = /^## (GP-\d+ — .+)$/m.exec(block)?.[1] ?? '(untitled)';
    const evidence = /^- evidence: (.+)$/m.exec(block)?.[1];
    if (!evidence) {
      errors.push(`${title}: missing evidence line`);
    } else if (!/(retro\.md|AGENTS\.md|skills\/)/.test(evidence)) {
      errors.push(`${title}: evidence does not cite retro.md / AGENTS.md / skills/`);
    }
    if (!/^- proposal: .+/m.test(block)) errors.push(`${title}: missing proposal line`);
    if (!/^- type: (failure-pattern|decay)/m.test(block)) errors.push(`${title}: missing/unknown type`);
    proposals.push({ title, evidence: evidence ?? null });
  }
  return { ok: errors.length === 0, proposals, errors };
}

/**
 * One garden pass: analyze retro + knowledge assets with K3, write proposals.
 * @returns {Promise<{ok: boolean, file: string, proposals: number, errors: string[]}>}
 */
export async function runGarden({ taskDir, cwd = process.cwd(), config, drivers, log = () => {}, sleep }) {
  const mctx = createModuleContext({ taskDir, cwd, config, drivers, log, sleep });
  const { retro, agents, skills } = collectInputs(taskDir, cwd);
  const patterns = extractFailurePatterns(retro);
  log(`garden inputs: retro ${retro.length} chars, ${patterns.length} recurring patterns, AGENTS.md ${agents ? 'present' : 'absent'}, ${skills.length} skills`);

  const prompt = buildGardenPrompt({
    patterns,
    retroTail: retro.split('\n').slice(-40).join('\n'),
    agents,
    skills,
  });
  const binding = resolveBinding(config, 'garden');
  const r = await callAgent(mctx, { role: 'garden', binding, prompt });
  if (!r.ok) {
    return { ok: false, file: null, proposals: 0, errors: [`garden agent call failed: ${r.errorCode}`] };
  }

  const file = join(taskDir, 'garden-proposals.md');
  const header = `# Garden proposals — ${new Date().toISOString()}\n\n` +
    `> Proposals only. A HUMAN reviews and applies them — Garden never edits AGENTS.md or skills (§4.6).\n\n`;
  const validation = validateGardenProposals(r.text);
  writeFileSync(file, `${header}${r.text}\n`, 'utf8');
  if (!validation.ok) {
    log(`garden proposals failed validation: ${validation.errors.join('; ')} (raw output kept for the human)`);
  }
  return { ok: validation.ok, file, proposals: validation.proposals.length, errors: validation.errors };
}
