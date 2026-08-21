# AGENTS.md

## Purpose

Flywheel（飞轮）is a loop-engineering system: a deterministic Node.js
orchestrator drives Plan / Execute / Review agent loops from frozen contracts
on disk, unattended, escalating to a human when stuck. This repo holds the
orchestrator (`bin/` + `orchestrator/`), the contract templates
(`templates/`), the button skills (`skills/`), and the test suite.

DESIGN.md is the authoritative design document. Its §2 十条设计铁律 are the
highest constraint: any implementation decision that conflicts with them is
wrong. The orchestrator is NOT an agent — keep every machine decision
mechanically checkable.

## Build & Test

```bash
npm ci                         # only runtime dependency: yaml
npm test                       # full gate: unit + mock e2e + real pi/kimi smoke
                               # (real smoke auto-skips without creds; see below)
npm run skill-format-check     # frontmatter gate for skills/
```

`npm test` is the ONLY acceptance gate. Do not yield work without it green.

Real smoke tests (`pi-smoke`, `kimi-smoke`) need the harness CLIs and
`OPENCODE_API_KEY` / `KIMI_API_KEY`; they skip cleanly when absent. They also
depend on environment facts recorded in HANDOFF.md (e.g. `~/.pi/agent/models.json`
overrides) — if a real smoke fails, read HANDOFF.md before touching product code.

## Hard Contracts

- **state.json**: written ONLY by the orchestrator, every write compare-and-set
  on `revision`, atomic (tmp+rename). Agents and skills never write it.
- **Error taxonomy**: `ERROR_CODES` / `RETRYABLE_CODES` / `FALLBACK_CODES` in
  `orchestrator/core/errors.js` are a wire contract. Changes are design changes:
  update DESIGN.md §6 and the 变更记录 first, then code, then tests.
- **Contract field set**: `PACKET_ALLOWED_FIELDS` in `orchestrator/core/contracts.js`
  must stay in lockstep with `templates/task-packet.schema.json`
  (`additionalProperties:false`). Validation happens at LOAD time — a malformed
  acceptance must fail fast, never crash mid-run after a spawn.
- **Ledger zone**: paths inside `docs/<slug>/` are classified by
  `orchestrator/core/ledger.js` (single source of truth). Executor-diff
  judgement excludes ledger paths; unit commits additionally exclude the
  volatile trio (metrics.jsonl / orchestrator.log / orchestrator.pid).
- **Skills are buttons**: a SKILL.md contains validation + CLI invocation +
  translation, never loop logic. Frontmatter requires `name` (matching the
  directory), non-empty `description`, `version` — enforced by
  `npm run skill-format-check` in CI.
- **Output contract**: `run`/`intake`/`garden` print a JSON summary to stdout;
  `status`/`stop` print human-readable text. Errors go to stderr. Exit codes:
  `0` ok, `1` usage/runtime error, `3` escalated / budget-stopped / failed
  manual step.

## Git Discipline

- No commits or pushes without explicit user approval.
- Commit messages: English Conventional Commits (`fix:`, `feat:`, `docs:`,
  `chore:`).
- Target repos: never commit secrets, keys, or live credentials into a task
  directory; `approvals/` records env var NAMES only.

## Language

User-facing docs (DESIGN.md, HANDOFF.md, README, SKILL.md bodies) are Chinese.
Code comments mix per existing convention: English for mechanics, Chinese for
design-document references.

## Maintaining This File

Add a rule only when it is repo-specific, non-obvious, actionable, and prevents
a recurring failure. Mechanizable constraints belong in code/tests/CI — this
file keeps only rationale and safe exceptions.
