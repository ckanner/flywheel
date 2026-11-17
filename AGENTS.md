# AGENTS.md

## Purpose

Flywheel（飞轮）is a loop-engineering system: a deterministic Go
orchestrator drives Plan / Execute / Review agent loops from frozen contracts
on disk, unattended, escalating to a human when stuck. This repo holds the
orchestrator (`cmd/` + `internal/`), the contract templates (`templates/`),
and the button skills (`skills/`).

[docs/design.md](docs/design.md) defines the architecture and required design
contracts. Read §2 and the sections affected by the task before changing behavior;
§10 links implementation and validation evidence. Schemas define valid contract
fields; current source and tests establish implemented behavior. If these disagree,
resolve the discrepancy explicitly rather than documenting a bug as the intended
contract. The orchestrator is NOT an agent — keep every machine decision
mechanically checkable and preserve the design invariants.

Note: this repo's own `docs/` holds project documentation (design.md /
review ledgers); the `docs/<slug>/` ledger zone defined in
the design is a TARGET-repo concept (internal/ledger) — same prefix,
disjoint worlds.

## Change Workflow

1. **Establish scope.** Inspect the branch, HEAD, working-tree changes, and existing
   PR before editing. Reuse the task checkout when appropriate and preserve other
   work. Read the [documentation index](docs/README.md), applicable instructions,
   design sections, schemas, source, and tests. Identify the affected contracts,
   user guides, and validation before implementation; historical proposals and
   acceptance records do not establish current defaults.
2. **Keep the change coherent.** Implement the requested behavior and update its
   owning documentation in the same PR. For contract changes, revise the relevant
   design/schema/protocol alongside code and tests. Keep unrelated model defaults,
   permissions, dependencies, and user configuration outside the task's scope.
3. **Verify the final change.** Run the applicable checks below, then review the
   final diff against the task and design contracts. Check both language editions
   and affected packaged or embedded files. Recheck evidence affected by a later
   edit; select checks for the changed behavior instead of repeating unrelated
   suites. Never weaken a gate to make a change pass.
4. **Complete the authorized delivery.** Report what changed, the checks actually
   run, and remaining failures or limits. Distinguish pass, fail, skipped, and not
   run; source validation, candidate installation, CI, registry publication, and
   user-machine acceptance are separate evidence. Finish authorized commits and
   PR updates using the reviewed change and the Git rules below.

## Build & Test

