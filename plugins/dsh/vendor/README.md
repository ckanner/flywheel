# Verified companion packages

Flywheel keeps exact reviewed package bytes. Codex Pro 0.10.2 and Claude Code
Pro 0.6.0 are reviewed source candidates from [Codex PR #9](https://github.com/ckanner/dsh-subagent-codex-pro/pull/9)
and [Claude PR #10](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/10).
They are not claimed to be published registry releases. LCU and ChatGPT below
have been verified against the public npm registry. The private preset generator
is intentionally bundled from reviewed source; it has no public npm release.
Version, source tree, archive and installed content remain separate identities.

| Package | Role | Source commit | Source tree | Record |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.2` | Bundled machine API source candidate | [18079d8b8c75](https://github.com/ckanner/dsh-subagent-codex-pro/commit/18079d8b8c751f5b50375473a96960eeb68b61be) | `b7c2e312c66dffb9b4aff6ce8c1b523cf5a3e7fe` | [codex-pro.json](codex-pro.json) |
| `dsh-preset-generator@0.4.0` | Bundled private PTC composition | [3dc21791b1ef](https://github.com/ckanner/dsh-preset-generator/commit/3dc21791b1efe81bea9a26aa9e92ff2172b4f645) | `877ea728cad4d8ef4b21a691df2a03cfc9075efc` | [preset-generator.json](preset-generator.json) |
| `dsh-subagent-claude-code-pro@0.6.0` | Optional official Agent SDK machine/native-jobs candidate | [25754c3b4144](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/25754c3b41446c1f1225e7d7c6c12905aa67e33a) | `7ffd8deef680661c5b36890b459ce61ad468deea` | [claude-code-pro.json](claude-code-pro.json) |
| `dsh-plugin-lcu@0.5.0` | Optional managed desktop peer | [505e516cbc90](https://github.com/ckanner/dsh-plugin-lcu/commit/505e516cbc909556a7b9855a454202cc28b583a8) | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | [lcu.json](lcu.json) |
| `dsh-plugin-chatgpt@0.1.20` | DSH provider compatibility fixture | [40d8f885e7a3](https://github.com/ckanner/dsh-plugin-chatgpt/commit/40d8f885e7a386af5f441b356b9c68496a47b0e3) | `c8c4f4829a00b1917fd1aef8c4254e083a5b6180` | [chatgpt.json](chatgpt.json) |

Published-package records include the registry URL, exact integrity, SHA-256 and
verification timestamp. Their verification toolchain does not claim how the
publisher built the archive. Source candidates instead set
`registryReleaseVerified:false`, retain their source-build toolchain and have no
registry-release record. The new Codex and Claude candidates were built and
installed with Node 24.22.1 / npm 11.9.0; their exact hashes and integrity are in
the records above. A test-only follow-up can have a different source tree while
preserving the packaged bytes; such a check does not rewrite the archive's
original source identity.

Earlier published versions and reviewed candidates remain identified in dated
validation records. In particular, the former Codex 0.10.0 and Claude 0.5.1
archives are not relabeled as the new activity/naming candidates. ChatGPT 0.1.20
remains the reviewed provider fixture. See the
[provider validation record](../../../docs/validation/2026-11-16-provider-liveness-and-titles.md)
for the initial implementation. The subsequent [review closure](../../../docs/validation/2026-11-16-provider-liveness-review-closure.md) records the earlier archives, additional fixes, exact-candidate acceptance and its limits. The [Codex follow-up](../../../docs/validation/2026-11-16-codex-review-fixes.md) records the preceding child-creation and required-naming fixes, regression results and exact-candidate acceptance. The [native history review](../../../docs/validation/2026-11-17-native-session-visibility.md) records the current retention and discovery fixes. Current source identities and archive hashes are in the table and records above.

Codex Pro and the generator are exact bundled runtime dependencies. Both source
locks resolve the corresponding vendor tarballs with actual integrity. Claude
and LCU remain optional peers; only the private test harness installs them by
default. PR CI verifies the checked-in Claude candidate's package name/version,
SHA-256 and integrity, and supplies those same bytes to both OS jobs. Actual
Flywheel publication and promotion must first obtain the optional Claude
0.6.0 companion from the public registry and verify that it matches the reviewed
bytes. Codex is distributed inside the complete Flywheel package; its exact
bundled bytes are checked without claiming a separate registry release. Source-candidate CI
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
