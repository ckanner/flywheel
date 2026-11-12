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

The default project entry is [README.md](../../README.md) in English, with
[README.zh-CN.md](../../README.zh-CN.md) as the Chinese counterpart. Current user
and contributor guides live in `docs/en/` and `docs/zh-CN/`, using matching
filenames and reciprocal language links.

| Content | Canonical location |
|---|---|
| Product overview and first commands | Root READMEs |
| Current installation, workflow, configuration, and maintenance | `docs/en/` and `docs/zh-CN/` |
| Authoritative architecture and invariants | `docs/design.md` |
| Design proposals and their original context | `docs/plans/` |
| Executed checks, environments, evidence, and limits | `docs/validation/` |
| Historical reviews and superseded material | `docs/reviews/` and `docs/archive/` |
| Package-specific interfaces and provenance | `plugins/dsh/` |

Update the existing section that owns a behavior. Add a named subsection when
needed, and update its navigation. Do not prepend a development update to the
introduction or append unrelated instructions after the license. Release changes
belong in the changelog; dated evidence belongs in validation records.

Update both languages when installation, configuration, workflows, commands, or
runtime boundaries change. Keep model IDs, JSON fields, permissions, examples,
and capability limits equivalent. Historical design, proposal, and validation
records do not need duplicate translations; label the language of those links.

Check packaged documentation as well as the source tree. The package manifest
must include the files linked from its README, or the link must intentionally
point to the repository. Verify the exact candidate: an older tarball does not
contain newly edited documentation merely because a source link now works.

## Prepare a reviewable PR

Explain the concrete problem, the change and resulting behavior, the checks run,
and remaining validation limits. Review the final diff and evidence independently
of an implementing model's completion claim. Follow active merge requirements
and the user's authorization; preserve required gates and repository settings.
Source merge and npm publication are separate operations.

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
