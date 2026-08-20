import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, readlinkSync, lstatSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { installSkills, listRepoSkills, skillFrontmatterOk, repoRoot } from '../orchestrator/modules/install-skills.js';

function fakeHome(withSkills = ['alpha-skill', 'beta-skill']) {
  const home = mkdtempSync(join(tmpdir(), 'flywheel-home-'));
  const root = mkdtempSync(join(tmpdir(), 'flywheel-repo-'));
  for (const name of withSkills) {
    mkdirSync(join(root, 'skills', name), { recursive: true });
    writeFileSync(join(root, 'skills', name, 'SKILL.md'), `---\nname: ${name}\ndescription: test skill ${name}\n---\n\n# ${name}\n`);
  }
  return { home, root };
}

test('repo skills inventory: all 10 repo skills pass the name+description check', () => {
  const names = listRepoSkills(repoRoot());
  assert.equal(names.length, 10);
  for (const n of names) {
    assert.ok(skillFrontmatterOk(join(repoRoot(), 'skills', n)), n);
  }
});

test('install-skills: default symlinks every skill into ~/.agents/skills', () => {
  const { home, root } = fakeHome();
  const { results, targetBase } = installSkills({ homeDir: home, root });
  assert.equal(results.length, 2);
  assert.ok(results.every((r) => r.action === 'linked'));
  for (const r of results) {
    assert.equal(lstatSync(r.dest).isSymbolicLink(), true);
    assert.equal(readlinkSync(r.dest), join(root, 'skills', r.name));
  }
  assert.equal(targetBase, join(home, '.agents', 'skills'));
});

test('install-skills: idempotent — a second run reports "already installed" and changes nothing', () => {
  const { home, root } = fakeHome();
  installSkills({ homeDir: home, root });
  const second = installSkills({ homeDir: home, root });
  assert.ok(second.results.every((r) => r.action === 'skipped'));
  assert.ok(second.results.every((r) => r.reason === 'already installed'));
  // still the same symlink
  assert.equal(readlinkSync(join(home, '.agents', 'skills', 'alpha-skill')), join(root, 'skills', 'alpha-skill'));
});

test('install-skills: --copy makes a real directory snapshot', () => {
  const { home, root } = fakeHome();
  const { results } = installSkills({ homeDir: home, root, copy: true });
  assert.ok(results.every((r) => r.action === 'copied'));
  const dest = join(home, '.agents', 'skills', 'alpha-skill');
  assert.equal(lstatSync(dest).isSymbolicLink(), false);
  assert.match(readFileSync(join(dest, 'SKILL.md'), 'utf8'), /name: alpha-skill/);
});

test('install-skills: --force replaces an existing entry', () => {
  const { home, root } = fakeHome();
  const dest = join(home, '.agents', 'skills', 'alpha-skill');
  mkdirSync(dest, { recursive: true });
  writeFileSync(join(dest, 'SKILL.md'), 'stale content');
  // without --force: skipped, stale stays
  const skipped = installSkills({ homeDir: home, root });
  assert.equal(skipped.results.find((r) => r.name === 'alpha-skill').action, 'skipped');
  assert.match(readFileSync(join(dest, 'SKILL.md'), 'utf8'), /stale content/);
  // with --force: replaced by the symlink
  const forced = installSkills({ homeDir: home, root, force: true });
  assert.equal(forced.results.find((r) => r.name === 'alpha-skill').action, 'linked');
  assert.equal(lstatSync(dest).isSymbolicLink(), true);
  assert.match(readFileSync(join(dest, 'SKILL.md'), 'utf8'), /name: alpha-skill/);
});

test('install-skills: --project installs into <project>/.agents/skills', () => {
  const { root } = fakeHome();
  const project = mkdtempSync(join(tmpdir(), 'flywheel-proj-'));
  const { targetBase, results, scope } = installSkills({ root, project });
  assert.equal(targetBase, join(project, '.agents', 'skills'));
  assert.match(scope, /project/);
  assert.ok(results.every((r) => r.action === 'linked'));
  assert.equal(readlinkSync(join(targetBase, 'beta-skill')), join(root, 'skills', 'beta-skill'));
});

test('install-skills: a skill whose frontmatter lacks name/description is reported as error', () => {
  const { home, root } = fakeHome(['broken-skill']);
  writeFileSync(join(root, 'skills', 'broken-skill', 'SKILL.md'), '# no frontmatter at all\n');
  const { results } = installSkills({ homeDir: home, root });
  assert.equal(results[0].action, 'error');
  assert.match(results[0].reason, /name\/description/);
});
