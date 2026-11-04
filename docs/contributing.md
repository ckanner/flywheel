# Contributing to Flywheel

Start with the [project overview](../README.en.md), the authoritative
[design document](design.md) (Chinese), and [AGENTS.md](../AGENTS.md).
Flywheel is a deterministic Go orchestrator. Models perform bounded planning,
implementation, and review; code owns state transitions, verification, budgets,
and lifecycle decisions. Changes must preserve that boundary.

## Scope and licensing

The repository currently declares **`UNLICENSED`**. Maintainers must decide on an
open-source license before describing the project as open source. Preparing
English documentation or publishing an npm package does not change this status.
Do not introduce a different license or change existing copyright notices as
part of an unrelated contribution.

There is no documented private security-reporting channel or response-time
commitment in this repository. For a potentially sensitive report, request a
private reporting route without publishing the details. Keep credentials,
private project contents, and unredacted model/session logs out of public issues
and PRs. Do not invent a maintainer contact address or promise an unconfigured
reporting service.

## Work in a task branch and worktree

Use or reuse a worktree dedicated to the change. For a new change, with `origin`
and `main` as the intended remote and base:

```sh
git status --short
git fetch origin
git worktree add -b task/my-change ../flywheel-my-change origin/main
cd ../flywheel-my-change
```

Inspect an existing branch or directory before reusing it; do not force-reset it
or discard another task's changes. Keep the task's contracts, configuration,
checks, and commits in the same integration worktree. When using Flywheel to
develop the change, let the engine manage its packet worktrees. Its local merge
target is the current branch of the integration checkout, not automatically
`main`.

Prepare requirements, scope, and executable acceptance criteria before a large
implementation. Open a Draft PR when there is a useful reviewable change or
plan; one PR can cover multiple packets. Keep commits focused and use English
Conventional Commit subjects, such as `fix: preserve cleanup evidence`.
Agents must follow the user's existing commit, push, and merge authorization;
do not ask again for authorization already given, or treat it as authorization
for a different operation.

See the [workflow guide](en/workflow.md) for contract preparation, worktree
selection, task recovery, and PR delivery. Worktrees are not security sandboxes,
and the current engine's `in-place` fallback still applies.

## Implement against observable contracts

Preserve these rules from the design and AGENTS files:

- Task state belongs to the Go engine and `internal/state`; every state write
  uses revision compare-and-set and atomic replacement. Models and skills do
  not edit `state.json`.
- The contract schemas define valid input. Validate at load time, and update
  design documentation before changing a wire contract such as error taxonomy.
- Test completion, cancellation, timeouts, and process cleanup using observable
  results. A terminal text marker or a worker exit is not evidence that a
  separate child process group has ended.
- Verification and review apply to the same candidate that is committed and
  merged. Preserve work and evidence when verification or Git delivery fails.
- Unknown usage, cost, permissions, or model capabilities stay unknown. Do not
  turn missing observations into success or zero cost.
- Skills validate inputs and invoke the CLI. Do not move state-machine or retry
  logic into skill prose or the desktop plugin.

Use meaningful failure-path tests and narrow I/O injection points. The project
requires 100% statement coverage for every measured Go function; the trivial
`cmd/flywheel` entry stub is excluded. Do not weaken that gate or add assertions
that merely repeat the implementation to increase the percentage.

## Build and verification

Use Go **1.27 or later** and Git. The published runtime requires Node
**22.18.0 or later**. CI and release work use Node **24.22.1** and npm **12.2.1**;
use those pinned versions when reproducing packaging and installation results.

The following commands run from the repository root:

```sh
make quality-gate
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro)NodeWorkerOfflineBridge$' -count=1
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

Go changes require `make quality-gate`, which includes vet, formatting, tests,
and coverage. Plugin changes additionally require the Node and process-bridge
checks. npm installation or release changes require the root npm tests, a
six-package candidate build, and the real npm installation test. Run the relevant
checks for the change, and report exactly what ran. Documentation-only edits
still need valid links, correct commands, and any affected embedding or package
content checks; do not claim an old artifact contains newly edited documentation.

For lifecycle, state, and process changes, the Linux CI race check is:

```sh
go test -race ./internal/machine ./internal/verify ./internal/watchdog ./internal/drivers ./internal/cli ./internal/modules ./internal/diagnostic -count=1
```

The default tests do not call paid models or use a real desktop profile. They
exercise real Git operations, processes, workers, and the pinned Codex Pro
package with protocol fixtures. Installation tests use actual candidate tarballs
with an isolated registry and user directory; DSH desktop registration is a CLI
fixture. Package fetching still needs registry access or the documented verified
dependency fixture. These tests do not establish live account access, model
quality, Mac desktop behavior, or subscription quota.

CI runs Ubuntu and macOS against the same candidate; Linux also runs the race
checks. Record an unavailable runner or environment as unavailable. Local passes
do not become remote CI passes, and cross-compiling a macOS binary does not test
its runtime behavior.

## Documentation and review

Update the matching English and Chinese user documentation when behavior
changes: installation, configuration, workflow, and the plugin entry points.
Keep command names, model IDs, JSON fields, permission semantics, and validation
limits consistent. Historical design and review records do not all need to be
translated. If a linked reference remains Chinese, label it clearly.

Check the installed package as well as the source tree when changing npm
documentation. A file must be included in the package manifest to accompany the
tarball; a working repository link does not prove that an installed relative
link resolves. Maintain evidence for the exact candidate being reviewed.

A PR should explain the concrete problem, changed behavior, test results, and
remaining validation limits. Review the final diff and evidence independently
of the implementing model's completion claim. Follow the repository's active
merge requirements and the user's authorization; do not weaken required checks
or change repository settings to get a merge through. Source merge and npm
publication are separate operations.

## Release changes

Follow the [release guide](en/releasing.md). All six Flywheel packages share an
exact version, with exact internal dependencies and the pinned Codex Pro runtime.
Build once from the final clean source and test those exact tarballs. Preserve
the candidate's source commit, tree, snapshot hash, and package integrity values.
The release tag must identify that source. Never relabel a dirty development
candidate as a clean release or rebuild different bytes for a published version.

Use the existing release scripts for candidate checks, publication, registry
verification, and promotion. Publish platform packages and the plugin before
the meta package, test the exact registry version before moving `latest`, and
verify all six final tags and integrities. Preserve the same candidate when
recovering from a partial publication. Manual publication needs its own explicit
authorization and an accurate record of checks that could and could not run; it
does not justify claiming that an unavailable CI or desktop check passed.
