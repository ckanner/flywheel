# Contributing to Flywheel

English · [简体中文](../zh-CN/contributing.md) · [Documentation](../README.md)

Start with the [project overview](../../README.md), the authoritative
[system design](../design.md) (Chinese), and [AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md).
Flywheel is a deterministic Go orchestrator: models plan, implement, and review
bounded tasks; code owns state transitions, verification, budgets, and process
lifecycles. Contributions must preserve that boundary.

## License and scope

Flywheel is distributed under the [MIT License](../../LICENSE). Keep its copyright
and license notice with copies or substantial portions of the software. Preserve
third-party notices and the provenance of bundled dependencies; the repository's
license does not replace their terms.

Keep credentials, private project contents, and unredacted model/session logs out
of issues, PRs, and fixtures. For a sensitive security report, request a private
reporting route before disclosing details; the repository does not publish a
separate response-time commitment.

## Choose a branch and execution directory

Use or reuse a worktree dedicated to the change. For a new change, when `origin`
and `main` are the intended remote and base:

```sh
git status --short
git fetch origin
git worktree add -b task/my-change ../flywheel-my-change origin/main
cd ../flywheel-my-change
```

Inspect an existing branch or directory before reusing it. Preserve other work
and keep the change's contracts, configuration, checks, and commits in the same
integration checkout. The original repository remains the session's project
workspace; the integration checkout and packet worktrees are execution locations.
When Flywheel develops the change, follow the effective `worktree.enabled` policy.
The engine merges accepted packets into the integration checkout's current
branch, not automatically into `main`.

Worktrees isolate checkout files and the index; they are not security sandboxes.
Serial execution can retain the documented `in-place` fallback. Parallel execution
with automatic worktree management requires successfully created isolated
worktrees. With automatic management disabled, shared execution uses batch-wide
verification and delivery. See the [workflow](workflow.md) and
[console](flow-console.md) guides for these distinctions and external directory
registration.

Prepare requirements, scope, and executable acceptance criteria before a large
implementation. Open a Draft PR once there is a useful reviewable plan or change;
one PR may cover several packets. Keep commits focused and use English
Conventional Commit subjects, such as `fix: preserve cleanup evidence`.
Agents follow the user's existing commit, push, and merge authorization without
asking again for an already authorized step or extending it to a different action.

## Implement against observable contracts

