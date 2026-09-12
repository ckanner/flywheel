# AGENTS.md

## Purpose

Flywheel（飞轮）is a loop-engineering system: a deterministic Go
orchestrator drives Plan / Execute / Review agent loops from frozen contracts
on disk, unattended, escalating to a human when stuck. This repo holds the
orchestrator (`cmd/` + `internal/`), the contract templates (`templates/`),
and the button skills (`skills/`).

docs/design.md is the authoritative design document. Session bootstrap: read it first — its changelog header records the current phase, its §10 records what is pending (evidence gates / next steps). Its §2 十条设计铁律 are the
highest constraint: any implementation decision that conflicts with them is
wrong. The orchestrator is NOT an agent — keep every machine decision
mechanically checkable.

Note: this repo's own `docs/` holds project documentation (design.md /
review ledgers); the `docs/<slug>/` ledger zone defined in
DESIGN is a TARGET-repo concept (internal/ledger) — same prefix,
disjoint worlds.

## Build & Test

```bash
go build ./...            # 需要 Go >= 1.27（唯一构建依赖）
make test                 # go test ./... 全部包
make coverage             # 覆盖率 100% 门禁（cmd/flywheel 单行桩除外）
make quality-gate         # vet + fmt-check + coverage —— CI 同款，唯一验收闸门
```

`make quality-gate` 绿是交付的唯一标准。真实 zcode/codex/kimi 冒烟不在默认门禁内；
mock 驱动覆盖全部编排逻辑，无凭证环境可完整运行。

## Hard Contracts

- **state.json**: written ONLY by the orchestrator, every write is
  compare-and-set on `revision`, atomic (tmp+rename). Agents and skills never
  write it. `internal/state` owns all reads/writes.
- **Contract field set**: `templates/*.schema.json` is the single source of
  truth; validation happens at LOAD time via jsonschema (internal/contract) —
  a malformed acceptance must fail fast, never crash mid-run after a spawn.
- **Error taxonomy**: `internal/errs` codes (AUTH/QUOTA/RATE_LIMIT/SERVER/
  TRANSPORT/TIMEOUT/EMPTY_RESPONSE/INVALID_REQUEST/CONTEXT_WINDOW_EXCEEDED)
  are a wire contract. Changes are design changes: update docs/design.md §6 and the
  变更记录 first, then code, then tests.
- **Ledger zone**: paths inside `docs/<slug>/` are classified by
  `internal/ledger`. Boundary judgement excludes ledger paths; unit commits
  additionally exclude the volatile set (metrics.jsonl / orchestrator.log /
  orchestrator.pid / .state.lock).
- **Skills are buttons**: a SKILL.md contains validation + CLI invocation +
  translation, never loop logic. Frontmatter requires `name` matching the
  directory and non-empty `description` (the only two fields both the
  agentskills.io spec and every harness mandate), plus `metadata.version`,
  `whenToUse` and `arguments`. The latter three are NOT agentskills.io
  standard: `whenToUse`/`arguments` are kimi-code-defined fields that drive
  its auto-invocation (moonshotai.github.io/kimi-code → customization/skills);
  other harnesses (pi / Warp / Claude Code) ignore them harmlessly.
  Enforced by internal/skillsembed tests (real YAML parsing + standard
  constraints, CI-blocking), NOT by regex.
- **Output contract**: `run`/`intake`/`garden` print a JSON summary to stdout;
  `status`/`stop`/`install-skills`/`template` print human-readable text; errors go to
  stderr. Exit codes: `0` ok, `1` usage/runtime error, `3` escalated /
  budget-stopped / failed manual step.
- **SIGTERM kill switch**: machine.RunTask registers SIGTERM → kills in-flight
  child process groups → awaits quiescence → exits 143. Never remove this.

## Coverage Discipline

100% statement coverage per package is a hard gate (`make coverage`). It is a
design tool: unreachable defensive branches get deleted (with a comment citing
why), everything else gets an injection seam (sleep/now/env/log/clock) rather
than a mocked-out test. Do not loosen the gate; do not write tests that assert
implementation trivia to chase the number.

## Git Discipline

- No commits or pushes without explicit user approval.
- Commit messages: English Conventional Commits (`fix:`, `feat:`, `docs:`,
  `chore:`).
- Target repos: never commit secrets, keys, or live credentials into a task
  directory; `approvals/` records env var NAMES only.

## Language

User-facing docs (docs/design.md, README, SKILL.md bodies) are Chinese.
Code comments mix per existing convention: English for mechanics, Chinese for
design-document references.

## Maintaining This File

Add a rule only when it is repo-specific, non-obvious, actionable, and prevents
a recurring failure. Mechanizable constraints belong in code/tests/CI — this
file keeps only rationale and safe exceptions.
