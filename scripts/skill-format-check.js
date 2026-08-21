#!/usr/bin/env node
// Skill format gate (repo-local, zero deps): every skills/<name>/SKILL.md must
// carry YAML frontmatter with `name` (matching the directory), a non-empty
// `description`, and a `version`. name+description are the Agent Skills
// standard contract; version is this repo's own discipline (DESIGN.md 铁律 6).
// Modeled on larksuite/cli's scripts/skill-format-check (CI-gated).
//
// Usage: node scripts/skill-format-check.js [skills-dir]   (default: <repo>/skills)

import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));

/** @returns {string[]} one human-readable error per violation (empty = pass) */
export function checkSkills(skillsDir) {
  const errors = [];
  const names = existsSync(skillsDir)
    ? readdirSync(skillsDir, { withFileTypes: true }).filter((d) => d.isDirectory()).map((d) => d.name).sort()
    : [];
  if (names.length === 0) {
    errors.push(`no skill directories found in ${skillsDir}`);
    return errors;
  }
  for (const name of names) {
    const file = join(skillsDir, name, 'SKILL.md');
    if (!existsSync(file)) {
      errors.push(`${name}: missing SKILL.md`);
      continue;
    }
    const content = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
    if (!content.startsWith('---\n')) {
      errors.push(`${name}: SKILL.md must start with YAML frontmatter (---)`);
      continue;
    }
    const m = /^---\n([\s\S]*?)\n---(?:\n|$)/.exec(content);
    if (!m) {
      errors.push(`${name}: unclosed or invalid YAML frontmatter`);
      continue;
    }
    const fm = m[1];
    for (const field of ['name', 'description', 'version']) {
      if (!new RegExp(`^${field}:\\s*\\S`, 'm').test(fm)) {
        errors.push(`${name}: frontmatter missing non-empty '${field}'`);
      }
    }
    const nameMatch = /^name:\s*(\S+)/m.exec(fm);
    if (nameMatch && nameMatch[1] !== name) {
      errors.push(`${name}: frontmatter name '${nameMatch[1]}' does not match directory '${name}'`);
    }
  }
  return errors;
}

// CLI entry — only when executed directly, never on import.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const skillsDir = process.argv[2] ?? join(HERE, '..', 'skills');
  const errors = checkSkills(skillsDir);
  for (const e of errors) console.error(`❌ ${e}`);
  if (errors.length > 0) {
    console.error(`\n❌ skill format check failed (${errors.length} problem${errors.length > 1 ? 's' : ''})`);
    process.exit(1);
  }
  console.log(`✅ skill format check passed (${skillsDir})`);
}
