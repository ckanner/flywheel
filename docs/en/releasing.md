# Releasing Flywheel

[Documentation](../README.md) · English · [简体中文](../zh-CN/releasing.md)

This runbook is for maintainers. Users should follow [getting started](getting-started.md).
A commit, tag, or successful pack is not a registry release. Record actual source,
installation, registry and desktop results separately in [validation records](../validation/README.md).

## 1. One complete package

From 0.33, publish only **`@ckanner/flywheel`**. One tarball contains:

- The CLI and explicit installer in `bin/` and `npm/`.
- DSH host, client and machine workers in `dist/`; the Node modules use `.mjs`.
- Four Go executables under `native/{darwin,linux}-{arm64,amd64}/flywheel`.
- `native/manifest.json`, identifying their version, sizes and SHA-256 hashes.
- Embedded skills/templates, paired guides, the MIT license and third-party notices.

The launcher selects and verifies its own platform binary; it does not download a
binary on first use or resolve an old platform/worker package. `plugins/dsh` is a
**private source/build/test harness**, never a second publication target. The four
old platform packages and `@ckanner/dsh-plugin-flywheel` are historical releases;
this flow neither uploads new versions of them nor unpublishes their existing bytes.

Codex Pro 0.10.1 is bundled from its [reviewed source candidate](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md).
The root and source-harness locks retain that candidate tarball's actual integrity;
this does not claim the companion has been published. Claude Code
Pro 0.5.2 is an optional external peer. Provider CLIs and account credentials are
not included. Ordinary npm installation only places files; `install`/`setup` owns
user setup, with no profile or credential changes hidden in postinstall.

The reviewed Claude 0.5.2 source archive is pinned in the vendor record.
PR CI verifies its checked-in package identity, SHA-256 and integrity; this is not
registry publication evidence. Actual publish and promotion paths must obtain the
exact companion from the public registry and compare it with the reviewed vendor
record before changing Flywheel's registry state. The local release path also
passes that verified tarball to installation tests, so the optional Claude test
cannot silently be skipped. An unavailable or different companion blocks release.

## 2. Source and validation

Use Go **1.27**, Node **24.22.1** and npm **12.2.1**, matching the workflows.
Require reviewed source, a clean checkout, and `vX.Y.Z` pointing at exact HEAD.
Never move a release tag, relabel a dirty candidate, or rebuild different bytes
for an already uploaded version.

Run applicable [contribution checks](contributing.md):

| Area | Evidence |
| --- | --- |
| Go changes | `make quality-gate`, retaining per-function 100% coverage, and core race checks |
| DSH and workers | Locked source-harness build/tests and real Go→Node offline bridges |
| npm and release | `npm test`, one complete candidate build, and actual installation of that tarball |
| Native/session changes | Native CLI checks and `npm run test:sessions` as applicable |
| Migration | Fresh install, split-package upgrade, failure/rollback/cancellation and repeat setup |
| Documentation | Paired guides, valid links, packaged documents and licenses |

Install the exact reviewed Claude source tarball for its backend tests; absence
or a skipped optional peer does not count as a pass. Set
`FLYWHEEL_CLAUDE_CODE_PRO_TARBALL` to that tarball before installation tests.
Source tests use isolated homes/profiles and offline model fixtures. They do not
prove live account access, model quality, or physical desktop behavior.

## 3. Build once and test the same artifact

From the final clean tagged checkout, stop on any failure:

