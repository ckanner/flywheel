# Desktop native configuration follow-up — 2026-11-14

## Scope

This follow-up makes the ordinary DSH plugins consume the capabilities already
implemented by their public machine APIs. It adds the plugin settings surface,
the PTC call surface and actual backend forwarding together.

- Codex Pro: [PR #7](https://github.com/ckanner/dsh-subagent-codex-pro/pull/7).
- Claude Code Pro: [PR #3](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/3).
- Flywheel integration: [PR #44](https://github.com/ckanner/flywheel/pull/44).

The earlier machine API work in Codex #6 and Claude #2 was already merged.
Those machine APIs did not by themselves supply live desktop defaults or a
visible plugin configuration page.

## Behavior

Both plugins export a real runtime Config schema and a web client loaded through
DSH's ModuleLoader with the host's shared React. The plugin-manager form uses
revisioned settings mutations, retains rejected/conflicting drafts and supports
resetting only the user override. Volatile defaults are read when a call starts;
a running call keeps that snapshot.

For ordinary delegation, omitted native uses the current plugin default, an
object replaces that default in full, and null explicitly turns collaboration
off. If both the call and plugin omit native, the CLI keeps its prior behavior.
Unknown or contradictory controls are rejected. The caller's permission policy
and model-selection restrictions still apply; native defaults do not expand
them.

Codex forwards the snapshot through its ordinary tool, Host subagent provider and
supervised app-server path. Claude forwards native Agent definitions, Workflow,
Advisor, concurrency/depth, settings and environment through the ordinary local
CLI path. A new configuration identity prevents automatic reuse of an
incompatible remembered session. Actual native completion and cleanup evidence
remain separate from the root's final text.

Independent review and interactive Team operations have explicit tool entries.
They are not persistent ordinary-delegation defaults. Claude jobs use DSH owner
and tool-call attribution with durable, private ownership records; recovery and
control address the caller's known job identity. Enabling a normal native default
never submits a remote review.

Foreground tool results keep the answer and native receipt in separate fields.
Codex image attachments survive the PTC render projection. Structured answers
are not rewritten by native metadata or image-delivery notes. Background
envelopes are documented by each plugin.

## Actual DSH compatibility

Contracts were checked against upstream
[39163dd30f703a79079e2d2446545cff3c453738](https://github.com/deepseek-ai/deepseek-harness/tree/39163dd30f703a79079e2d2446545cff3c453738)
and the published 0.2.2-rc.3 development packages.

A runtime Config export alone does not render a settings form. The plugins
therefore provide the actual plugins.row.config client slot, not just schema
metadata. Client factories use the single require argument and return their
exports, as required by the current ModuleLoader.

The current PTC generator accepts a smaller schema vocabulary than general JSON
Schema. Unsupported anyOf/bounds/dictionary keywords can turn a whole tool's
parameter type into unknown. The plugins use compatible oneOf/object schemas
and retain the full constraints in strict runtime validation. Their regression
suites check real DSH schema validation and PTC SDK generation, not only a fake
tool-registration callback.

Published home-paths and master source are not assumed to have identical exports.
Claude supports the actual released home-paths contract as well as the current
desktop helper. Schema serialization/rehydration, nullable defaults and live
getter behavior are exercised with real Schemastery.

## Reviewed source artifacts

| Package | Final reviewed source | Source tree | Packed SHA-256 | Full npm integrity |
| --- | --- | --- | --- | --- |
| `dsh-subagent-codex-pro@0.10.0` | [284f12c208d0](https://github.com/ckanner/dsh-subagent-codex-pro/commit/284f12c208d03f3f922ccab8eb8002d3bb950796) | `47ab29706d2f8346dc600cf96144c7b43263091c` | `d5437a2a1a125c05f6fe61afcae4c822f2bca94bc11cc5e15c45f9137cc74a61` | [codex-pro.json](../../plugins/dsh/vendor/codex-pro.json) |
| `dsh-subagent-claude-code-pro@0.5.0` | [d14ff8d8334f](https://github.com/ckanner/dsh-subagent-claude-code-pro/commit/d14ff8d8334fefd039c3d48ee2a969747d1e88c8) | `49ab2543721295bf1c27bb4504a01b41c9acf9fc` | `ff2b1198a9358c43add7f89eb08b4e443c78fb0663c6d0a64ab79952aa992272` | [claude-code-pro.json](../../plugins/dsh/vendor/claude-code-pro.json) |

The two vendored tarballs were produced from those immutable commits with
Node 24.22.1 and npm 12.2.1. Preparation run:
[43599091232](https://github.com/ckanner/flywheel/actions/runs/43599091232).

The preparation workflow uses ordinary npm pack, resolves both lockfiles from
the actual new package metadata and proves the restored manifests with npm ci.
The transfer records include byte counts, SHA-256, npm SHA-512 integrity and Git
blob identity. No registry integrity or compiled artifact is invented.

Desktop Schemastery/home-paths dependencies are optional peers. Flywheel's
independent machine imports remain usable without DSH/React. The clean installed
candidate test explicitly checks that those desktop modules cannot be resolved
before exercising the existing real worker/fixture-process flows.

Flywheel's direct native-profile requests keep their own configuration snapshot;
desktop defaults are not merged into a frozen machine request. The public
machine protocol and pinned package versions remain unchanged.

## Validation and limits

| Companion | Final source CI | Unit/process/UI results per OS/Node lane | Additional gates |
| --- | --- | --- | --- |
| Codex Pro | [43598663547](https://github.com/ckanner/dsh-subagent-codex-pro/actions/runs/43598663547), all four lanes successful | 217 passed, 0 failed, 6 live/CLI-environment checks skipped | Typecheck, build, real DSH/PTC contracts, actual installed npm consumer |
| Claude Code Pro | [43599021157](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/43599021157), all four lanes successful | 126 passed, 0 failed, 0 skipped | Published/current-source DSH/PTC contracts and actual installed npm consumer |

Both matrices cover Node 22/24 on Ubuntu/macOS. Live/CLI-environment skips are
not counted as passes. Package consumers reported zero inference requests.

The local executor was unavailable during this follow-up. Implementation and
validation continued through reviewable GitHub commits and GitHub Actions.
No local rerun is claimed.

The integrated CI still requires the Go quality gate, DSH Node and legacy-host
checks, all four Go-to-Node adapter bridges, root npm contracts, actual
same-candidate installation, Linux race checks and real-engine offline session
evaluation on Linux/macOS. Its exact accepted head and completed results are
recorded in the [PR status](https://github.com/ckanner/flywheel/pull/44) rather
than attributing an earlier successful run to this new artifact.

These tests use actual public packages and process boundaries with offline
model/runtime fixtures. They do not claim a logged-in user-account run, a visual
acceptance session in the user's desktop, a paid cloud review or a live
interactive Claude/tmux Team. Claude still reports tools-only effective
verification and observed limits; unknown remote completion/cancellation remains
unknown.

Earlier source candidates and outcomes remain in the
[original implementation record](2026-11-14-native-harness-implementation.md).
This work does not publish packages or alter the user's installed profiles.
