// install-skills: expose this repo's skills/ to kimi code and pi through the
// shared Agent Skills install point (~/.agents/skills/<name>, or a project's
// .agents/skills/). Default is a SYMLINK (single source of truth, edits live);
// --copy makes a snapshot for distribution. Idempotent: an existing entry is
// reported and skipped unless --force replaces it.

import { readdirSync, existsSync, lstatSync, mkdirSync, symlinkSync, cpSync, unlinkSync, readFileSync, readlinkSync, writeFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { homedir } from 'node:os';


/** Anchor written next to the installed skills so SKILL.md instructions can
 * locate the flywheel repo root via ${KIMI_SKILL_DIR}/../<this file>. */
export const ANCHOR_FILE = 'flywheel-home.txt';
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
  // Anchor file: skills locate the flywheel repo root through
  // "${KIMI_SKILL_DIR}/../flywheel-home.txt" regardless of symlink
  // resolution — walking up two levels is unreliable under symlinks
  // (the install point is the stable reference, not the link target).
  writeFileSync(join(targetBase, ANCHOR_FILE), `${root}\n`, 'utf8');

  const results = [];
  for (const name of listRepoSkills(root)) {
    const src = join(skillsDir, name);
    const dest = join(targetBase, name);
    if (!skillFrontmatterOk(src)) {
      results.push({ name, dest, action: 'error', reason: 'SKILL.md frontmatter missing name/description' });
      continue;
    }
    if (entryExists(dest)) {
      const lst = lstatSync(dest);
      if (lst.isSymbolicLink()) {
        const target = readlinkSync(dest);
        if (!force) {
          // Report stale links honestly instead of "already installed" (P1-11).
          if (target !== src || !existsSync(target)) {
            results.push({ name, dest, action: 'stale', reason: `symlink -> ${target} (expected ${src}; re-run with --force)` });
            continue;
          }
          results.push({ name, dest, action: 'skipped', reason: 'already installed' });
          continue;
        }
        unlinkSync(dest); // symlink: remove the link itself, never the target
      } else if (lst.isFile()) {
        if (!force) {
          results.push({ name, dest, action: 'skipped', reason: 'already installed (file)' });
          continue;
        }
        unlinkSync(dest); // plain file: safe to remove
      } else {
        // A real directory is NEVER auto-deleted, even with --force (P1-11).
        results.push({ name, dest, action: 'error', reason: 'a real directory already exists here — refusing to remove it; handle it manually' });
        continue;
      }
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
