# Releasing Flywheel

[Documentation](../README.md) · English · [简体中文](../zh-CN/releasing.md)

This runbook is for maintainers. Users should follow
[getting started](getting-started.md). A source commit, GitHub Release, or
successful `npm pack` does not establish that a version is available from npm.
Actual releases and verification limits belong in the
[validation records](../validation/README.md).

## 1. Release identity and prerequisites

One release contains six packages at the same exact version, published in
the following order:

| Order | Package | Contents |
| --- | --- | --- |
| 1–4 | `@ckanner/flywheel-{darwin,linux}-{arm64,amd64}` | Four platform-specific Go binaries |
| 5 | `@ckanner/dsh-plugin-flywheel` | Desktop plugin and workers |
| 6 | `@ckanner/flywheel` | User-facing CLI, installer, and documentation |

The meta package pins all five dependencies. Every Flywheel package declares
MIT and includes its `LICENSE`; the plugin retains third-party notices.
The Codex Pro dependency is pinned to 0.9.0 and bundled from the
[recorded source candidate](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md). Claude Code
Pro 0.4.0 remains an optional peer, not a seventh Flywheel release package.

Use Git, Go **1.27**, Node **24.22.1**, and npm **12.2.1**, matching the
repository workflows. The end-user Node requirement is lower; the release
toolchain is deliberately fixed. The reviewed source must be clean, all six
versions must agree, and the exact `vX.Y.Z` tag must point to HEAD. Never move
an existing release tag or replace already-published package bytes.

`candidate.json` records the source commit, tree, snapshot SHA-256, and each
tarball's SHA-512 integrity. Build once, then verify, test, and publish those
same tarballs. A candidate with `source.dirty: true` is for development only.

## 2. Validate the release source

