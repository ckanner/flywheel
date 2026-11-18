# ChatGPT Web Pro: implementation candidate and review evidence

English · [简体中文](2026-11-17-chatgpt-web-implementation.md) · [Design](../plans/2026-11-17-chatgpt-web-provider.en.md) · [Validation index](README.md)

**Status: implementation candidate, not merged or released; live website and macOS acceptance remain incomplete.** This record covers revision 2 of PR #57 and its companion plugin implementation. Implemented mechanisms, offline checks, and outstanding capabilities are separate. This is not a claim that T01–T36 all passed.

## 1. Components and source identities

| Component | Delivery | Baseline and version |
| --- | --- | --- |
| ChatGPT DSH plugin | Native Web model, browser transport, tool protocol, sessions, recovery, provider card | `ckanner/dsh-plugin-chatgpt`, based on `40d8f885e7a386af5f441b356b9c68496a47b0e3`; candidate `0.2.0-alpha.1` |
| DSH host | Public replay veto, compaction policy, durable ACK, separate main/summary adoption | Official `ca33fd6843b205e279c7ccb6cb73a472c78f5760`, `0.2.2-rc.4`, plus an explicitly maintained compatibility patch |
| Flywheel | Exact DSH effort, SDK admission, stable ownership, activity, budgets, ledger and configuration UI | Based on `db6b93e6b2f74b8bcc6563c54bcafbd0813ccd0c`; implementation added to PR #57 |
| Oracle | Behavioral and test-design reference | Fixed `0a064e7b604df933f84e64f0b5a69f1b178dee78`; no runtime, private HTTP, or source-snapshot dependency |

