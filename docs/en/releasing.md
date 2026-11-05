# Releasing Flywheel

English · [简体中文](../releasing.md)

This guide is for maintainers. Users install from npm using the
[getting started guide](getting-started.md). A merged PR, a GitHub Release or
a successful local build does not establish that npm installation is ready.

The subscription-model and migration release target is **0.28.2**. Its [local review record](../validation/2026-11-04-release-0.28.2.md) separates source checks from npm publication, latest promotion and desktop acceptance. Version 0.28.2 includes the interactive release tool: build its own clean exact tag, then run `npm run release:local -- --repository-visibility private`. The historical 0.28.0 two-checkout example below is not required for this new release.

## Release contract

One release contains six packages at the same exact version, in this order:

1. `@ckanner/flywheel-darwin-arm64`
2. `@ckanner/flywheel-darwin-amd64`
3. `@ckanner/flywheel-linux-arm64`
4. `@ckanner/flywheel-linux-amd64`
5. `@ckanner/dsh-plugin-flywheel`
6. `@ckanner/flywheel`

The meta package pins the plugin and platform dependencies. The plugin pins
`dsh-subagent-codex-pro@0.8.0`. Release tooling uses Node **24.22.1**, npm
**12.2.1** and Go **1.27**. These are maintainer toolchain requirements; users
need Node 22.18.0 or later and do not need Go to install from npm.

Build once from a clean, reviewed checkout whose `vX.Y.Z` tag points at exact
HEAD. `candidate.json` records the source commit, tree, snapshot SHA-256 and
every tarball's SHA-512 integrity. Verify, test and publish those same bytes.
Development candidates with `source.dirty: true` cannot be published.

For the native Conversation View source, the isolated build installs the
plugin's locked development and optional dependencies with `npm ci` and
disabled lifecycle scripts before compiling its CSS Modules. It does not
reuse a developer's `node_modules` or `dist` directory. These build
dependencies are not runtime dependencies of the installed plugin.

Upload under `candidate-X-Y-Z` first. After all exact versions match their
recorded integrity and registry installation succeeds, promote `latest` in
the order above, with the user-facing meta package last. npm has no atomic
six-package promotion; the scripts support safe retries and refuse to move
a newer `latest` backwards.

## Normal GitHub Actions path

Merge the reviewed source after required checks pass, then create the release
tag at that exact commit. The tag triggers
[`publish.yml`](../../.github/workflows/publish.yml). An existing tag can also
be supplied to that workflow's manual dispatch. Linux and macOS verify the
same candidate artifact; publishing refreshes the remote tag and requires it
to still identify the verified source.

GitHub access is separate from npm publishing permission. Configure a trusted
publisher for **each of the six packages**, with owner `ckanner`, repository
`flywheel`, workflow `publish.yml`, no environment, and permission for both
`npm publish` and `npm dist-tag`. The workflow also supports an authorized
`NPM_TOKEN` fallback. Do not copy credentials into logs or task records.

The publishing command requires explicit repository visibility. A private
repository uses `--repository-visibility private`, which disables unsupported
provenance generation. A public repository uses `public`, which requires
provenance. Do not change visibility to make publication work. Trusted
publishing authentication and provenance are separate capabilities.