Run the checks required by the [contribution guide](contributing.md) and
the current [CI workflow](https://github.com/ckanner/flywheel/blob/main/.github/workflows/ci.yml):

| Area | Required evidence |
| --- | --- |
| Go | `make quality-gate` and the workflow's core-package race checks |
| DSH plugin | Locked dependency installation, build, and behavior tests |
| Backend boundary | DSH, Codex Pro, and Claude Code Pro offline bridge tests |
| npm | `npm test` and real installation of the six-package candidate |
| Native CLI | Build, version/help, and JSON configuration checks |
| Session policies | `npm run test:sessions` with the built native binary |
| Documentation | Links, language parity, packaged guides, and licenses |

Prepare the exact optional Claude tarball using the workflow's pinned source
SHA and SHA-256 check. For plugin/bridge tests, the workflow temporarily
installs it as a development dependency and restores the source manifests.
Export `FLYWHEEL_CLAUDE_CODE_PRO_TARBALL` as the absolute path to that verified
tarball before running candidate installation tests.
An absent optional peer does not count as a passed Claude backend check.

These checks use isolated profiles and offline model fixtures. They do not
establish live account readiness, model quality, or physical desktop layout.
Record the actual platforms tested; cross-compilation does not establish
runtime verification on the target platform.

## 3. Build one candidate

From the clean checkout of the reviewed release tag, run each command and
stop on failure:

```sh
FLYWHEEL_RELEASE_VERSION=$(node -p "require('./package.json').version")
FLYWHEEL_RELEASE_SHA=$(git rev-parse HEAD)
node scripts/release-preflight.js --version "$FLYWHEEL_RELEASE_VERSION" --tag "v$FLYWHEEL_RELEASE_VERSION" --json
npm run build:npm
node scripts/release-publish.js verify-candidate --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

Set the Claude tarball environment variable described above before the final
command. The build installs locked plugin dependencies inside an isolated
source snapshot, with lifecycle scripts disabled, then builds the workers and
four Go binaries. It does not reuse the caller's `node_modules` or `dist`.
Root-level `npm install` is not required.

Preserve `dist-npm/`, its `candidate.json`, source identity, toolchain versions,
and validation results. Verify all six packed MIT licenses and the root and
plugin language editions. If source changes before any upload, prepare a new
candidate from the new reviewed source. After any upload, retain the original
candidate and follow the recovery rules below.

## 4. GitHub Actions publication

Pushing the reviewed tag triggers [publish.yml](https://github.com/ckanner/flywheel/blob/main/.github/workflows/publish.yml).
An existing tag can also be supplied to its manual dispatch. Linux and macOS
verify the same candidate artifact; the publish job refreshes remote refs and
requires the tag to still identify the checked source.

GitHub access and npm publication rights are separate. Configure trusted
publishing for each of the six packages with owner `ckanner`, repository
`flywheel`, workflow `publish.yml`, and no environment unless the workflow is
changed to use one. Both **npm publish** and **npm dist-tag** permissions are
needed. The workflow also supports an authorized `NPM_TOKEN` fallback.

New packages require npm's initial package-creation authorization before
their trusted publisher can be configured. Use the normal authorized npm
flow; do not upload different placeholder bytes under the release version.
See the official [trusted publisher](https://docs.npmjs.com/trusted-publishers/)
and [dist-tag](https://docs.npmjs.com/adding-dist-tags-to-packages/) documentation
for account setup. `npm whoami` identifies an account, not its publishing rights.

Publication requires explicit repository visibility: `public` requires
provenance; `private` disables unsupported private-source provenance. Choose
the repository's actual visibility. Publishing a package does not change
repository visibility, and OIDC authentication is separate from provenance.

## 5. Publish locally from private source

A maintainer can build from a normal Git checkout. With the current release
policy, new uploads from a public repository must use section 4: the publisher
requires provenance, which npm generates only in a supported cloud CI/CD
environment, not an ordinary local browser-login session. See
[npm provenance requirements](https://docs.npmjs.com/generating-provenance-statements/).
The local upload example below applies only when the source repository is
actually private. Do not label a public repository private to bypass this
requirement. No downloaded archive is required. Complete source review and
the checks above first. If Actions cannot run, record the maintainer's
authorization, actual local results, and checks that were not performed;
do not report unavailable CI as passing.

From the same clean tagged source and its original candidate:

```sh
npm login --auth-type=web --scope=@ckanner --registry=https://registry.npmjs.org/
npm run release:local -- --repository-visibility private
```

Complete the normal browser/2FA prompts in the terminal. The publisher
requires a TTY; do not pipe its output through `tee`.

`release:local` performs these steps:

1. Recheck clean source, exact local and remote tag, tree, snapshot, candidate
   manifest, and package integrity.
2. Run candidate installation, then verify the source and candidate again.
3. Upload platform packages, plugin, and meta under `candidate-X-Y-Z`.
4. Install the exact version from the official npm registry.
5. Recheck source and remote tag, promote `latest` in the same order, and
   verify all six tags and integrities.

The command uses the repository's existing release checks. Only npm publish
and dist-tag writes inherit terminal I/O; registry queries retain strict JSON
parsing and bounded timeouts. The command does not log in, create or move
tags, or change account security settings.

For a reviewed older release that needs newer tooling, `--source-directory`
can point at its exact tagged worktree and `--directory` at its original
candidate. The source defaults to the tooling checkout; the candidate
directory defaults to `dist-npm/` inside the selected source directory. Explicit
relative paths resolve from the invocation directory. Do not copy newer
tooling into old release source or rebuild an already-published version.

## 6. Visibility, promotion, and retry behavior

Packages first use the candidate tag. All exact versions and integrities must
be visible and registry installation must succeed before `latest` moves.
Platform packages are promoted first, the plugin next, and the user-facing
meta package last. npm has no atomic six-package tag transaction.

Only a structured E404 means a queried version is absent. Authentication,
transport, cancellation, malformed output, or missing integrity fails the
operation. Registry queries accept the supported scalar and single-element
array response shapes; multiple results are rejected.

After upload, visibility checks have a 180-second budget per package, with
E404 backoff from 2 seconds to at most 15 seconds. Each read is bounded by
30 seconds and the remaining budget. Promotion verification uses the same
bounded approach for `latest` and integrity. It repeats reads, not writes,
and refuses a newer `latest`, conflicting integrity, or invalid version.

| Failure | Recovery |
| --- | --- |
| Authentication or browser approval | Resolve the authorized npm login; retain the candidate. |
| Some packages uploaded | Retry the original candidate; existing versions must match its integrity. |
| Registry visibility timeout | Preserve the artifact and retry after the external issue is resolved. |
| Installation or tag promotion failed | Resume with the same candidate; do not downgrade a newer `latest`. |
| Source must change after upload | Use a new uniform version and a new reviewed tag. |
| Original candidate is missing | Recover it; do not assume a rebuild has identical bytes. |

With Actions, rerun failed jobs using the original artifact. Locally, rerun
the final `release:local` command against the preserved source and candidate.

## 7. Record completion

Publication is complete only after all six exact versions and integrities,
registry installation, and final `latest` checks have passed. Record those
results separately from source review, local tests, CI, and desktop acceptance.
For an additional exact-version installation check:

```sh
FLYWHEEL_NPM_VERSION=X.Y.Z npm run test:registry
```

Replace `X.Y.Z` with the released version. Record source SHA, candidate
identity, actual platforms, failures/retries, and remaining limits in a dated
validation record and release notes. See the
[0.30.2 release record](../validation/2026-11-11-release-0.30.2.md) for a
historical example; it is evidence for that release, not a live registry status.
