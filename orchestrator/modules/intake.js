// Intake Loop (DESIGN.md §4.5): scan task sources -> dedup candidates into
// triage-inbox.md -> LLM triage (pi + V4 Flash) advises on each. Intake NEVER
// freezes requirements and NEVER writes contracts: a human approves a
// candidate, and freezing still goes through $flywheel-new + human review
// (§12). This loop is the human/machine boundary for judgement calls.

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { createHash } from 'node:crypto';
import { resolveBinding } from '../core/fallback.js';
import { createModuleContext, callAgent } from './module.js';
import { gateEnv } from '../core/verify.js';

const defaultExec = promisify(execFile);

export function fingerprint(sourceName, title) {
  return createHash('sha256').update(`${sourceName}${title}`).digest('hex').slice(0, 12);
}

/** Fetch candidates from one configured source. `exec` is injectable for tests. */
export async function fetchSourceCandidates(source, { cwd, exec = defaultExec } = {}) {
  if (source.type === 'github-issues') {
    const args = ['issue', 'list', '--repo', source.repo, '--state', 'open', '--json', 'number,title,body,url,labels', '--limit', String(source.limit ?? 50)];
    if (Array.isArray(source.labels) && source.labels.length > 0) {
      args.push('--label', source.labels.join(','));
    }
    const { stdout } = await exec('gh', args, { cwd, env: gateEnv(), maxBuffer: 4 * 1024 * 1024 });
    const issues = JSON.parse(stdout);
    return issues.map((i) => ({
      title: i.title,
      summary: String(i.body ?? '').replace(/\s+/g, ' ').slice(0, 300),
      url: i.url,
      source: source.name,
      sourceType: source.type,
    }));
  }
  if (source.type === 'command') {
    const { stdout } = await exec('/bin/sh', ['-c', source.cmd], { cwd, env: gateEnv(), maxBuffer: 4 * 1024 * 1024 });
    const candidates = [];
    for (const line of stdout.split('\n')) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      if (trimmed.startsWith('{')) {
        try {
          const obj = JSON.parse(trimmed);
          if (obj.title) {
            candidates.push({ title: String(obj.title), summary: String(obj.summary ?? ''), url: obj.url ?? null, source: source.name, sourceType: source.type });
            continue;
          }
        } catch { /* fall through to plain-line */ }
      }
      candidates.push({ title: trimmed, summary: '', url: null, source: source.name, sourceType: source.type });
    }
    return candidates;
  }
  throw new Error(`unknown intake source type: ${source.type}`);
}

function parseExistingFingerprints(content) {
  const fps = new Set();
  for (const m of content.matchAll(/<!-- fp:([0-9a-f]{12}) -->/g)) {
    fps.add(m[1]);
  }
  return fps;
}

/** Parse the triage verdict JSON (建议接 recommend / 建议不接 skip / 需人判断 human). */
export function parseTriageVerdict(text) {
  if (!text) return { ok: false, error: 'empty triage output' };
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean);
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i].replace(/^```(?:json)?|```$/g, '').trim();
    if (!line.startsWith('{')) continue;
    try {
      const v = JSON.parse(line);
      if (['recommend', 'skip', 'human'].includes(v.verdict)) {
        return { ok: true, verdict: { verdict: v.verdict, reason: typeof v.reason === 'string' ? v.reason : '' } };
      }
    } catch { /* keep scanning */ }
  }
  return { ok: false, error: 'no verdict JSON found in triage output' };
}

function candidateSection(candidate, fp) {
  return [
    `## ${fp} — ${candidate.title}`,
    `- source: ${candidate.source} (${candidate.sourceType})`,
    `- url: ${candidate.url ?? '—'}`,
    `- summary: ${candidate.summary || '—'}`,
    `- triage: pending`,
    `- approval: pending  (human: change to "approved" to pick this up for $flywheel-new)`,
    `<!-- fp:${fp} -->`,
    '',
  ].join('\n');
}

function writeTriageVerdict(content, fp, verdict) {
  const blockRe = new RegExp(`(## ${fp} —[\\s\\S]*?)- triage: pending`, 'm');
  return content.replace(blockRe, `$1- triage: ${verdict.verdict} — ${verdict.reason}`);
}