```bash
go build ./...            # Go 引擎需要 Go >= 1.27；npm／DSH 部分另需 Node >= 22.18.0
make test                 # go test ./... 全部包
make coverage             # 覆盖率 100% 门禁（cmd/flywheel 单行桩除外）
make quality-gate         # vet + fmt-check + coverage —— Go 完整门禁
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check  # DSH 插件构建与离线行为测试
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test((DSH|CodexPro|ClaudeCodePro|KimiNative)NodeWorkerOfflineBridge|(DSH|CodexPro|ClaudeCodePro)ProviderActivityOfflineBridge|KimiPrintSessionTitleOfflineBridge)$' -count=1
npm test                 # npm 启动器、安装器、发行门禁的单元与协议测试
make build
FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions # 36 条真实引擎 / public adapter 离线配对场景
npm run build:npm        # 隔离构建一个完整包（含四平台引擎），只生成候选，不发布
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

Go 改动必须通过 `make quality-gate`；DSH 插件还须通过 Node 与跨进程桥接检查。
npm 发行修改须运行根目录 `npm test`，并构建一次单包候选，通过真实 npm
安装测试。CI 在 Linux/macOS 验证同一个候选，并在 Linux 检查核心包 race。
安装测试使用隔离 registry／用户目录和 DSH 协议桩；不改真实 profile。
真实模型、桌面 UI 和账号冒烟不在默认门禁内；无凭证环境可以运行全部离线检查。
Claude Code Pro 是 optional peer；验证该后端时先在 `plugins/dsh` 安装精确 0.6.0
公开接口（未发布时使用已审查源码的真实 npm tarball）。默认未安装的跳过不算该后端
通过；记录实测 tarball 身份，不能改版本冒充兼容或虚构 registry 发行。
Codex Pro 0.10.2 以 vendor 记录中的已审查源码包及实际 integrity 锁定，
通过 bundledDependencies 随插件分发；不可替换成未验证 registry integrity。
P4 fixture 验证真实引擎与实验工具，不代表真实模型质量、缓存或性能结论。

## Hard Contracts

- **state.json**: written ONLY by the orchestrator, every write is
  compare-and-set on `revision`, atomic (tmp+rename). Agents and skills never
  write it. `internal/state` owns all reads/writes.
- **Contract field set**: `templates/*.schema.json` is the single source of
  truth; validation happens at LOAD time via jsonschema (internal/contract) —
  a malformed acceptance must fail fast, never crash mid-run after a spawn.
- **Error taxonomy**: `internal/errs` codes (AUTH/QUOTA/RATE_LIMIT/SERVER/
  TRANSPORT/TIMEOUT/EMPTY_RESPONSE/INVALID_REQUEST/CONTEXT_WINDOW_EXCEEDED)
  are a wire contract. Changes are design changes: define the change in
  docs/design.md §6 before implementing it, then update code, tests, and
  CHANGELOG.md in the same PR.
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
  `control <action>` prints one protocol-1 JSON response, independently of worker
  protocol versions (see docs/design.md §12 and `internal/cli/control.go`);
  `decision replay|evaluate` prints one JSON decision report after evaluation;
  `config inspect|migrate` and `doctor` print human text, or a single JSON document with `--json`;
  `install-skills` prints human text or one complete protocol-versioned summary with `--json`;
  npm `install`/`setup --json` returns a distinct installation report, retaining partial failures;
  `status`/`stop`/`template` and interactive `session` use text output.
  Human diagnostics use stderr. Control failures, including parse errors, stay
  in the stdout JSON envelope; other commands may return an error report or
  fail before producing one. Preserve each interface's failure contract.
  Exit codes: `0` ok, `1` usage/runtime error, `3` escalated / budget-stopped /
  failed manual step; decision evaluation also uses `3` for failed or unknown requests.
  The retired hidden `shadow` entry returns a migration error without running anything.
- **SIGTERM kill switch**: machine.RunTask registers SIGTERM → kills in-flight
  child process groups → awaits quiescence → exits 143. Never remove this.
  The npm launcher must forward cancellation and await Go cleanup; a wrapper
  exiting alone does not prove its native child has stopped.

## npm Installation & Release

- The primary user entry is `npx -y @ckanner/flywheel@latest install`.
  Plain npm installation only places package files; explicit `install`/`setup`
  owns user setup. Never hide profile, skills or credential changes in postinstall.
  Existing DSH and Node are prerequisites; harness login remains the user's own flow.
- Publish only `@ckanner/flywheel`. It contains the CLI, DSH host/client and
  workers, embedded skills, and all four native binaries. Validate the bundled
  native manifest and checksums; never fetch an engine at runtime or fall back
  to an old platform/worker package. `plugins/dsh` is a private build/test harness,
  not a separately published package. Legacy registry versions remain immutable.
- User `flywheelBinary` takes precedence over the managed GUI installation
  record. A missing record uses the installed package’s bundled CLI (source-only
  fixtures may use PATH); an invalid record or changed launcher must fail visibly. Preserve custom config and modified
  skills: ownership requires a matching content manifest, not merely a marker.
- Build worker and native binaries in an isolated source snapshot, pack once,
  then test and publish those exact tarballs. Publication requires a clean
  checkout and the release tag pointing at exact HEAD. Existing registry
  versions must match integrity; only a structured E404 means absent.
- Publish the complete package under a candidate tag. Verify its exact
  version, bundled contents and registry installation before promoting latest; never
  downgrade a newer latest. A failed retry reuses the checked artifact instead
  of rebuilding different bytes for an already published version.

## Coverage Discipline

100% statement coverage for every measured Go function is a hard gate
(`make coverage`; the one-line cmd entry stub is excluded). It is a
design tool: exercise meaningful failure paths with narrow injection seams
(sleep/now/env/log/clock), and assert observable behavior. Demonstrate why a branch
is unreachable before removing it; never remove a valid defense solely to raise
coverage. Do not loosen the gate or mirror implementation trivia in tests.

## Git Discipline

- Commit and push only within the user's authorization. An explicit request to
  submit or update a PR authorizes the commits and branch push needed for that PR;
  reuse that authorization without asking again. It does not authorize merging
  into the base branch, tagging, publishing, deploying, or changing user settings.
- Preserve other work and concurrent remote updates. Inspect the current branch
  head before updating it; do not overwrite another contributor's commits or
  force-push without explicit authorization. A new base commit may need to be
  integrated and the affected checks repeated before the PR is ready.
- Commit messages: English Conventional Commits (`fix:`, `feat:`, `docs:`,
  `chore:`).
- Target repos: never commit secrets, keys, or live credentials into a task
  directory; `approvals/` records env var NAMES only.

## Documentation Maintenance

Use the change-to-document mapping and review checklist in the
[English contribution guide](docs/en/contributing.md#keep-documentation-organized)
or [中文贡献指南](docs/zh-CN/contributing.md#保持文档结构清晰). For every change:

- Update the section that owns the behavior; add a named subsection when needed.
  Keep the root README focused on the project and first steps. Release changes
  belong in `CHANGELOG.md`; dated proposals, validation, and reviews belong in
  their indexed directories. Never append a task journal to a README or this file.
- Update affected English and Chinese guides in the same PR. Match commands,
  fields, defaults, permissions, and limitations; prose need not be word-for-word.
- When adding or moving a document, update navigation, reciprocal language links,
  and incoming references, including scripts, tests, templates, and embedded
  skills. Preserve compatibility entry points when existing consumers need them.
- Check links, anchors, fenced examples, and the relevant package contents.
  Relative package links must resolve inside the installed package; use explicit
  repository links for source that is not shipped. Do not claim a dedicated docs
  CI gate unless the repository actually implements it.
- Preserve historical dates, source/candidate identities, results, and limits.
  Mark factual corrections with their basis; record later acceptance separately
  instead of replacing old outcomes. Package versions alone do not establish
  registry or installed versions.

### Language

Use English as the default root and plugin README (`README.md`), with paired
`README.zh-CN.md` translations. Keep `README.en.md` only as a compatibility
link. Maintain user and maintainer guides in parallel `docs/en/` and
`docs/zh-CN/` directories with matching filenames and reciprocal language links.
Keep the authoritative `docs/design.md` and SKILL.md bodies in
Chinese; historical plans, reviews and validation records do not need duplicate
translations. Label links to material available in only one language.
Code comments mix per existing convention: English for mechanics, Chinese for
design-document references.

## Maintaining This File

Add a rule only when it is repo-specific, non-obvious, actionable, and prevents
a recurring failure. Put detailed procedures and examples in the paired
contribution guides and link them here. Update the owning section, remove rules
superseded by the same change, and keep temporary progress or dated evidence out
of this file. Mechanizable constraints belong in code/tests/CI; do not describe
a proposed check as enforced until it exists.
