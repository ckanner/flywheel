# Bundled public Codex Pro source candidate

Flywheel imports `dsh-subagent-codex-pro/machine` from the actual public package **0.9.0**. This directory contains that package's reviewed npm tarball, not a replacement wrapper or a fabricated registry download.

- Source: [37da3d75de32b809c4fb1a631ccc15191f3d87f9](https://github.com/ckanner/dsh-subagent-codex-pro/commit/37da3d75de32b809c4fb1a631ccc15191f3d87f9)
- Source tree: `00df1042b0010dc37ce8ab6048f75a434325962c`
- SHA-256: `4a86380fdde0d7f2673c4b1f2d5c8682cf61d6a4b83deb65eab8fe76f6032198`
- npm integrity: `sha512-NtguC96kZVgKOMHv7kSiFVEZ4thjorHS1ZrNFQwbBLEBU2jOSWkFGlXk+BILPuxALUKkJJ4E1DSbA2+JG3gM0Q==`
- Package license: MIT, Copyright (c) 2026 ckanner; the original `LICENSE` is present in the tarball and installed package.

The source candidate adds lifecycle v1, durable acknowledgements, an independent supervisor journal, and read-only recovery. It has not been represented as a published registry version. The runtime dependency remains exactly `0.9.0`; the source lock resolves this real `file:vendor/…tgz` with its actual integrity. `npm ci --ignore-scripts` materializes it, and `bundledDependencies` includes that package in Flywheel's DSH plugin tarball. A user installing the checked Flywheel candidate therefore does not need a nonexistent `0.9.0` registry release. No native Codex executable, login data, or credentials are bundled.

The tarball was built with Node 24.22.1/npm 12.2.1 from the pinned tree, using the upstream lockfile, `npm run build`, and `npm pack --ignore-scripts`. Upstream public API, compiled supervisor, and clean npm-install tests cover these bytes. Flywheel additionally runs its real worker/Go bridge and actual six-package installation checks. The package manager uses the lock integrity; `codex-pro.json` records source provenance separately.

Claude Code Pro **0.4.0** remains an optional peer and is not bundled. Its matching source candidate is [ec0b99176b67512f3ca1838ec2a349dcb0189214](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/ec0b99176b67512f3ca1838ec2a349dcb0189214), with tarball SHA-256 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`. CI builds that immutable source, verifies the exact package hash, and installs it before running Claude-specific gates.

This is source-candidate packaging. It neither publishes the upstream packages nor changes Flywheel's npm `latest` tag.
