# Verified companion packages

Flywheel keeps exact reviewed package bytes. Codex Pro 0.10.2 and Claude Code
Pro 0.7.0 are reviewed source candidates; the Claude subscription change is
[PR #12](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/12).
Antigravity 0.2.0 is an independent npm plugin whose complete source is maintained
at [dsh-subagent-antigravity](https://github.com/ckanner/dsh-subagent-antigravity)
and bundled from its reviewed tarball. These records do not claim registry release.
LCU and ChatGPT below retain their previously verified registry identities. The
private preset generator remains bundled from reviewed source.

| Package | Role | Source commit | Source tree | Record |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.2` | Bundled machine API source candidate | [18079d8b8c75](https://github.com/ckanner/dsh-subagent-codex-pro/commit/18079d8b8c751f5b50375473a96960eeb68b61be) | `b7c2e312c66dffb9b4aff6ce8c1b523cf5a3e7fe` | [codex-pro.json](codex-pro.json) |
| `dsh-preset-generator@0.4.0` | Bundled private PTC composition | [3dc21791b1ef](https://github.com/ckanner/dsh-preset-generator/commit/3dc21791b1efe81bea9a26aa9e92ff2172b4f645) | `877ea728cad4d8ef4b21a691df2a03cfc9075efc` | [preset-generator.json](preset-generator.json) |
| `dsh-subagent-claude-code-pro@0.7.0` | Optional Agent SDK subscription machine candidate | [ae3af4d752e4](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/ae3af4d752e4a974d41c848301472f4bb5d46d0f) | `501e99df8e626ef71568faefbafc974803e401bf` | [claude-code-pro.json](claude-code-pro.json) |
| `@ckanner/dsh-subagent-antigravity@0.2.0` | Bundled official agy CLI, Orchestrator/subagents/Swarm and standalone DSH plugin | [9803af1977f8](https://github.com/ckanner/dsh-subagent-antigravity/commit/9803af1977f8868d6687653b7c0d8bbc887e5786) | `9d760ed5724dc6eb509e4f1c31795cde7a3ab43b` | [antigravity.json](antigravity.json) |
| `dsh-plugin-lcu@0.5.0` | Optional managed desktop peer | [505e516cbc90](https://github.com/ckanner/dsh-plugin-lcu/commit/505e516cbc909556a7b9855a454202cc28b583a8) | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | [lcu.json](lcu.json) |
| `dsh-plugin-chatgpt@0.1.20` | DSH provider compatibility fixture | [40d8f885e7a3](https://github.com/ckanner/dsh-plugin-chatgpt/commit/40d8f885e7a386af5f441b356b9c68496a47b0e3) | `c8c4f4829a00b1917fd1aef8c4254e083a5b6180` | [chatgpt.json](chatgpt.json) |

Published-package records include the registry URL, exact integrity, SHA-256 and
verification timestamp. Their verification toolchain does not claim how the
publisher built the archive. Source candidates instead set
`registryReleaseVerified:false`, retain their source-build toolchain and have no
registry-release record. The current Claude and Antigravity candidates were built and installed with
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

The current 0.2.0 candidate was reviewed after adding fixed-effort Orchestrator,
explicit collaboration, bounded child observations and the DSH resume fix. The
same archive bytes passed external installation/ESM/types/bin consumption and a
real disposable DSH profile install. [Current follow-up validation](../../../docs/validation/2026-11-20-orchestration-adaptive-selection.md)
records the scope and limits; neither those tests nor this source identity prove
real-account model inference or a registry release.

Earlier published versions and reviewed candidates remain identified in dated
validation records. In particular, the former Codex 0.10.0 and Claude 0.5.1
archives are not relabeled as the new activity/naming candidates. ChatGPT 0.1.20
remains the reviewed provider fixture. See the
[provider validation record](../../../docs/validation/2026-11-16-provider-liveness-and-titles.md)
for the initial implementation. The subsequent [review closure](../../../docs/validation/2026-11-16-provider-liveness-review-closure.md) records the earlier archives, additional fixes, exact-candidate acceptance and its limits. The [Codex follow-up](../../../docs/validation/2026-11-16-codex-review-fixes.md) records the preceding child-creation and required-naming fixes, regression results and exact-candidate acceptance. The [native history review](../../../docs/validation/2026-11-17-native-session-visibility.md) records the current retention and discovery fixes. Current source identities and archive hashes are in the table and records above.

Codex Pro, Antigravity and the generator are exact bundled runtime dependencies. Both source
locks resolve the corresponding vendor tarballs with actual integrity. Claude
and LCU remain optional peers; only the private test harness installs them by
default. PR CI verifies the checked-in Claude candidate's package name/version,
SHA-256 and integrity, and supplies those same bytes to both OS jobs. Actual
Flywheel publication and promotion must first obtain the optional Claude
0.7.0 companion from the public registry and verify that it matches the reviewed
bytes. Codex and Antigravity are distributed inside the complete Flywheel package; their
bundled bytes are checked without claiming separate registry releases. The Antigravity
tarball is also shipped for deliberate standalone DSH installation. Source-candidate CI
does not satisfy that publication prerequisite. ChatGPT is a provider fixture,
not an additional implicit model backend.

The [Agent SDK migration](../../../docs/validation/2026-11-17-claude-agent-sdk.md)
records Claude 0.6.0's exact source/SDK identities, native history acceptance,
authentication boundaries and SDK-tail review corrections.

The machine entry points remain independent of optional desktop peers. Desktop
native settings belong to the desktop plugin; direct Flywheel workers use their
own frozen native profile. Installing a package in the desktop profile does not
make it available in the global worker resolution tree or upgrade the SDK profile.
See [native setup](https://github.com/ckanner/flywheel/blob/main/docs/en/native-capabilities.md) and
the [desktop validation record](../../../docs/validation/2026-11-14-desktop-native-configuration.md).

No native provider executable, credentials, or private DSH application resources
are bundled. Updating these records does not publish Flywheel or alter user
profiles.