The [design invariants](../design.md#2-设计不变量) and AGENTS file define the core rules:

- Go and `internal/state` own task state. Writes use revision compare-and-set and
  atomic replacement; models and skills never edit `state.json`.
- Contract schemas define valid input. Validate at load time and update the design
  before changing a wire contract, including the error taxonomy.
- Test completion, cancellation, timeouts, and cleanup using observable results.
  A terminal text marker or worker exit does not prove its child groups stopped.
- Verification and independent review apply to the candidate that is committed
  and merged. Preserve work and evidence when a gate or Git delivery fails.
- Missing usage, cost, permission, or capability evidence stays unknown; it cannot
  become success or zero cost.
- Skills validate inputs and invoke the CLI. The desktop plugin consumes Go
  control interfaces; neither owns a second state machine or retry loop.

Use meaningful failure-path tests and narrow I/O injection points. Every measured
Go function must reach 100% statement coverage; only the trivial `cmd/flywheel`
entry stub is excluded. Preserve that gate and test behavior rather than repeating
the implementation solely to increase the percentage.

## Build and verification

Source development needs **Go 1.27 or later** and Git. Flywheel's npm runtime
requires **Node 22.18.0 or later**; the selected DSH release may require a newer
Node version. CI and releases pin **Node 24.22.1 / npm 12.2.1**. Use those versions
when reproducing package and installation evidence.

Run the applicable checks from the repository root:

```sh
make quality-gate
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro|ClaudeCodePro)NodeWorkerOfflineBridge$' -count=1
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

| Change | Required verification |
|---|---|
| Go engine | `make quality-gate`: vet, formatting, tests, and coverage |
| DSH plugin or worker | Plugin checks and the applicable Go-to-Node bridges |
| npm installation or release | Root npm tests, a six-package candidate build, and installation of those tarballs |
| Documentation | Links, anchors, fences, runnable examples, translation consistency, and affected embedding/package contents |

The Claude bridge needs the exact optional 0.4.0 machine API in the worker's
dependency tree. An absent-peer skip does not verify that backend. Preserve the
reviewed source/tarball identity when using a source candidate. Codex Pro's
bundled 0.9.0 source package is likewise pinned by its actual integrity and
[vendor record](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md).

For lifecycle, state, or process changes, reproduce the Linux CI race check:

```sh
go test -race ./internal/machine ./internal/verify ./internal/watchdog ./internal/drivers ./internal/cli ./internal/modules ./internal/diagnostic -count=1
```

For changes to session quality or the evaluation harness, use the fixed offline
paired scenarios described in [Session evaluation](session-evaluation.md):

```sh
make build
FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions
```

Default tests exercise real Git, processes, workers, and public adapter packages
with offline fixtures. Installation tests consume actual candidate tarballs in an
isolated registry and user directory; desktop registration uses a CLI fixture.
Dependencies require registry access or the documented verified fixture. These
checks do not establish live account access, model quality, subscription quota,
or a real Mac desktop pass.

CI uses the same candidate on Ubuntu and macOS, with race checks on Linux. Report
what actually ran: unavailable CI remains unavailable, and cross-compiling a Mac
binary does not establish its runtime behavior. Stop additional testing once the
applicable gates and concrete remaining risks are resolved.

## Keep documentation organized

Documentation changes belong in the same PR as the behavior they describe. Identify
which guides are affected before implementation, then update those guides in both
languages. A narrow change does not require rewriting every guide or running every
repository test.

### Use the right source for each claim

| Claim | Authoritative source | How to use it |
|---|---|---|
| Architecture, responsibilities, and invariants | [System design](../design.md), especially its design invariants | Treat these as intended constraints, not optional descriptions. |
| Legal fields, types, required values, and wire formats | [Schemas and templates](https://github.com/ckanner/flywheel/tree/main/templates), the relevant parser, and [worker protocol](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md) | Check examples against the applicable schema and implemented interface. A schema accepting a field does not prove a backend supports it. |
| Defaults, precedence, migration, and model bindings | [Templates embedded from the current source revision](https://github.com/ckanner/flywheel/tree/main/templates) and [configuration parsing and normalization](https://github.com/ckanner/flywheel/tree/main/internal/config) | For an installed release, also verify that version's actual templates and `config inspect` output. Distinguish defaults, omitted-field compatibility, and the user's effective configuration; an older installation cannot establish new source defaults. |
| CLI, worker, and desktop behavior | Current [CLI](https://github.com/ckanner/flywheel/tree/main/internal/cli), [drivers](https://github.com/ckanner/flywheel/tree/main/internal/drivers), and [DSH plugin](https://github.com/ckanner/flywheel/tree/main/plugins/dsh) implementation and relevant tests | Verify commands, lifecycle boundaries, permission support, and UI labels against the implemented path. |
| Packaging, release order, and automated gates | [Package manifest](https://github.com/ckanner/flywheel/blob/main/package.json), lockfiles, [scripts](https://github.com/ckanner/flywheel/tree/main/scripts), and [workflows](https://github.com/ckanner/flywheel/tree/main/.github/workflows) | Document the commands and gates that exist. A policy in prose is not evidence that CI enforces it. |
| Completed tests, releases, or evaluations | [Dated validation records](../validation/README.md) and [evaluation evidence](../evaluation/README.md) | Limit claims to the recorded source, platform, inputs, and results; past results do not establish current registry or live-account status. |

When design and implementation disagree, record the mismatch and resolve the
intended behavior from the task's accepted requirements and design constraints.
Correct the implementation when it violates that contract; update the design when
an intended design change is part of the task, then align code, tests, and guides in
the same PR. Do not silently declare the code correct by weakening an invariant,
or describe a planned capability as implemented. If the intended behavior remains
unresolved, state that limit instead of inventing a resolution.

### Put content in its canonical location

The default root and plugin `README.md` are English, with `README.zh-CN.md`
counterparts. `README.en.md` remains a compatibility link, not another manual.
Current user and maintainer guides use matching filenames in `docs/en/` and
`docs/zh-CN/`, with reciprocal language links and the appropriate documentation index.

| Content | Canonical location |
|---|---|
| Product overview and first commands | Root READMEs |
| Current installation, workflow, configuration, and maintenance | `docs/en/` and `docs/zh-CN/` |
| Current architecture and invariants | `docs/design.md` |
| Design proposals and their original context | `docs/plans/` |
| Executed checks, environments, evidence, and limits | `docs/validation/` and `docs/evaluation/` |
| Historical reviews and superseded material | `docs/reviews/` and `docs/archive/` |
| Package-specific interfaces and provenance | `plugins/dsh/` |
| Rules for contributors and coding agents | The paired contribution guides and [AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md) |

This repository's documentation is separate from the `docs/<task>/` contracts and
runtime ledger created in a target repository.

### Map a change to its documentation

Use the following as an impact checklist. Update the named material only where the
change affects its claims; design/schema/protocol changes also follow their own
contract rules.

| Change | Documentation to inspect and update when affected |
|---|---|
| Installation, upgrade, prerequisites, or setup | Root and plugin READMEs, `getting-started.md`, installation examples, and package contents |
| CLI commands, control responses, workers, or backend support | `getting-started.md`, `workflow.md`, relevant backend guide, plugin README/`PROTOCOL.md`, and affected skill instructions |
| Configuration, defaults, accounts, models, or permissions | `configuration.md`, `model-policy.md`, relevant README tables, actual template explanations, and migration instructions |
| Scheduling, worktrees, execution locations, or delivery | `workflow.md`, `configuration.md`, `flow-console.md`, design sections, and affected skills |
| Session reuse, recovery, handoff, or accounting | `session-recovery.md`, configuration/workflow/console sections, design, and affected worker contracts |
| Desktop UI, task discovery, or operator controls | `flow-console.md`, plugin READMEs, and getting-started instructions that name the UI |
| Evaluation tools, scenarios, or result interpretation | `session-evaluation.md` or `jev.md`, their input/output contracts, and dated evidence when new checks actually run |
| Package layout, licenses, or release behavior | `releasing.md`, relevant installation instructions, manifests/lockfiles, packaged links, and applicable license/provenance notices |
| A documentation move, rename, or heading change | Both documentation indexes, language links, all incoming paths/anchors, package lists, embedding references, scripts, and tests |
| Contributor or AI operating rules | [AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md), both contribution guides, and their navigation; keep the requirements consistent |

The affected English and Chinese guides must ship in the same PR. Translate the
meaning, not just the headings: commands, model IDs, JSON fields, examples, defaults,
permissions, failure behavior, and capability limits must agree. A translation must
not promise a broader capability or hide a limitation present in the other language.

### Maintain readable structure and reliable history

Update the section that owns a behavior; add a named subsection only when needed.
Keep one page title, a logical heading hierarchy, short introductions, typed code
fences, readable tables, and blank lines around headings and lists. Label fragments
and placeholders, and use consistent terminology for project workspace, execution
directory, task, packet, and session. Write concrete instructions rather than a
conversation transcript. Do not prepend development updates to introductions or
append unrelated status paragraphs after the license. Release changes belong in
`CHANGELOG.md`; dated execution evidence belongs in validation records.

Historical plans, reviews, archives, and validation records retain their original
language and dated context. Preserve recorded dates, commits, trees, versions,
hashes, inputs, platforms, and pass/fail/skip/not-run results. Correct a historical
error with an explicitly labelled correction that identifies what changed and why;
do not silently rewrite an old outcome or replace its candidate identity with a
new one. Later completion belongs in a new dated record, linked from the old record
when useful. Historical material does not need duplicate translations, but indexes
must make its language and status clear.

### Validate the affected documentation

Before requesting review, check the items relevant to this change:

- [ ] Update the owning section and affected English/Chinese counterparts in the
  same PR, with equivalent meaning, matching filenames, and reciprocal language links.
- [ ] Verify behavior and default claims against the relevant design, CLI, schema,
  and source templates; support completed-test claims with matching dated evidence.
  State unresolved differences and validation limits.
- [ ] For moves or heading changes, use `rg` to check incoming paths and anchors in
  guides, READMEs, skills, template embedding code, manifests, scripts, and tests.
  Resolve links from the referring file; preserve historical immutable commit links.
- [ ] Check local links, anchors, headings, fences, JSON/YAML examples, and safe shell
  syntax as applicable; run `git diff --check`. Use a repository documentation checker
  only if one exists; identify manual or ad hoc checks accurately.
- [ ] For package or embedding changes, inspect the relevant build/pack output and
  links in the actual candidate. Include relative-link targets or use repository URLs
  for source-only material; an old tarball cannot verify newly edited documentation.
- [ ] Record actual passed/failed/skipped/not-run results, methods, environments, and
  applicable candidate identity in the PR. Do not imply unconfigured CI checks or
  validation beyond the evidence; apply only gates relevant to the change.

Use source/help inspection, syntax checks, or isolated fixtures for examples. Do not
run live models, change accounts, install into real user profiles, publish, or deploy
merely to check documentation; those actions require an already authorized task scope.

## AI-assisted contribution workflow

1. Establish the current task scope and existing authorization. Read applicable
   repository instructions, inspect the branch, HEAD, and dirty work, and preserve
   unrelated edits. Reuse the intended PR/worktree when continuing a task.
2. Inspect the relevant authoritative sources and identify documentation impact
   before implementation. Separate current behavior, proposed changes, and historical
   evidence; surface unresolved design/code differences.
3. Implement the scoped change and its affected bilingual documentation in the same
   PR. Preserve the engine's state ownership, frozen contracts, and user customizations.
4. Run the applicable checks and resolve concrete failures. Do not expand a small
   documentation change into unrelated runtime work or live-account experiments.
5. Review the final diff, relevant branch/base changes, translations, references, and
   verification evidence. Recheck any conclusions invalidated by later edits.
6. Complete the authorized commit/push/PR work and report the result. Reuse prior
   authorization without asking again for the same action; a request for a PR alone
   does not authorize merging, publishing, deploying, or changing repository visibility.

## Prepare a reviewable PR

Explain the concrete problem, the change and resulting behavior, the affected
bilingual guides, and any intentionally unchanged documentation whose relevance is
not obvious. Review the final diff and evidence independently of an implementing
model's completion claim. Follow active merge requirements and the user's
authorization; preserve required gates and repository settings.

Record each applicable check as **passed**, **failed**, **skipped**, or **not run**,
including the command/method, relevant environment, and reason for skipped or
unavailable checks. Distinguish local evidence from remote CI and live validation.
For package or release work, record the tested candidate's source commit/tree,
snapshot hash, tarball integrity, and dirty/clean status as applicable. Do not attach
results from a different source or candidate to the PR's final state without stating
that limitation. Source merge and npm publication remain separate operations.

## Release changes

Follow the [release guide](releasing.md). All six Flywheel packages share one exact
version with exact internal dependencies and the pinned Codex Pro runtime. Build
once from final clean source and test those tarballs. Preserve the source commit,
tree, snapshot hash, and package integrity values; the release tag must identify
that source. Do not relabel a dirty candidate as a clean release or rebuild
different bytes for a published version.

Use the repository's release scripts for candidate checks, publication, registry
verification, and promotion. Publish platform packages and the plugin before the
meta package, test the exact registry version before moving `latest`, and verify
all six tags and integrities. A partial publication resumes with the same
candidate. Manual publication requires its own explicit authorization and an
accurate record of completed and unavailable checks.
