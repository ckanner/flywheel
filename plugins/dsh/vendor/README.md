# Verified companion packages

Flywheel keeps exact reviewed package bytes. Codex Pro, LCU and ChatGPT below
have been verified against the public npm registry. Claude Code Pro 0.5.1 is the
reviewed, unpublished source candidate from [PR #4](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/4).
The private preset generator is intentionally bundled from its reviewed source;
it has no public npm release. Version, source tree, archive and installed content
remain separate identities.

| Package | Role | Source commit | Source tree | Record |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.0` | Bundled machine API | [29b388a28674](https://github.com/ckanner/dsh-subagent-codex-pro/commit/29b388a286748740b8bbbcbcdf0e5bf02038befa) | `47ab29706d2f8346dc600cf96144c7b43263091c` | [codex-pro.json](codex-pro.json) |
| `dsh-preset-generator@0.4.0` | Bundled private PTC composition | [3dc21791b1ef](https://github.com/ckanner/dsh-preset-generator/commit/3dc21791b1efe81bea9a26aa9e92ff2172b4f645) | `877ea728cad4d8ef4b21a691df2a03cfc9075efc` | [preset-generator.json](preset-generator.json) |
| `dsh-subagent-claude-code-pro@0.5.1` | Optional machine/native-jobs source candidate | [412904d31208](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/412904d3120895c30c2d75fd47216e2d663a64c6) | `58758c87df696f3b8c33cfdaa41f4be35a429bc2` | [claude-code-pro.json](claude-code-pro.json) |
| `dsh-plugin-lcu@0.5.0` | Optional managed desktop peer | [505e516cbc90](https://github.com/ckanner/dsh-plugin-lcu/commit/505e516cbc909556a7b9855a454202cc28b583a8) | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | [lcu.json](lcu.json) |
| `dsh-plugin-chatgpt@0.1.20` | DSH provider compatibility fixture | [40d8f885e7a3](https://github.com/ckanner/dsh-plugin-chatgpt/commit/40d8f885e7a386af5f441b356b9c68496a47b0e3) | `c8c4f4829a00b1917fd1aef8c4254e083a5b6180` | [chatgpt.json](chatgpt.json) |

Published-package records include the registry URL, exact integrity, SHA-256 and
verification timestamp. Their verification toolchain does not claim how the
publisher built the archive. The generator retains its original source-build
toolchain and `registryReleaseVerified:false`. The Claude 0.5.1 candidate also
sets `registryReleaseVerified:false`, with no registry-release record. Its
Node 24.22.1 / npm 11.9.0 source pack was reproduced byte-for-byte with npm
12.2.1; both yield SHA-256 `5e12a708078729a85449ea72094bd7c50e57604fa5035c3aa0384c42ccfc884c`.

The published Codex and LCU tarballs match their earlier reviewed candidates
byte-for-byte. The previous Claude 0.5.0 registry artifact remains identified in
its dated validation record; it is not relabeled as the 0.5.1 source candidate.
ChatGPT 0.1.20 carries the reviewed image/tool-output changes; its earlier 0.1.19
source candidate differed only in package version, and is no longer the fixture.
Historical candidate identities remain in the dated validation records.

Codex Pro and the generator are exact bundled runtime dependencies. Both source
locks resolve the corresponding vendor tarballs with actual integrity. Claude
and LCU remain optional peers; only the private test harness installs them by
default. PR CI verifies the checked-in Claude candidate's package name/version,
SHA-256 and integrity, and supplies those same bytes to both OS jobs. Actual
Flywheel publication and promotion must first obtain Claude 0.5.1 from the public
registry and verify that it matches these reviewed bytes. Source-candidate CI
does not satisfy that publication prerequisite. ChatGPT is a provider fixture,
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