```sh
FLYWHEEL_RELEASE_VERSION=$(node -p "require('./package.json').version")
FLYWHEEL_RELEASE_SHA=$(git rev-parse HEAD)
node scripts/release-preflight.js --version "$FLYWHEEL_RELEASE_VERSION" --tag "v$FLYWHEEL_RELEASE_VERSION" --json
npm run build:npm
node scripts/release-publish.js verify-candidate --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

The builder captures an isolated source snapshot, installs both locked dependency
sets without lifecycle scripts, builds the plugin and four native binaries, and
packs the root exactly once. It never reuses caller `dist/` or `node_modules/`.
Candidate **format 2** records the source commit/tree/snapshot, dirty status and
one tarball's SHA-512 integrity. Verification also opens the tarball and checks
all four native hashes, required entry points, licenses and bundled adapter identity.
Linux and macOS must install the same candidate, not separately repack it.

Keep `dist-npm/`, `candidate.json`, toolchain identity and results. Source changes
before upload require a new reviewed candidate. After upload, retain those exact
bytes; a source fix requires a new version.

## 4. Automatic GitHub Actions publication

Pushing the reviewed tag triggers [publish.yml](https://github.com/ckanner/flywheel/blob/main/.github/workflows/publish.yml).
Manual dispatch accepts an exact existing tag. Build and both OS checks precede
the publication job, which rechecks source and tag identity.

Configure **one** npm Trusted Publisher for `@ckanner/flywheel`:

| Setting | Value |
| --- | --- |
| Provider | GitHub Actions |
| Organization/user | `ckanner` |
| Repository | `flywheel` |
| Workflow filename | `publish.yml` |
| Environment | Empty, matching the workflow |
| Permissions | Allow `npm publish` and `npm dist-tag` |

The publication job uses OIDC only, with an empty npm user config; it does not
fall back to a stored `NPM_TOKEN` or interactive browser authentication. The
account's 2FA policy remains independent. Creating the trust relationship can
require a maintainer security-key confirmation; that is initial setup, not a
per-release approval. npm may mark a new connection pending validation until its
first real CI publication. Saved settings or `npm publish --dry-run` do not prove
that publication has succeeded. See [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/).

The public source repository requires provenance. Private-source publication
explicitly disables unsupported provenance; it must never falsely claim that a
public repository is private. If Actions is blocked by billing or another runner
failure, report that separately from local checks. A successful CI verification
of existing packages is not proof that fresh OIDC writes work.

## 5. Publication order and recovery

Publish the complete tarball under `candidate-X-Y-Z`, verify its exact registry
integrity, and run the actual registry installation before moving `latest`.
Finally verify `latest` and integrity. There is one package upload and one tag
promotion, both authenticated by the CI identity; no six-package transaction is needed.

Only a structured E404 means a version is absent. Authentication, transport,
cancellation, malformed JSON and mismatched integrity fail visibly. Existing
versions must match the saved candidate, and a newer `latest` must never be downgraded.
Visibility checks retain their bounded 180-second window and repeat reads, not writes.
If npm has accepted an upload but is still validating it, retain the artifact and
wait for the exact version to become visible before resuming.

| Failure | Recovery |
| --- | --- |
| OIDC authorization | Verify the exact package/repository/workflow and both allowed actions |
| Interrupted upload | Read registry identity first; resume the same saved tarball |
| Registry visibility timeout | Preserve accepted bytes and retry read-only verification |
| Registry installation or promotion fails | Resume the same candidate; keep newer tags intact |
| Code must change after upload | Use a new version and reviewed tag |
| Candidate is missing | Recover the original artifact; do not assume a rebuild is identical |

For an explicitly authorized **private-source** emergency only, `npm run release:local --
--repository-visibility private` retains source/tag/candidate/installation checks and
normal interactive npm authorization. It requires a TTY. An older release must use
its own tooling, exact tagged worktree and original artifact format. Do not invoke
the new single-package publisher against historical format-1 six-package candidates.

## 6. User migration and acceptance

`npx -y @ckanner/flywheel@latest install` remains the entry point. Wait for active Flywheel tasks to settle and quit DSH before
migrating a split installation. Setup installs and verifies the complete desktop
package, then removes the old `@ckanner/dsh-plugin-flywheel` through the official
DSH CLI and verifies only the new identity remains. It preserves user configuration,
modified skills and task histories; it does not rewrite `state.json`.

A custom file/git legacy source fails before desktop-profile mutation; earlier
completed installer steps remain in the report. If migration fails, report
its actual partial state. Roll back only a replacement added by this attempt when
the original old package is still confirmed present. Cancellation does not start
new rollback processes; retry reads the profile again and completes the remaining
work. Never report success while both identities remain. Existing global launcher
records and explicit `flywheelBinary` overrides retain their checks and precedence.
Without a record, the installed plugin can resolve its own bundled CLI; Node still
needs to be available to the host.

A complete release record includes source/candidate/registry identities, installation
results and limitations. Separately verify CLI, skills, the installed desktop package,
configuration and the reloaded native view. Running tasks must not be interrupted just
to claim desktop acceptance; record a pending reload when necessary.

An existing managed launcher must match the installed package version. After updating
only the DSH copy, run setup for that version to refresh the managed installation;
a stale record is reported instead of mixing a newer view with an older engine.