/** Candidates the human marked approved (input for $flywheel-new; §4.5 人确认). */
export function listApprovedCandidates(taskDir) {
  const file = join(taskDir, 'triage-inbox.md');
  if (!existsSync(file)) return [];
  const content = readFileSync(file, 'utf8');
  const approved = [];
  for (const m of content.matchAll(/## ([0-9a-f]{12}) — ([^\n]+)\n([\s\S]*?)(?=\n## |\n*$)/g)) {
    if (/- approval: approved/.test(m[3])) {
      approved.push({ fingerprint: m[1], title: m[2].trim() });
    }
  }
  return approved;
}

function buildTriagePrompt(candidate) {
  return [
    'You are the triage advisor of the Flywheel system (Intake Loop). You ADVISE on task candidates; you never freeze requirements and never decide — the human does.',
    '',
    '## Candidate',
    `title: ${candidate.title}`,
    `source: ${candidate.source} (${candidate.sourceType})`,
    `url: ${candidate.url ?? '—'}`,
    `summary: ${candidate.summary || '—'}`,
    '',
    '## Advise',
    '- "recommend" (建议接): a well-scoped, verifiable engineering task with a clear acceptance oracle.',
    '- "skip" (建议不接): vague, non-engineering, duplicate-looking, or not actionable.',
    '- "human" (需人判断): judgement call — priority, product taste, unclear scope.',
    '',
    'End your reply with a final line containing ONLY this JSON:',
    '{"verdict":"recommend"|"skip"|"human","reason":"<one sentence>"}',
  ].join('\n');
}

/**
 * One intake pass: scan sources (or one), append new candidates, triage them.
 * @returns {Promise<{scanned: number, new: number, duplicates: number, triaged: object, inbox: string, ok: boolean}>}
 */
export async function runIntake({ taskDir, cwd = process.cwd(), config, drivers, source = null, log = () => {}, sleep, exec }) {
  const mctx = createModuleContext({ taskDir, cwd, config, drivers, log, sleep });
  const sources = config.intake?.sources ?? [];
  if (sources.length === 0) throw new Error('config intake.sources is empty — nothing to scan');
  const selected = source ? sources.filter((s) => s.name === source) : sources;
  if (source && selected.length === 0) throw new Error(`unknown intake source: ${source}`);

  // 1. Scan
  const all = [];
  for (const s of selected) {
    try {
      const candidates = await fetchSourceCandidates(s, { cwd, exec });
      log(`source ${s.name}: ${candidates.length} candidates`);
      all.push(...candidates);
    } catch (err) {
      log(`source ${s.name} failed: ${err.message} — skipped (source errors never block intake)`);
    }
  }

  // 2. Dedup against the existing inbox
  const inboxFile = join(taskDir, 'triage-inbox.md');
  const existing = existsSync(inboxFile) ? readFileSync(inboxFile, 'utf8') : '# Triage inbox (flywheel intake)\n\n';
  const knownFps = parseExistingFingerprints(existing);
  const fresh = [];
  let duplicates = 0;
  for (const c of all) {
    const fp = fingerprint(c.source, c.title);
    if (knownFps.has(fp)) {
      duplicates += 1;
    } else {
      fresh.push({ ...c, fp });
    }
  }

  // 3. Append new candidates
  let content = existing;
  for (const c of fresh) {
    content += `\n${candidateSection(c, c.fp)}\n`;
  }
  if (fresh.length > 0) writeFileSync(inboxFile, content, 'utf8');

  // 4. LLM triage for the new candidates (advice only, never a freeze decision)
  const binding = resolveBinding(config, 'intake');
  const triaged = { recommend: 0, skip: 0, human: 0, failed: 0 };
  for (const c of fresh) {
    const r = await callAgent(mctx, { role: 'intake', binding, prompt: buildTriagePrompt(c) });
    const verdict = r.ok ? parseTriageVerdict(r.text) : { ok: false };
    if (verdict.ok) {
      triaged[verdict.verdict.verdict] += 1;
      content = writeTriageVerdict(content, c.fp, verdict.verdict);
    } else {
      triaged.failed += 1;
      content = writeTriageVerdict(content, c.fp, { verdict: 'human', reason: `triage failed (${r.ok ? verdict.error : r.errorCode}) — human must look` });
      triaged.human += 1;
    }
    writeFileSync(inboxFile, content, 'utf8');
  }

  return {
    scanned: all.length,
    new: fresh.length,
    duplicates,
    triaged,
    inbox: inboxFile,
    ok: true,
  };
}
