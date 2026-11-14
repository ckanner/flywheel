# Reviewed companion source candidates

Flywheel 0.34.0 consumes real public packages from the reviewed trees below.
The source checkout keeps their actual npm tarballs and lockfile integrity;
these are not replacement wrappers or invented registry downloads.

| Package | Distribution role | Fetchable source commit | Source tree | Artifact identity |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.0` | Bundled public machine API | [c0045b06e676](https://github.com/ckanner/dsh-subagent-codex-pro/commit/c0045b06e6762b8dfc56f50b14cce7a53af2725b) | `1c39704cf385e9f505f5d50e9d96ef2e0b88d5f5` | [codex-pro.json](codex-pro.json) |
| `dsh-preset-generator@0.4.0` | Bundled PTC composition API | [0ec05fbeb37d](https://github.com/ckanner/dsh-preset-generator/commit/0ec05fbeb37d59f7e3af4baa0f6d9bfb0c5573a3) | `c93bbc8453a95c2b4f06d432f1c7d0e67a56aaf0` | [preset-generator.json](preset-generator.json) |
| `dsh-subagent-claude-code-pro@0.5.0` | Optional machine/native-jobs peer | [54d7eafa9102](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/54d7eafa9102713597e3b96a6d70a9cbd86afc6b) | `bf992d3e33889857526af370bbb16d91585c0ecc` | [claude-code-pro.json](claude-code-pro.json) |
| `dsh-plugin-lcu@0.5.0` | Optional managed desktop peer | [7986057e9231](https://github.com/ckanner/dsh-plugin-lcu/commit/7986057e92314b78caf66dde43d1a8d45ab6ada7) | `6256c2a947efec8812ae5e4e7e83c4d098ad1ee1` | [lcu.json](lcu.json) |
| `dsh-plugin-chatgpt@0.1.19` | DSH provider compatibility fixture | [fa5cfafcc1cc](https://github.com/ckanner/dsh-plugin-chatgpt/commit/fa5cfafcc1cc6bfb1f640e646b6dc3c4329d976c) | `168dc4e6284f002f09291667a926eab8a31b6687` | [chatgpt.json](chatgpt.json) |

The records include tarball filename, SHA-256, npm integrity, MIT license,
Node 24.22.1/npm 12.2.1 build toolchain and `registryReleaseVerified:false`.
Source trees, remote commit identity, packed bytes, registry release and user
installation are separate evidence. Local merge commit hashes can differ from
remote API-created commits; the tree and artifact are checked explicitly.

`dsh-subagent-codex-pro@0.10.0` and `dsh-preset-generator@0.4.0` remain exact
runtime dependencies and bundledDependencies. Root lock entries resolve
`file:plugins/dsh/vendor/…tgz`; the private build-harness lock resolves
`file:vendor/…tgz`. Both use actual integrity. A consumer of the complete
Flywheel candidate receives these bytes without depending on an unpublished
registry version. Original licenses remain inside both bundled packages.

Claude Code Pro 0.5.0 and LCU 0.5.0 are exact optional peers, not bundled
runtime dependencies. The private build harness installs their reviewed local
packages using normal npm resolution for real public-API tests. CI also fetches
the fixed Claude source commit, packs it with the pinned toolchain, verifies its
SHA-256 and installs the same candidate for Linux/macOS gates. ChatGPT 0.1.19 is
a DSH provider fixture used to verify image/attachment compatibility; it is not
a separate implicit inference backend for Flywheel.

Normal npm resolver, public manifests, compiled APIs, source behavior and clean
consumer installation are checked without `--force` or `--legacy-peer-deps`.
The generator supplies shared composition policy; it does not bundle the DSH
application's private resources. No native provider executable, login data or
credentials are included. Details and actual test outcomes are in the
[implementation validation](https://github.com/ckanner/flywheel/blob/main/docs/validation/2026-11-14-native-harness-implementation.md).

This is source-candidate packaging. It does not publish companion packages or
change Flywheel's npm latest tag.
