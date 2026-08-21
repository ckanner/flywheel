// Gate: every shipped skill passes the frontmatter contract (name matching the
// directory, non-empty description, version) — the same check CI runs via
// `npm run skill-format-check` (modeled on larksuite/cli's skill-format-check).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { checkSkills } from '../scripts/skill-format-check.js';
import { repoRoot, listRepoSkills } from '../orchestrator/modules/install-skills.js';

test('skill-format: every shipped SKILL.md passes the frontmatter contract', () => {
  const skillsDir = join(repoRoot(), 'skills');
  const shipped = listRepoSkills(repoRoot());
  assert.ok(shipped.length >= 10, `expected the shipped skill set, got ${shipped.length}`);
  const errors = checkSkills(skillsDir);
  assert.deepEqual(errors, [], `violations:\n${errors.join('\n')}`);
});

test('skill-format: violations are reported — missing version, wrong name, no frontmatter', () => {
  const dir = mkdtempSync(join(tmpdir(), 'flywheel-skillfmt-'));
  const ok = join(dir, 'good-skill');
  mkdirSync(ok, { recursive: true });
  writeFileSync(join(ok, 'SKILL.md'), '---\nname: good-skill\ndescription: demo\nversion: 1.0.0\n---\n\n# good\n', 'utf8');

  const noVersion = join(dir, 'no-version');
  mkdirSync(noVersion, { recursive: true });
  writeFileSync(join(noVersion, 'SKILL.md'), '---\nname: no-version\ndescription: x\n---\n\n# x\n', 'utf8');

  const wrongName = join(dir, 'wrong-name');
  mkdirSync(wrongName, { recursive: true });
  writeFileSync(join(wrongName, 'SKILL.md'), '---\nname: other\ndescription: x\nversion: 1.0.0\n---\n\n# x\n', 'utf8');

  const noFm = join(dir, 'no-frontmatter');
  mkdirSync(noFm, { recursive: true });
  writeFileSync(join(noFm, 'SKILL.md'), '# nothing\n', 'utf8');

  const errors = checkSkills(dir);
  assert.ok(errors.some((e) => e.includes('no-version') && e.includes("version")));
  assert.ok(errors.some((e) => e.includes('wrong-name') && e.includes('does not match')));
  assert.ok(errors.some((e) => e.includes('no-frontmatter') && e.includes('frontmatter')));
  assert.ok(!errors.some((e) => e.includes('good-skill')));
});
