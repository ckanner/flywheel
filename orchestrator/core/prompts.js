// Prompt builders (DESIGN.md §8 cache discipline): stable contract content
// goes FIRST (prefix-cache friendly, byte-identical across attempts), volatile
// data — attempt numbers, timestamps — goes in the FINAL section only.
//
// Reviewer information diet (§3, §4.3): the review prompt contains the packet,
// the git diff, L1 results, and any defect-injection outcome. It never
// contains the executor's transcript or self-narration.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const CACHE_SCOPE_CAP = 50_000; // ~50KB total for cache-scope file contents (§8.1)

/** Read cache-scope declared files into the stable prefix, capped (§8.1). */
function readCacheScope(cwd, files = []) {
  if (!cwd || files.length === 0) return [];
  const sections = ['## Cache-scope files (stable prefix, §8)'];
  let budget = CACHE_SCOPE_CAP;
  let truncated = false;
  for (const rel of files) {
    if (budget <= 0) {
      truncated = true;
      break;
    }
    let content;
    try {
      content = readFileSync(join(cwd, rel), 'utf8');
    } catch {
      sections.push(`### ${rel}\n(not found at runtime — skipped)`);
      continue;
    }
    if (content.length > budget) {
      content = `${content.slice(0, budget)}\n... [cache-scope truncated at ${CACHE_SCOPE_CAP} chars total]`;
      truncated = true;
    }
    budget -= content.length;
    sections.push(`### ${rel}\n${content}`);
    if (truncated) break;
  }
  if (truncated) sections.push('(further cache-scope files omitted by the 50KB cap)');
  return sections;
}

export function buildExecutePrompt({ packet, spec, taskDir, cwd = null, attempt }) {
  const stable = [
    'You are the executor for exactly one task packet of the Flywheel system. Do only this one thing.',
    '',
    '## Spec (stable contract content)',
    `Goal: ${spec.goal}`,
    `Non-goals: ${(spec['non-goals'] ?? []).join('; ') || 'none'}`,
    '',
    ...readCacheScope(cwd, packet['cache-scope']),
    '',
    '## Task packet (contract)',
    `packet-id: ${packet['packet-id']}`,
    `goal: ${packet.goal}`,
    `requirements: ${packet.requirements.join(', ')}`,
    `context-files: ${(packet['context-files'] ?? []).join(', ') || 'none'}`,
    `boundaries.must-not-touch: ${(packet.boundaries?.['must-not-touch'] ?? []).join(', ') || 'none'}`,
    `boundaries.max-files: ${packet.boundaries?.['max-files'] ?? 'unset'}`,
    '',
    '## Work discipline',
    '1. Read the contract and every context file before writing code.',
    '2. IMPLEMENT ONLY — never write or modify tests (v0.14 DC1). Acceptance tests are provided by the contract or already exist in the repo. Creating or modifying ANY test file fails this unit (git diff is checked).',
    '3. If the design is hard to test, that is a design defect: STOP and escalate so the Plan Loop can adjust — do not touch tests yourself.',
    '4. Run the acceptance commands yourself and report exactly which passed and which failed.',
    '5. NEVER declare the task done. Completion is judged externally by acceptance commands and review.',
    '6. If reality contradicts the design or the work needs a scope change: STOP and say so. Do not expand or shrink scope on your own.',
    '7. Do not touch must-not-touch paths; the git diff is checked after you finish.',
    `8. Finally write your done-report to ${taskDir}/reports/${packet['packet-id']}.done.json (absolute path) with EXACTLY these array keys: requirements-covered, changed-files, commands-run, failures. Facts only.`,
    '',
    '## Acceptance commands (L1, run by the orchestrator after you exit)',
    ...(packet.acceptance?.L1 ?? []).map((c) => `- ${c.cmd ?? c.script}  (expect ${c.expect})`),
  ];
  const volatile = [
    '',
    '## Runtime (volatile)',
    `attempt: ${attempt}`,
    `task directory: ${taskDir}`,
    `started-at: ${new Date().toISOString()}`,
  ];
  return [...stable, ...volatile].join('\n');
}

