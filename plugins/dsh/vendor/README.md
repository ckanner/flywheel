# Verified companion packages

Flywheel keeps exact reviewed package bytes. Codex Pro 0.10.2, Claude Code Pro
0.7.0 and Antigravity 0.2.1 are published companion versions. The
[2026-11-22 registry verification](registry-verification-2026-11-22.json)
records fresh downloads from the official npm registry: all three archives
match their original reviewed vendor SHA-256 and integrity. The Claude
subscription change, [PR #12](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/12), is merged.
Antigravity 0.2.1 is an independent npm plugin whose complete source is maintained
at [dsh-subagent-antigravity](https://github.com/ckanner/dsh-subagent-antigravity)
and bundled from its reviewed tarball. Registry availability and the original
source-build provenance are recorded separately.
LCU and ChatGPT below retain their previously verified registry identities. The
private preset generator remains bundled from reviewed source.

| Package | Role | Source commit | Source tree | Record |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.2` | Bundled published machine API | [18079d8b8c75](https://github.com/ckanner/dsh-subagent-codex-pro/commit/18079d8b8c751f5b50375473a96960eeb68b61be) | `b7c2e312c66dffb9b4aff6ce8c1b523cf5a3e7fe` | [codex-pro.json](codex-pro.json) |
| `dsh-preset-generator@0.4.0` | Bundled private PTC composition | [3dc21791b1ef](https://github.com/ckanner/dsh-preset-generator/commit/3dc21791b1efe81bea9a26aa9e92ff2172b4f645) | `877ea728cad4d8ef4b21a691df2a03cfc9075efc` | [preset-generator.json](preset-generator.json) |
| `dsh-subagent-claude-code-pro@0.7.1` | Optional published Agent SDK subscription machine peer | [377836d4f375](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/377836d4f375870c84f6d612cc9763043356442a) | `a4cdc1367bd6ec7650c4d971d469e284ce9cc93a` | [claude-code-pro.json](claude-code-pro.json) |
| `@ckanner/dsh-subagent-antigravity@0.2.1` | Bundled adapter for official agy CLI, Orchestrator/subagents/Swarm and standalone DSH | [a20eb63bbfa2](https://github.com/ckanner/dsh-subagent-antigravity/commit/a20eb63bbfa2849df63043595475957bf4a4f04d) | `01bbc8964c37e40106b3b4be9aaf97d9a5df36dd` | [antigravity.json](antigravity.json) |
| `dsh-plugin-lcu@0.5.0` | Optional managed desktop peer | [505e516cbc90](https://github.com/ckanner/dsh-plugin-lcu/commit/505e516cbc909556a7b9855a454202cc28b583a8) | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | [lcu.json](lcu.json) |
| `dsh-plugin-chatgpt@0.1.20` | DSH provider compatibility fixture | [40d8f885e7a3](https://github.com/ckanner/dsh-plugin-chatgpt/commit/40d8f885e7a386af5f441b356b9c68496a47b0e3) | `c8c4f4829a00b1917fd1aef8c4254e083a5b6180` | [chatgpt.json](chatgpt.json) |

Published-package verification includes the registry URL, exact integrity,
SHA-256 and verification timestamp. Its verification toolchain does not claim how
the publisher built the archive. Original source-build records retain
`registryReleaseVerified:false`: this describes what was verified when those
records were created, not whether a version is available now. Later publication
is recorded separately without rewriting source commits, trees or archive hashes.
Each release attempt still requires its fresh registry check; the dated record
above cannot authorize a future publication or prove account access.
The reviewed Claude and Antigravity archives were built and installed with
Node 24.22.3 / npm 11.19.1; each companion retains its own toolchain and exact
archive identity in the records above. A test-only follow-up can have a different source tree while
preserving the packaged bytes; such a check does not rewrite the archive's
original source identity.

The previous Antigravity 0.1.0 candidate's test-only npm 11/12 packaging and macOS temporary-root follow-ups
were recorded separately as
verification source `4b7fda3cf2fecf2a413c2e303518c4f34f7eb749` (tree
`2399a6daa71f88a461aab279396d65ef29fc2440`). Both real pack/install checks
reproduced the original archive SHA-256 and SHA-512. The physical-root fixtures
also passed the complete plugin and real DSH host checks under a symlink TMPDIR.
Its packaging helpers and tests were excluded from that published file set; its
original archive identity remains in the dated subscription-CLI validation record.

The preceding 0.2.0 candidate was reviewed after adding fixed-effort Orchestrator,
explicit collaboration, bounded child observations and the DSH resume fix. The
same archive bytes passed external installation/ESM/types/bin consumption and a
real disposable DSH profile install. [Follow-up validation](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-20-orchestration-adaptive-selection.md)
records the scope and limits of that earlier acceptance. The current 0.2.1
archive and its later registry verification are identified separately above;
package publication alone does not establish real-account model inference.

Earlier published versions and reviewed candidates remain identified in dated
validation records. In particular, the former Codex 0.10.0 and Claude 0.5.1
archives are not relabeled as the new activity/naming candidates. ChatGPT 0.1.20
remains the reviewed provider fixture. See the
[provider validation record](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-16-provider-liveness-and-titles.md)
for the initial implementation. The subsequent [review closure](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-16-provider-liveness-review-closure.md) records the earlier archives, additional fixes, exact-candidate acceptance and its limits. The [Codex follow-up](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-16-codex-review-fixes.md) records the preceding child-creation and required-naming fixes, regression results and exact-candidate acceptance. The [native history review](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-17-native-session-visibility.md) records the current retention and discovery fixes. Current source identities and archive hashes are in the table and records above.

Codex Pro, Antigravity and the generator are exact bundled runtime dependencies. Both source
locks resolve the corresponding vendor tarballs with actual integrity. Claude
and LCU remain optional peers; only the private test harness installs them by
default. PR CI verifies the checked-in Claude candidate's package name/version,
SHA-256 and integrity, and supplies those same bytes to both OS jobs. Actual
Flywheel publication and promotion must obtain the optional Claude
0.7.0 companion from the public registry and verify that it matches the reviewed
bytes on every attempt. Codex and Antigravity are distributed inside the complete Flywheel package;
their bundled bytes are checked independently of their separate registry releases. The Antigravity
tarball is also shipped for deliberate standalone DSH installation. Source-candidate CI
does not satisfy that publication prerequisite. ChatGPT is a provider fixture,
not an additional implicit model backend.

The [Agent SDK migration](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-17-claude-agent-sdk.md)
records Claude 0.6.0's exact source/SDK identities, native history acceptance,
authentication boundaries and SDK-tail review corrections.

The machine entry points remain independent of optional desktop peers. Desktop
native settings belong to the desktop plugin; direct Flywheel workers use their
own frozen native profile. Installing a package in the desktop profile does not
make it available in the global worker resolution tree or upgrade the SDK profile.
See [native setup](https://github.com/ckanner/flywheel/blob/main/docs/en/native-capabilities.md) and
the [desktop validation record](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-14-desktop-native-configuration.md).

No native provider executable, credentials, or private DSH application resources
are bundled. Updating these records does not publish Flywheel or alter user
profiles.
