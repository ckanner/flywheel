# Verified companion packages

Flywheel 0.34.0 keeps exact reviewed package bytes. Codex Pro, Claude Code Pro,
LCU and ChatGPT below have been verified against the public npm registry.
The private preset generator is intentionally bundled from its reviewed source;
it has no public npm release. Version, source tree, archive and installed content
remain separate identities.

| Package | Role | Source commit | Source tree | Record |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.0` | Bundled machine API | [29b388a28674](https://github.com/ckanner/dsh-subagent-codex-pro/commit/29b388a286748740b8bbbcbcdf0e5bf02038befa) | `47ab29706d2f8346dc600cf96144c7b43263091c` | [codex-pro.json](codex-pro.json) |
| `dsh-preset-generator@0.4.0` | Bundled private PTC composition | [3dc21791b1ef](https://github.com/ckanner/dsh-preset-generator/commit/3dc21791b1efe81bea9a26aa9e92ff2172b4f645) | `877ea728cad4d8ef4b21a691df2a03cfc9075efc` | [preset-generator.json](preset-generator.json) |
| `dsh-subagent-claude-code-pro@0.5.0` | Optional machine/native-jobs peer | [e559153123f5](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/e559153123f52a12c35b758633290cf6ebd4cf4b) | `49ab2543721295bf1c27bb4504a01b41c9acf9fc` | [claude-code-pro.json](claude-code-pro.json) |
| `dsh-plugin-lcu@0.5.0` | Optional managed desktop peer | [505e516cbc90](https://github.com/ckanner/dsh-plugin-lcu/commit/505e516cbc909556a7b9855a454202cc28b583a8) | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | [lcu.json](lcu.json) |
| `dsh-plugin-chatgpt@0.1.20` | DSH provider compatibility fixture | [40d8f885e7a3](https://github.com/ckanner/dsh-plugin-chatgpt/commit/40d8f885e7a386af5f441b356b9c68496a47b0e3) | `c8c4f4829a00b1917fd1aef8c4254e083a5b6180` | [chatgpt.json](chatgpt.json) |

Published-package records include the registry URL, exact integrity, SHA-256 and
verification timestamp. Their verification toolchain does not claim how the
publisher built the archive. The generator retains its original source-build
toolchain and `registryReleaseVerified:false`.

The published Codex and LCU tarballs match their earlier reviewed candidates
byte-for-byte. Claude 0.5.0 has identical file contents but different archive file
modes; the current tarball, lock and CI hash are the actual published bytes. npm
consumer installation normalizes its machine module to readable mode 0644.
ChatGPT 0.1.20 carries the reviewed image/tool-output changes; its earlier 0.1.19
source candidate differed only in package version, and is no longer the fixture.
Historical candidate identities remain in the dated validation records.

Codex Pro and the generator are exact bundled runtime dependencies. Both source
locks resolve the corresponding vendor tarballs with actual integrity. Claude
and LCU remain optional peers; only the private test harness installs them by
default. CI fetches the exact published Claude 0.5.0 tarball, verifies its SHA-256,
and supplies those same bytes to both OS jobs. ChatGPT is a provider fixture,
not an additional implicit model backend.

The machine entry points remain independent of optional desktop peers. Desktop
native settings belong to the desktop plugin; direct Flywheel workers use their
own frozen native profile. Installing a package in the desktop profile does not
make it available in the global worker resolution tree or upgrade the SDK profile.
See [native setup](https://github.com/ckanner/flywheel/blob/main/docs/en/native-capabilities.md) and
the [desktop validation record](../../../docs/validation/2026-11-14-desktop-native-configuration.md).

No native provider executable, credentials, or private DSH application resources
are bundled. Updating these records does not publish Flywheel or alter user
profiles.
