// install-skills: expose this repo's skills/ to kimi code and pi through the
// shared Agent Skills install point (~/.agents/skills/<name>, or a project's
// .agents/skills/). Default is a SYMLINK (single source of truth, edits live);
// --copy makes a snapshot for distribution. Idempotent: an existing entry is
// reported and skipped unless --force replaces it.

import { readdirSync, existsSync, lstatSync, mkdirSync, symlinkSync, cpSync, rmSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { homedir } from 'node:os';

export function repoRoot() {
  return join(dirname(fileURLToPath(import.meta.url)), '..', '..');
}

/** Skill directories in this repo (must contain SKILL.md with name+description). */
export function listRepoSkills(root = repoRoot()) {
  const skillsDir = join(root, 'skills');
  return readdirSync(skillsDir, { withFileTypes: true })
    .filter((d) => d.isDirectory() && existsSync(join(skillsDir, d.name, 'SKILL.md')))
    .map((d) => d.name)
    .sort();
}

function entryExists(path) {
  try {
    lstatSync(path); // lstat: a broken symlink still counts as "exists"
    return true;
  } catch {
    return false;
  }
}

/** Frontmatter sanity check: name + description are required by the standard. */
export function skillFrontmatterOk(skillDir) {
  try {
    const content = readFileSync(join(skillDir, 'SKILL.md'), 'utf8');
    const m = /^---\r?\n([\s\S]*?)\r?\n---/.exec(content);
    if (!m) return false;
    return /^name: .+$/m.test(m[1]) && /^description: .+$/m.test(m[1]);
  } catch {
    return false;
  }
}

/**
 * @param {object} opts
 * @param {boolean} [opts.force] replace existing entries
 * @param {boolean} [opts.copy] copy instead of symlink (release snapshot)
 * @param {string|null} [opts.project] install into <project>/.agents/skills
 * @param {string} [opts.homeDir] user-level base (defaults to os.homedir())
 * @param {string} [opts.root] repo root (defaults to this package's root)
 */
export function installSkills({ force = false, copy = false, project = null, homeDir = homedir(), root = repoRoot() } = {}) {
  const skillsDir = join(root, 'skills');
  const targetBase = project ? join(project, '.agents', 'skills') : join(homeDir, '.agents', 'skills');
  mkdirSync(targetBase, { recursive: true });

  const results = [];
  for (const name of listRepoSkills(root)) {
    const src = join(skillsDir, name);
    const dest = join(targetBase, name);
    if (!skillFrontmatterOk(src)) {
      results.push({ name, dest, action: 'error', reason: 'SKILL.md frontmatter missing name/description' });
      continue;
    }
    if (entryExists(dest)) {
      if (!force) {
        results.push({ name, dest, action: 'skipped', reason: 'already installed' });
        continue;
      }
      rmSync(dest, { recursive: true, force: true });
    }
    if (copy) {
      cpSync(src, dest, { recursive: true });
      results.push({ name, dest, action: 'copied' });
    } else {
      symlinkSync(src, dest, 'dir');
      results.push({ name, dest, action: 'linked' });
    }
  }
  return { targetBase, scope: project ? `project (${project})` : 'user', results };
}