export function buildReviewPrompt({ packet, l1Evidence, diffSummary, diffText = null, defectInjection = null }) {
  const sections = [
    'You are the reviewer for one executed unit of the Flywheel system. Judge only what commands cannot.',
    '',
    '## Task packet (contract)',
    `packet-id: ${packet['packet-id']}`,
    `goal: ${packet.goal}`,
    `requirements: ${packet.requirements.join(', ')}`,
    `boundaries.must-not-touch: ${(packet.boundaries?.['must-not-touch'] ?? []).join(', ') || 'none'}`,
    '',
    '## Review rubric (pass line included)',
    packet['review-rubric'],
    '',
    '## L1 deterministic test results (ground truth, already green)',
    JSON.stringify(l1Evidence, null, 2),
  ];
  if (defectInjection) {
    sections.push(
      '',
      '## Defect-injection spot check (orchestrator-run, read-only for you)',
      JSON.stringify(defectInjection, null, 2),
      'Judge what this says about test assertion quality: an injected defect the tests did NOT catch proves the tests are fake/weak.',
    );
  }
  sections.push('', '## Changed files', diffSummary || '(unavailable)');
  if (diffText) {
    sections.push('', '## git diff (full, against the unit base ref)', diffText);
  } else {
    sections.push('', '## git diff', '(diff unavailable — judge from the changed-files list only, and say so in your reasons)');
  }
  sections.push(
    '',
    '## Your checks',
    '- Code quality, project conventions, design intent, security hazards.',
    '- Test assertion quality: coverage proves execution, not assertion validity. Remember the executor never writes tests (DC1) — any test file in the diff is already a boundary violation.',
    '- Coverage matrix: every numbered requirement above must have at least one verification + evidence; a missing one means return.',
    '- Scope: the diff must stay within the packet boundaries; any change to contract files (spec.md / task-packet-*.md) means return.',
    '',
    '## Output',
    'End your reply with a final line containing ONLY this JSON:',
    '{"verdict":"pass"|"return"|"verification-gap","score":0.0-1.0,"reasons":["..."]}',
    'Use "verification-gap" when the code looks right but the acceptance is too weak to prove it. The gap goes back to the Plan Loop to strengthen acceptance — the unit is NEVER completed on a gap (v0.14 DC2).',
  );
  return sections.join('\n');
}

export function buildPlanPrompt({ spec, state, milestone, packets, coldStart = false, retroTail = null }) {
  const lines = [
    'You are the planner of the Flywheel system. You only produce contract files; you never implement code and never expand requirement scope.',
    '',
    '## Spec (frozen)',
    `Goal: ${spec.goal}`,
    `Milestones: ${spec.milestones.map((m) => `${m.id} ${m.title}`).join('; ')}`,
    '',
    '## Current state',
    `status: ${state.status}, current milestone: ${state['current-milestone']}`,
    '',
    `## Task: issue/confirm packets for milestone ${milestone.id}`,
    `Existing packets: ${packets.map((p) => p['packet-id']).join(', ') || 'none'}`,
    'Confirm the packet sequence for this milestone, revise returned packets, or strengthen acceptance after a verification-gap. Scope changes are yours alone to make. Acceptance tests are YOURS to provide or strengthen — the executor never writes tests (v0.14 DC1).',
  ];
  if (coldStart) {
    // §4.1 session rebuild: the contract layer is the only memory — a fresh
    // session is booted from spec + state + retro, nothing else.
    lines.push('', '## Cold start note', 'This is a fresh planning session rebuilt from the contract layer (previous resident session died). All authoritative context is above/in the contract files on disk.');
    if (retroTail) lines.push('', '## retro.md (tail)', retroTail);
  }
  return lines.join('\n');
}

export function buildJudgePrompt({ spec, milestone, packets, state, gate }) {
  const requirements = spec.requirements.filter((r) => r.milestone === milestone.id);
  const matrix = requirements.map((r) => {
    const covering = packets.filter((p) => p.requirements.includes(r.id)).map((p) => p['packet-id']);
    const evidence = covering.map((id) => {
      const ps = state.packets[id];
      const l1 = ps?.evidence.l1?.pass === true ? 'L1✓' : 'L1—';
      const review = ps?.evidence.review?.verdict === 'pass' ? 'Review✓' : 'Review—';
      return `${id}(${l1}/${review})`;
    });
    return `- ${r.id} ${r.title} -> ${evidence.join(', ') || 'NO COVERAGE'}`;
  });
  return [
    'You are the judge for a milestone final verification of the Flywheel system. Fresh context, no prior involvement: re-verify what the execution/review loops produced.',
    '',
    '## Spec',
    `Goal: ${spec.goal}`,
    `Milestone: ${milestone.id} ${milestone.title}`,
    '',
    '## Requirement coverage matrix (anti-omission)',
    ...matrix,
    '',
    '## Milestone gate results (L2/L3, deterministic)',
    JSON.stringify(gate?.levels ?? {}, null, 2),
    '',
    '## Your checks',
    '- Every numbered requirement of this milestone has at least one verification with evidence; any gap fails.',
    '- The delivered work matches the spec intent; nothing outside the milestone scope was built.',
    '- The evidence is coherent (L1 commands, review verdicts, gate results tell one consistent story).',
    '',
    '## Output',
    'End your reply with a final line containing ONLY this JSON:',
    '{"verdict":"pass"|"return"|"verification-gap","score":0.0-1.0,"reasons":["..."]}',
  ].join('\n');
}

/** Parse the reviewer's final verdict JSON line. */
export function parseReviewVerdict(text) {
  if (!text) return { ok: false, error: 'empty review output' };
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean);
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i].replace(/^```(?:json)?|```$/g, '').trim();
    if (!line.startsWith('{')) continue;
    try {
      const v = JSON.parse(line);
      if (['pass', 'return', 'verification-gap'].includes(v.verdict)) {
        return {
          ok: true,
          verdict: {
            verdict: v.verdict,
            score: typeof v.score === 'number' ? v.score : null,
            reasons: Array.isArray(v.reasons) ? v.reasons : [],
          },
        };
      }
    } catch {
      // keep scanning upwards
    }
  }
  return { ok: false, error: 'no verdict JSON found in review output' };
}