Plugin sources and guides are on [feat/chatgpt-web-pro](https://github.com/ckanner/dsh-plugin-chatgpt/tree/feat/chatgpt-web-pro). Its [compatibility manifest](https://github.com/ckanner/dsh-plugin-chatgpt/blob/feat/chatgpt-web-pro/compat/manifest.json) pins the patch hash, upstream commit, patched tree, and verification scope. This is not a published official SDK. Installing the plugin does not patch an existing SDK or Desktop application.

The initial implementation snapshot used a 334,255-byte compatibility patch with SHA-256 `9718aaf2b8087dc953507347ef018f8fe6a8bbb181c7fa60aaadf32517c978b9`; its patched tree was `333e1019f2333068ba4f6157705bf2d6d4b676cf`. An isolated Git index verified both apply-check and actual application against the exact base. These historical identities do not include the later `summaryRoute: 2` follow-up in §8; use the companion plugin's compatibility manifest for its current patch identity.

## 2. Model behavior

The existing `chatgpt` provider adds exact model `gpt-6-pro-web`, display name `GPT-6 Pro · Web`, and native effort `pro`. Custom provider routes remain supported. Exact Web IDs select the browser path; SIWC retains its existing authentication, conversion, images, context and retry behavior.

Web is disabled by default. Model discovery, settings reads and static diagnostics do not launch Chrome or send a prompt. The existing provider card configures a dedicated browser and manual account binding. Each actual call checks GPT-6, Pro, account, project and conversation identity again. An explicit profile owns its settings; Connect does not overwrite them.

DSH retains tool permissions, execution, working directories and authoritative history. The plugin exposes tool blocks only after complete JSON, identity, registry, schema, size, depth and call-count validation. The following prompt explicitly maps short provider call IDs to durable DSH tool IDs.

Matching history prefixes append only real deltas. Edits, compaction, tool/instruction binding changes and local history/turn limits create a new generation with authoritative bootstrap context. Main calls and auxiliary summaries/titles own separate chats. Readable titles are derived locally; conversations and reviewable links remain available.

A single DSH CompactionEngine owns automatic and manual compression. Public pressure policy resolves the mismatch between default headroom and the conservative Web budget. Summary routing happens before call construction and records the actually resolved model and Pro effort. There is no second Go summarizer.

## 3. Recovery and concurrency invariants

| Boundary | Behavior |
| --- | --- |
| Before sending | Persist complete preparation before its host index and conversation reservation, then a distinct dispatch intent. Project creation and prompt submission have separate WAL intents. |
| Delivery | Persist the complete artifact, digest and validated blocks, then end the stream. The host commits and flushes before acknowledging through a separate callback. |
| Capacity | One active turn per actual Chrome profile. Healthy siblings queue with cancellation; capture retains capacity until durable host ACK. The stream never waits for its own ACK. |
| Ambiguity | Dispatch, outcome, remote generation, local cleanup and usage remain separate. Fresh sessions, provider changes, new chats and always retry cannot replay unknown work. |
| Live ownership | Healthy in-process work may queue. An idle event synchronously revokes finished calls' live exceptions. Restarted processes cannot infer live execution from persistent intent alone. |
| Domain inheritance | Main/auxiliary calls, non-Web subagents and nested tools share ownership. Physical workspace aliases persist external domain associations across fresh sessions. |
| Managed identity | Completing a main response does not replace Flywheel's invocation ID, budget or isolation. The entire invocation freezes its connection, account and policy. |
| Cleanup | Closing CDP, or proving the original process exited, can establish local cleanup. Clicking Stop does not prove cloud termination. |
| Adoption | Main and summary use different maintenance transactions. Real JSONL history, original messages/surface/header/route/tools and durable identity are checked. Repeated adoption does not repeat tools or replacement. |
| Usage | Missing website token/cost counters remain unknown/null. No zeroes, estimates or hidden budget resets are reported as authoritative usage. |

Final main success, Agent idle, and title delivery are separate facts. An empty invocation reports `not-dispatched / failed / idle`. A captured response cannot settle another unknown local tool result in the same domain.

## 4. Review findings fixed during implementation

Independent reviewers and minimal production-class reproductions found and fixed the following; these are not outstanding items:

- Nested Web schema defaults synthesized a partial configuration and broke SIWC discovery. Actual default `ctx.plugin` composition now passes.
- Default DSH headroom exceeded the Web local window, preventing compression. Public model pressure policy now covers the default engine configuration.
- Pro defaults were resolved after pre-projection. The actual admitted effort is now materialized while original media/history validation remains intact.
- Auxiliary calls escaped the original managed ID, domain, isolation and wall after main completion. Managed and native invocation rotation are now distinct.
- Connection/account/policy could drift inside an outer invocation, and managed Connect tried an unwritable save. Invocation binding is frozen and verification is separate from saving.
- Healthy siblings were treated as recovery faults; capture released capacity too early; failed ACK waited for idle before stopping. Queueing, ACK capacity and immediate domain cancellation now cover these cases.
- Non-Web children missed root ownership, nested unknown results were overwritten, and cancellation missed canonical aliases. Shared ownership and durable tool facts now apply.
- Preparation could become undiscoverable; crashes between capture/delivery or capture/cleanup lost recoverability; lock reaping had ABA and Chrome startup races. Stage evidence and real process-crash regressions cover them.
- JSON record limits conflicted with admitted response/history limits. Original input, response and recoverable-record budgets are checked before sending.
- Summary adoption must not recognize an old checkpoint merely because text matches. Original request/response identity and opaque replay state are required.

## 5. Verification

| Check | Result and evidence scope |
| --- | --- |
| Flywheel `go build ./...` | Passed |
| Flywheel `make quality-gate` | Passed; total and every-function coverage 100% |
| Flywheel DSH Node check | Main 553/553, legacy 10/10, zero skipped |
| Go → Node worker → published SDK | Existing providers and Web consumption contract passed; Web/browser peers are explicitly offline fixtures |
| Plugin TypeScript and Node tests | 319 tests: 318 passed, one skipped, zero failed; the skip is the constrained real Chromium fixture |
| Plugin build and installation | Build, ten client Remote methods, 182-file pack and bilingual checks passed. The exact archive installs/imports with real Cordis 4.1.0 / Schemastery 3.19.0 / DSH rc.3 peers, proving legacy SIWC peer loadability, not Web support in stock rc.3 |
| Compatible DSH host | Initial 19-file / 512-case suite passed; final summary-identity fix passed 18 focused cases, including eight new rejections, and six cross-layer cases. Affected source/test TypeScript, lint, public API and bilingual/generated checks passed |
| Actual host integration | Six cases passed; only the browser page is scripted. DSH, plugin, JSONL, tools, adoption and the Flywheel policy module are actual source |
| Full upstream host build | Incomplete in this runner: tsx IPC denied and aggregate TypeScript exceeded available memory. Affected-project checks do not substitute for a release build |
| User macOS / actual ChatGPT account | Not run; not included in passing evidence |

The plugin's `scripts/verify-web-harness.mjs` accepts a DSH checkout and optional Flywheel checkout. Cases cover default composition; main→durable ACK→tool→final; capture/disconnect→adoption→exactly-once tool; default automatic/manual compaction; always-policy veto; and actual `createWebPolicy` admission plus the durable terminal receipt after disposal.

## 6. Original 2026-11-17 acceptance snapshot

This table preserves the first implementation snapshot. The dated continuation
below records subsequent source changes; the original gaps are not current
claims that image upload and format correction remain unimplemented.

| Outstanding area | Current behavior / required evidence |
| --- | --- |
| Live website/macOS parts of T01/T05/T25/T28/T36 | DOM fixtures, selection and naming are implemented. Actual account, current layout, Pro in Projects and revisitable history still require live acceptance. Chromium socket creation returned `EPERM` here. |
| Images, screenshots and LCU vision | Original media is rejected before projection; upload support is not declared. Authorized local text tools remain usable. |
| T17 website full-context rejection | Local byte-pressure migration is implemented. A refusal without reliable no-generation/no-side-effect proof remains fenced. |
| Bounded format correction and retire/discard | Not implemented. Invalid complete responses retain artifact and capacity; observation cannot make invalid JSON adoptable. |
| T26 ambiguous project creation | Creation intent remains recoverable evidence, but reliable automatic read-only project discovery is unavailable. Repeated replacement creation is blocked. |
| T30 heavy presets, real PTC/Team/LCU and hosted apps | Native tools, child domains and nested outcomes have contract regressions; this is not live acceptance of every tool or external application. |
| Deployment and release | Exact compatibility source and candidate build paths are delivered. No patched SDK, Desktop binary or official npm release was published. |

Both PRs remain Draft. Complete the missing evidence in the real environment and update the design matrix item by item; offline green checks alone do not establish delivery of the complete capability.

## 7. 2026-11-18 continuation: images, format correction and Flywheel reconciliation

The companion ChatGPT plugin now implements bounded durable image upload with
original user/tool occurrence metadata, byte and digest verification, accepted
upload names, and private recovery artifacts. Format correction is a bounded
continuation of the captured answer, with immutable parent/child identities;
it cannot alter tool arguments or task meaning. Explicit discard binds the
inspected rejected-response digest and records a failed retirement separately
from a host commit. These source paths have offline regression coverage.
Upload acceptance remains `modelConsumption: unverified`; live model attention,
account-specific limits and actual LCU vision are separate acceptance evidence.

Flywheel now records `chatgpt-web/read-only-recovery/1` checkpoints before the
prompt and its original SDK prompt/wait/terminal/shutdown proof. Reconciliation
reads original plugin and worker artifacts after their owners exit, verifies
the original binding/domain/configuration and candidate/role guards, and sends
no new SDK, browser, model or tool work. The outcome matrix is documented in
the [model policy](../en/model-policy.md#reconcile-an-interrupted-web-invocation)
and [worker protocol](../../plugins/dsh/PROTOCOL.md#原调用只读恢复协议).
Pure final adoption after an interrupted SDK, adopted tools or summaries, and
missing original candidate proof continue to block; they cannot become task
completion merely because a response was captured. Explicit retirement can
settle a known failure only with the required original shutdown and guard proof.

The Flywheel source change passed `make quality-gate` with every measured Go
function at 100% coverage, the DSH Node check (including legacy-host loading),
and the enabled cross-process worker bridge tests. New tests cover the original
successful terminal, known unsent and failed outcomes, retired responses,
unresolved adoption, changed identities and guards, immutable response/repair
artifacts, and image-root acceptance even when the final is a correction tail.
The corresponding plugin helper tests run a real host and transport in an
exited child process with only the browser page replaced.

Follow-up review closed three concrete integration gaps: the original candidate
receipt now has its own state-CAS-confirmed digest before the main receipt is
replaced; failed retirement retains the SDK's actual observed turn ID; and the
Go query closes stdin and forwards the original noncredential SDK selection.
Tests cover all three checkpoint/CAS interruption windows and the real Go parser
consuming Node recovery results for completed, retired and unsent invocations.
The final Go gate reports 100% for every measured function. The source DSH suite
passed 560 tests plus ten legacy-host cases before the subsequent packaging
change; its independent reader and recovery checks then passed nine targeted
tests, and release validation passed 45 tests.

The packaging follow-up builds the candidate's public read-only reader into an
independent Flywheel module, including its schema validators. Its checked
external imports are limited to Node filesystem, crypto, path and OS helpers.
The exact source/input/output manifest and all original licenses ship with the
module. A clean consumer no longer depends on finding the provider in another
SDK profile. The final root release gates must run against the stable source:

```sh
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm \
  FLYWHEEL_CLAUDE_CODE_PRO_TARBALL="$PWD/plugins/dsh/vendor/dsh-subagent-claude-code-pro-0.6.0.tgz" \
  npm run test:install
```

Their final candidate identity, packed-install recovery results and CI evidence
are recorded in the PR after execution; this source record does not predeclare
those release-gate results or require repacking merely to append them here.

The ChatGPT source candidate recorded for the preceding packaging check was
[`191a2cc11ded693970e0c77a8fd351dfa3aec9cf`](https://github.com/ckanner/dsh-plugin-chatgpt/commit/191a2cc11ded693970e0c77a8fd351dfa3aec9cf),
tree `eff36fdf3756ac95856420e518abf50328f9aefc`. Its 199-file, 735161-byte
`dsh-plugin-chatgpt-0.2.0-alpha.1.tgz` has SHA-256
`e53a5ee4d8c6d5300f896ca680f362a453fcf1385e5eec227eeaa29b395b59d7`;
that archive identity is retained here as historical evidence. The currently
selected candidate, full integrity and build toolchain are in
[vendor/chatgpt.json](../../plugins/dsh/vendor/chatgpt.json).
[CI run 44245907991](https://github.com/ckanner/dsh-plugin-chatgpt/actions/runs/44245907991)
passed Node 22/24 and both Ubuntu/macOS exact-host jobs. Each OS exercised the
391 host tests, eight real cross-layer cases and six real Chromium fixture
cases, including media. The macOS profile-cleanup correction changed test
files only; repacking produced identical candidate bytes.

These are source, browser-fixture and installed-peer checks; they do not publish
a patched SDK or npm release, install the provider in a user's SDK profile, or
attest a real ChatGPT account. Real account/current-page acceptance, a complete
heavy Preset task, and ambiguous project-creation discovery remain separate gates.

## 8. 2026-11-18 follow-up review: browser ownership and compaction contracts

This follow-up reviews the companion plugin and host contract consumed by
Flywheel. It preserves the earlier snapshots and their test results; those
results do not certify the updated candidate.

| Finding | Required behavior in the updated companion candidate |
| --- | --- |
| A newly prepared composer could navigate to another chat before upload or submission | Revalidate the admitted new-chat URL at the mutation boundary. A different conversation or new-chat page cannot receive the original request's images or prompt. |
| Recovery could treat a stopped page with an error banner as a complete answer | Apply the same generation-error checks during recovery as during normal polling. Visible partial text does not become an adoptable complete response merely because generation is no longer running. |
| Web compaction policy could override an explicit different summary provider/model | Pass explicit `configuredTarget` separately from `proposedTarget` and the historical default; explicit selection wins. Only a selected Web summary receives the Web Pro policy. Other explicit routes retain their own effort and accounting. |
| Manual compaction lost the actual initiating Agent; with separate plugin state and workspace directories, an interrupted captured summary lacked recovery metadata and fenced the state directory, allowing a fresh non-Web Agent in the real workspace to proceed | Keep the actual Agent's context throughout the host maintenance operation. Bind summary recovery and the execution-domain fence to its real session/workspace, preserve that fence across fresh and non-Web Agents, and adopt the original summary only through the explicit idempotent host transaction. The plugin must not infer an Agent from a session ID. |

The third fix requires `summaryRoute: 2` in the compatible DSH host and plugin
admission. `replayVeto`, `durableResponseAck`, `durableSummaryAck`, and
`pressurePolicy` remain version 1. Web extension v1, `chatgptWebHost` v1,
binding `hostContractVersion: 1`, and `chatgpt-web/read-only-recovery/1` are
unchanged. The older summary-route capability is rejected before dispatch;
installing updated plugin bytes alone does not update the host.

Before the companion changes, this review rebuilt Flywheel commit
`0804c467c7f29f436e01bba27398ade0e15cfa56` and passed all 18 focused Node Web
policy/recovery/bundle tests, with zero skips. The Web Go tests in
`internal/drivers` and `internal/machine` also passed with
`FLYWHEEL_TEST_NODE_BRIDGE=1`, including the actual Node worker and four recovery
dispositions. No additional Flywheel execution/recovery defect was reproduced
in that review.

The updated companion archive is pinned to plugin source commit
[`783c8ce53287be47840c9b28eb70041584834e74`](https://github.com/ckanner/dsh-plugin-chatgpt/commit/783c8ce53287be47840c9b28eb70041584834e74),
tree `0e02c3aed787b585209587e7e819b924d1f43de3`. The single 203-file,
755337-byte `dsh-plugin-chatgpt-0.2.0-alpha.1.tgz` was built with Node 24.22.1 /
npm 12.2.1 and has SHA-256
`3a1bf051514a4649492e9909a6a1286ed30253c32248956988f8b3f109ecfb60`.
Its full integrity is recorded in [vendor/chatgpt.json](../../plugins/dsh/vendor/chatgpt.json)
and the source lock. The embedded compatible DSH patch remains based on official
commit `ca33fd6843b205e279c7ccb6cb73a472c78f5760` (`0.2.2-rc.4`); its
354986-byte patch has SHA-256
`d6f629762a2a7a7a3792eda1748f3942a222f9ebeeac99c3edec999ecc283fbb`
and produces tree `4086a378e17bc09b7fbfb19999e76be180664d94`.

Plugin [CI run 44293903906](https://github.com/ckanner/dsh-plugin-chatgpt/actions/runs/44293903906)
passed all four jobs on that exact source commit. Each Node 22/24 job reported
400 tests: 399 passed and one skipped. Each Linux/macOS compatible-host job
passed 395 host tests, 14 real cross-layer cases and all seven Chromium fixture
cases with zero browser skips; both jobs verified the exact patched host tree
before and after testing. The single archive passed per-file content verification
and an isolated consumer install with Cordis 4.1.0, Schemastery 3.19.0 and DSH
`0.2.2-rc.3` peers. That install establishes package loadability; stock peer
versions do not establish the required Web host capabilities.

After updating this exact vendor pin, Flywheel source checks passed with Node
24.22.1, npm 12.2.1 and Go 1.27.4:

| Check | Result |
| --- | --- |
| Fresh root and private DSH `npm ci`; exact installed plugin contents | Passed; all 203 installed plugin files match the pinned archive. The rebuilt independent recovery manifest records the same plugin commit, tree and archive SHA-256. |
| Root `npm test` | 167 passed, zero failed or skipped. |
| `npm --prefix plugins/dsh run check` | Build, all 563 primary tests and all ten legacy-host checks passed, zero skipped. |
| Go to Node bridge matrix below | Eleven top-level tests and 118 passing records including nested cases, zero failed or skipped. |

The Go result combines the documented ten-test selection with the additional
Kimi identity test selected by CI, without repeating the first selection:

```sh
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test((DSH|CodexPro|ClaudeCodePro|KimiNative)NodeWorkerOfflineBridge|(DSH|CodexPro|ClaudeCodePro)ProviderActivityOfflineBridge|KimiPrintSessionTitleOfflineBridge|DSHWebNodeWorkerOfflineBridge|DSHWebRecoveryNodeBridge)$' -count=1 -v
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^TestKimiPrintIdentityFailureIsNotRetryableOfflineBridge$' -count=1 -v
```

`TestDSHWebRecoveryNodeBridge` passed `completed`, `retired`, `unsent` and
`browser-unsent`, retaining SDK dispatch separately from browser dispatch and
proving recovery made no further model request. `TestDSHWebNodeWorkerOfflineBridge`
executed both cold and prewarmed actual Node worker calls, preserving invocation
and execution-domain identity and unknown usage/cost.

The final Flywheel candidate and exact installation results belong in the PR
after building the clean pushed source, without repacking to append evidence.
Real macOS Desktop, the current ChatGPT website and logged-in account acceptance
remain outstanding.

## 9. 2026-11-18 follow-up: submission identity, current workspaces and recovery directories

Earlier sections retain their original source identities and results. This review
reproduced and fixed three issues:

| Finding | Reproduction and correction |
| --- | --- |
| P1: submission identity changed at the same URL | After preparation, a different account, model, Pro effort, active generation or hosted control could still receive a text submission; images could upload before the account mismatch was detected. The plugin rechecks before any upload or text entry and again after uploading. It neither silently restores a selection nor resends. Production-page DOM negative controls detect the old behavior. |
| P1: current worktrees were missing from execution domains | The integration read the initial root header, and roots or already inherited children skipped later directory registration. Actual DSH directory changes, separate child worktrees and non-Web tools reproduced the gap. Ownership now uses the current Agent's public directory service while preserving its parent domain and earlier directories. Fresh non-Web calls remain fenced after restart. |
| P2: recovery workers started in a different directory | Normal Web, Codex Pro and Claude Code Pro dispatch used the original cwd, but recovery used the engine's launch directory. Six actual subprocess cases with relative workers or cwd-dependent absolute wrappers failed before the fix. Both read-only recovery entries now retain the original physical cwd; all six cases pass. |

Stable workspace bindings retain their early return so the integration can settle
durable tool results before its normal guard. An independent negative/positive
control observed zero flushes and dispatches with the wrong ordering, and one of
each after correction; the regression also requires the guard to reject before
settlement. Older hosts without the directory service retain relative-path
fallback. A present service's failure cannot select the stale header. Protocols,
host capability versions, environment filtering, original candidate/role checks
and the unknown-outcome replay veto are unchanged.

### Exact companion candidate and local checks

Plugin commit: `5d2325abf0da9d7897cd273ed30d463dba0cc204`; tree:
`b780ceb8cf4501e0aed47270cba140b6e29624cb`. The single packed
`0.2.0-alpha.1` candidate contains **203 files / 756836 bytes**, SHA-256
`f49fb116f45d3be2c4e8749b53fa246a4bb94c25f6bfbdba0006e5e28b556f03`.
Vendor metadata, archive, lock and reader source agree; all 203 installed files
match byte-for-byte. The DSH patch retains the exact identity from section 8.

With Node 24.22.1, npm 12.2.1 and Go 1.27.4:

| Check | Evidence |
| --- | --- |
| Complete plugin suite | 410 tests: 409 pass, one independent Chrome-fixture skip, no failure. |
| Plugin build and install | TypeScript, build, 11 Remote methods and bilingual README passed; exact-archive contents and isolated installation with actual peers passed. |
| Actual patched DSH and Flywheel policy | 17 cross-layer scenarios passed, including current root/child directories; only the page is scripted. |
| Flywheel DSH | After the exact vendor update, build, 563/563 main tests and 10/10 legacy-host tests passed with no skips. |
| Flywheel Go | Final quality gate passed vet, formatting, all tests and 100% statements in every measured function; all six new recovery-directory cases passed. |
| Root npm | Final full serial run passed 167/167 with no skips. An initial concurrent run failed one timeout cleanup-log assertion and is not counted as passing. The original case passed alone. Delaying doctor startup reproduced the missing log while its process group had exited; that assertion alone does not establish a cleanup leak. |

Local real Chromium exited before exercising the fixture because the runner
denied Unix socket/ptrace operations. This attempt is not passing evidence and
no restriction was bypassed. Current-commit dual-OS CI, the Flywheel DSH gate,
complete candidate and same-archive installation results are recorded in the PR.
Compatible SDK/Desktop artifacts and live-account acceptance remain outstanding;
these development candidates have not been published.
