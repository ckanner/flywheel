# AGENTS.md

## Purpose

Flywheel（飞轮）is a loop-engineering system: a deterministic Go
orchestrator drives Plan / Execute / Review agent loops from frozen contracts
on disk, unattended, escalating to a human when stuck. This repo holds the
orchestrator (`cmd/` + `internal/`), the contract templates (`templates/`),
and the button skills (`skills/`).

docs/design.md is the authoritative design document. Session bootstrap: read it first — its introduction records the implementation phase, and its validation and future-work sections record pending checks and follow-ups. Its §2 design invariants are the
highest constraint: any implementation decision that conflicts with them is
wrong. The orchestrator is NOT an agent — keep every machine decision
mechanically checkable.

Note: this repo's own `docs/` holds project documentation (design.md /
review ledgers); the `docs/<slug>/` ledger zone defined in
DESIGN is a TARGET-repo concept (internal/ledger) — same prefix,
disjoint worlds.

## Build & Test

```bash
go build ./...            # Go 引擎需要 Go >= 1.27；npm／DSH 部分另需 Node >= 22.18.0
make test                 # go test ./... 全部包
make coverage             # 覆盖率 100% 门禁（cmd/flywheel 单行桩除外）
make quality-gate         # vet + fmt-check + coverage —— Go 完整门禁
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check  # DSH 插件构建与离线行为测试
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro|ClaudeCodePro)NodeWorkerOfflineBridge$' -count=1
npm test                 # npm 启动器、安装器、发行门禁的单元与协议测试
npm run build:npm        # 隔离构建六个同版本包，只生成候选，不发布
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

Go 改动必须通过 `make quality-gate`；DSH 插件还须通过 Node 与跨进程桥接检查。
npm 发行修改须运行根目录 `npm test`，并构建一次六包候选，通过真实 npm
安装测试。CI 在 Linux/macOS 验证同一个候选，并在 Linux 检查核心包 race。
安装测试使用隔离 registry／用户目录和 DSH 协议桩；不改真实 profile。
真实模型、桌面 UI 和账号冒烟不在默认门禁内；无凭证环境可以运行全部离线检查。
Claude Code Pro 是 optional peer；验证该后端时先在 `plugins/dsh` 安装精确 0.3.2
公开接口（未发布时使用已审查源码的真实 npm tarball）。默认未安装的跳过不算该后端
通过；记录实测 tarball 身份，不能改版本冒充兼容或虚构 registry 发行。

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
  orchestrator.pid / .state.lock / .run.lock / .stop-request / .control/).
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
  `control launch|start|status|stop|config|doctor` prints one protocol-versioned JSON response;
  `config inspect|migrate` and `doctor` print human text, or a single JSON document with `--json`;
  `install-skills` prints human text or one complete protocol-versioned summary with `--json`;
  npm `install`/`setup --json` returns a distinct installation report, retaining partial failures;
  `status`/`stop`/`template` print human-readable text; errors go to
  stderr. Exit codes: `0` ok, `1` usage/runtime error, `3` escalated /
  budget-stopped / failed manual step.
- **SIGTERM kill switch**: machine.RunTask registers SIGTERM → kills in-flight
  child process groups → awaits quiescence → exits 143. Never remove this.
  The npm launcher must forward cancellation and await Go cleanup; a wrapper
  exiting alone does not prove its native child has stopped.

## npm Installation & Release

- The primary user entry is `npx -y @ckanner/flywheel@latest install`.
  Plain npm installation only places package files; explicit `install`/`setup`
  owns user setup. Never hide profile, skills or credential changes in postinstall.
  Existing DSH and Node are prerequisites; harness login remains the user's own flow.
- Keep the meta package, four optional platform packages and DSH plugin at one
  exact version. The meta pins the plugin dependency as well as platform
  optionalDependencies. Resolve package roots through Node, including hoisted
  installs; do not silently fall back to mismatched binaries or workers.
- User `flywheelBinary` takes precedence over the managed GUI installation
  record. A missing record may use legacy PATH lookup; an invalid record or
  changed launcher must fail visibly. Preserve custom config and modified
  skills: ownership requires a matching content manifest, not merely a marker.
- Build worker and native binaries in an isolated source snapshot, pack once,
  then test and publish those exact tarballs. Publication requires a clean
  checkout and the release tag pointing at exact HEAD. Existing registry
  versions must match integrity; only a structured E404 means absent.
- Publish dependencies/plugin before meta under a candidate tag. Verify all six
  exact versions and registry installation before promoting latest; never
  downgrade a newer latest. A failed retry reuses the checked artifact instead
  of rebuilding different bytes for an already published version.

## Coverage Discipline

100% statement coverage for every measured Go function is a hard gate
(`make coverage`; the one-line cmd entry stub is excluded). It is a
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

Keep the Chinese README, design documents and SKILL.md bodies. Maintain the
English README (`README.en.md`), essential user guides (`docs/en/`) and DSH
plugin README alongside their Chinese counterparts. Changes to installation,
configuration, workflow or runtime boundaries must update both languages;
historical design and validation records do not need duplicate translations.
Code comments mix per existing convention: English for mechanics, Chinese for
design-document references.

## Maintaining This File

Add a rule only when it is repo-specific, non-obvious, actionable, and prevents
a recurring failure. Mechanizable constraints belong in code/tests/CI — this
file keeps only rationale and safe exceptions.