See npm's official [trusted publishing](https://docs.npmjs.com/trusted-publishers/)
and [dist-tag](https://docs.npmjs.com/adding-dist-tags-to-packages/)
documentation for account setup. `npm whoami` identifies an authenticated
account; it does not prove publish or dist-tag authorization.

## Authorized manual release when Actions cannot run

A maintainer may explicitly authorize a manual release when the CI service
cannot start, as with the recorded account billing blockage for 0.28.0.
Record that authorization and the actual checks performed in the PR or release
notes. Keep the normal workflow intact. Linux tests and macOS cross-compilation
do not establish macOS runtime, desktop, account or model-call verification.

Start from the final merged commit, including documentation changes. Check
the remote tag freshly through authenticated Git or the GitHub API; a cached
local tag is insufficient. Use the same checkout for these commands:

```sh
export npm_config_registry=https://registry.npmjs.org/
FLYWHEEL_RELEASE_VERSION=0.28.0
FLYWHEEL_RELEASE_SHA=$(git rev-parse HEAD)
node scripts/release-preflight.js --version "$FLYWHEEL_RELEASE_VERSION" --tag "v$FLYWHEEL_RELEASE_VERSION" --json
make quality-gate
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro)NodeWorkerOfflineBridge$' -count=1
go test -race ./internal/machine ./internal/verify ./internal/watchdog ./internal/drivers ./internal/cli ./internal/modules ./internal/diagnostic -count=1
npm test
bash scripts/build-npm.sh "$FLYWHEEL_RELEASE_VERSION" dist-npm
node scripts/release-publish.js verify-candidate --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

The explicit registry replaces the CI setup-node registry setting; do not
inherit a fixture or private mirror for release writes. Run commands
individually and stop at any failure. Preserve the six tarballs,
`candidate.json`, toolchain versions and test results in a durable release
artifact before uploading. If the source changes, discard the unpublished
candidate and repeat from the new clean source. Never rebuild a replacement
for a version already accepted by the registry.

Use a legitimate npm publishing session or an authorized token. A normal
interactive login is `npm login --auth-type=web --registry=https://registry.npmjs.org/`.
Local execution cannot use an Actions OIDC identity. Do not disable account
2FA or change package security settings to make a manual run succeed.

For credentials that support non-interactive publication, the existing script
performs the complete upload and promotion checks:

```sh
node scripts/release-publish.js publish --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA" --repository-visibility private
FLYWHEEL_NPM_VERSION="$FLYWHEEL_RELEASE_VERSION" npm run test:registry
node scripts/release-publish.js promote --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
node scripts/release-publish.js verify-latest --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
```

The standard CI runner continues to capture npm output. For a browser/2FA
session, use the repository's `npm run release:local` command below. It reuses
the existing release functions and inherits terminal I/O **only for `npm
publish` and `npm dist-tag` writes**. Registry queries retain strict JSON
parsing, timeouts, kill signals and buffer limits. A pending browser approval
is not a successful publish.

The registry smoke test installs the exact version from npm, runs the native
CLI and embedded skills, and exercises DSH registration through a protocol
fixture. It does not call a model or prove the user's live desktop is ready.
Refresh and compare the remote tag again before publication and promotion.

## Local build and publication from source

A prebuilt archive is optional. A maintainer can clone the repository, build
the six packages on their Mac/Linux machine and publish them from a normal
terminal. Use Git, Go 1.27 (1.27.3 was validated for this release), Node
24.22.1 and npm 12.2.1, with authorized GitHub read access and npm publishing
access. Run and record the source quality gates above; the local publisher
provides candidate and registry installation gates, not Go quality/race
validation or source review.

The existing `v0.28.0` tag remains fixed at
`c865d2c130aa009f2348a883c01d0ac29da59624`. The newer interactive tooling lives
on `main`, so keep a current checkout for the tooling and a detached worktree
for the exact release source. Both come from Git; no downloaded candidate is
required and no existing tag is moved. From the current `main` checkout, run
these commands individually and stop at any failure:

```sh
export npm_config_registry=https://registry.npmjs.org/
git fetch origin --tags
git worktree add --detach ../flywheel-v0.28.0 c865d2c130aa009f2348a883c01d0ac29da59624
node ../flywheel-v0.28.0/scripts/release-preflight.js --version 0.28.0 --tag v0.28.0 --json
npm --prefix ../flywheel-v0.28.0 run build:npm
npm login --auth-type=web --scope=@ckanner --registry=https://registry.npmjs.org/
npm whoami --registry=https://registry.npmjs.org/
npm run release:local -- --source-directory ../flywheel-v0.28.0 --repository-visibility private
```

The candidate is written to `../flywheel-v0.28.0/dist-npm/`. No root
`npm install` is needed before building: the build script compiles the worker
and all four Go binaries. `--source-directory` defaults to the tooling
repository; `--directory` selects another candidate directory. Explicit
relative paths are resolved from the invocation directory. The source
checkout must be clean and its version, local/remote tag, HEAD, tree,
snapshot and candidate integrity must all agree.

`release:local` performs the following sequence:

1. Verify the complete source/candidate identity and query the source
   repository's `origin` with `git ls-remote` for a fresh tag check.
2. Run the real candidate installation test on this machine, then recheck
   the source, candidate and remote tag.
3. Publish platform packages, plugin and meta under `candidate-X-Y-Z`,
   retaining existing integrity and newer-`latest` checks.
4. Install the exact version from the official npm registry. Failure stops
   before promotion.
5. Recheck the source, candidate and remote tag, promote all six `latest`
   tags and verify completion.

Use a real terminal and complete npm's normal browser/2FA prompts. Do not
pipe the publisher through `tee`. The command does not log in, create or move
tags, change repository visibility, or alter npm security settings. `private`
matches the current repository; public-source releases must use `public`
and satisfy provenance requirements.

After an interruption, keep the source worktree and its original `dist-npm/`,
resolve the failure and rerun only the final command. Existing versions are
skipped only when their integrity matches. Do not rebuild over a candidate
that may already be uploaded. A newly built candidate that conflicts with an
existing version must stop: recover the original candidate or arrange a new
uniform version.

Installation tests use isolated user directories and a DSH protocol fixture,
with no model requests. On a Mac they exercise the native Darwin CLI/worker
for the current architecture; they do not prove the live desktop or account
works. Record the source SHA, candidate manifest, actual platform and test
results in the GitHub Release before marking publication complete.

## Recovery and completion

Only a failed npm response with structured `E404` means a version is absent.
Authentication, network and malformed-response errors stop publication.
Registry visibility checks use a bounded 180-second deadline per package;
an accepted upload that is temporarily invisible is not permission to rebuild.

If some packages are already published, retain the original artifact and
retry it. Existing versions must have exactly matching integrity. Resolve an
installation or authentication failure before promoting; never unpublish,
overwrite or retag different bytes to repair an incomplete release.

A release is complete when all six exact versions match the candidate,
registry installation passes and all six `latest` tags identify that version
with the same integrity. The record must identify whether this was an Actions
release or an authorized manual release, list any unperformed platform checks,
and link the source/tag and validation evidence. Historical release outcomes
remain in the [Chinese release log](../releasing.md#3-失败恢复).
