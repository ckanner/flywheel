# Official Antigravity ACP interactive integration proposal — CLI first

English · [简体中文](2026-11-22-antigravity-native-acp.md) · [Plans index](README.md) · [Current Antigravity guide](../en/antigravity.md)

- Date: 2026-11-22; revised after the requested comparison of the two official runtimes. Prefer CLI ACP for personal interactive use; retain standalone ACP as a separately qualified option.
- Status: **initial interactive client implemented in the 0.39.0 source candidate**, following the user's subsequent implementation request. The original proposal and source observations remain below; current scope and evidence are recorded in [the implementation record](../validation/2026-11-22-antigravity-acp.md). No registry publication or real-account acceptance is implied.
- Original baseline: Flywheel `291c0a02f9042d522d3e0f3c62977f63149465cd` / 0.38.0; `@ckanner/dsh-subagent-antigravity` 0.2.1, source `a20eb63bbfa2849df63043595475957bf4a4f04d`.
- Revision baseline: main `4d83ad350504411e0d413638bc95a43eeffadb44`. [PR #62](https://github.com/ckanner/flywheel/pull/62) merged as `bf780416136be7bfdd11101f34bbfa358c316394`; its receipt/packaging fixes and validation are separate from this proposal. The original baseline above remains historical evidence.

## Implementation scope after approval

The user subsequently requested implementation and ACP as the default. Companion
0.3.0 now supplies the stable v1 client, authenticated host/UI, official login
helper, runtime provenance checks, typed images, permissions and exact session
records. Flywheel 0.39.0 mounts it directly. The personal entry admits only
Gemini models reported by the native session; Claude and other non-Gemini models
are not implemented choices. No host system-prompt replacement/append or host
tool bridge is injected; official user rules still load normally. New sessions use retained Git worktrees from committed HEAD rather
than sharing a live automatic-task workspace. This implementation choice replaces
the proposed shared-workspace lease for the initial direct path; it does not
claim a global lock for independently launched tools.

The current guide and [validation record](../validation/2026-11-22-antigravity-acp.md)
own shipped-source behavior. The later Registry and machine-binding stages below
remain proposals. Native login/inference, image delivery and detached-task cleanup
have not been established by offline fixtures or unauthenticated probes.

## 1. Decision, use case and stages

**Prefer Google's unmodified official Antigravity CLI, launched as `agy acp`, for the first personal interactive external-agent session in DSH/Flywheel.** The same official runtime owns Google sign-in, credentials, model calls, tools and the internal agent loop. The user connects, sends development tasks, handles permissions, reviews results and interrupts work; the desktop host provides the ACP client and interface. CLI 1.5.14 is the exact implemented runtime target [S28]. Official bytes for four platforms and unauthenticated Linux initialization have now been checked; live-account and other-platform execution acceptance remain separate.

The independent Registry `antigravity-acp` remains an official, valid alternative with its own qualification. Neither runtime silently substitutes for the other or for headless. The desktop connection now defaults to ACP; the standalone legacy task/tool interface requires explicit `transport:"headless"`. The machine stream-json API and existing automated task behavior remain separate. Initial ACP sessions are not automatically added to JEV, Plan/Execute/Review or other machine candidate sets. A UI session does not establish Flywheel task completion or independent Review.

**Decision history.** The original proposal incorrectly grouped Google's independent `.par` distribution with third-party wrappers. The first revision corrected that and selected Registry-first to match the requested artifact. After the user's explicit request to compare both official choices, this revision recommends CLI-first based on official integration guidance, versioned fixes, T3's merged migration and reduced maintenance duplication (§3). It preserves the official status of the standalone distribution and the original filenames/links. This is an engineering preference, not a claim that only CLI ACP is permitted or that Google has retired standalone ACP.

| Stage | Scope, implementation status and boundary |
| --- | --- |
| Initial: personal interactive external agent | Implemented in the source candidate using official `agy acp` on macOS/Linux: native login helper, text and typed-image input, native Gemini model/configuration options, permissions, stop and exact history loading. Real-account login, image delivery and native collaboration still need separate acceptance. Upstream Windows packages do not establish this project's Windows acceptance. |
| Later: independent Registry runtime | Explicitly selected original `antigravity-acp`; independently validate source/version, platform command, auth, history and capabilities. It is not an automatic recovery route. |
| Later: automated Flywheel binding | Establish a separate implementation scope with technical proof and support for the selected use/authentication context before machine APIs, role admission or automatic scheduling. An interactive session cannot upgrade itself into this binding. |

ACP means **Agent Client Protocol**, connecting a client to a complete agent; it is not a model name. The implemented personal desktop entry now defaults to ACP. Auto/Quick/Full, plan approval, JEV's single switch, existing automated-task model/subscription defaults and independent acceptance remain unchanged. Speed and quality gains require measurement; the choice does not pronounce on every existing Flywheel use case.

## 2. Existing system and separate repair

The existing machine path remains:

`Flywheel Go owner → Antigravity worker → public plugin machine API → owned headless agy process`.

The plugin owns process groups and durable recovery journals; Go validates terminal state, identity, configuration and cleanup before consumption. The DSH UI does not write task state. Sources are [the Go driver][C1], [worker bridge][C2], [plugin runner][C3] and [machine types][C4]. New interactive sessions have separate session records; they neither write `state.json` nor fabricate Go invocation receipts.

| Baseline fact | Constraint on this proposal |
| --- | --- |
| Headless closes stdin after each prompt. | Preserve that behavior; history reuse, process residency and protocol are separate dimensions. |
| JEV/investigation require evidence-only; ordinary execution requires supported explicit authority. | Initial ACP joins none of these roles; a later machine binding proves each equivalent contract. |
| Model, effort, physical cwd, subscription controls and native mode are frozen identity. | A later machine identity also needs the precise ACP distribution and capabilities, not just `acp`. |
| Packet images currently require a verified Claude binding. | Interactive ACP image acceptance does not automatically change engine packet admission. |
| Swarm already requires Full, reviewed work and input checks. | Preserve them for existing and future machine bindings. An interactive session is not a reviewed packet. |
| Unknown invocations are not replayed. | An ACP Session ID or successful reconnect does not prove a lost prompt never executed. |

**Historical baseline defect and subsequent repair.** At `291c0a0`, adapter 0.2.1 accepted a nonempty `init.tools` catalog after managed-agent validation, but Go still required an empty catalog. Valid receipts could become unknown, freezing JEV's baseline or blocking investigation. [PR #62](https://github.com/ckanner/flywheel/pull/62) has since merged; it binds acceptance to managed-agent proof and a present tool catalog, fixes successful-receipt keyword misclassification, and records adapter → worker → Go → decision/investigation/recovery checks. [The corrected driver][C5] and [proof validation][C6] are subsequent evidence, not a rewrite of the original observations.

The original documentation revision checked M0 against the merged prerequisite and its existing acceptance record. The later implementation has its own dated test record; it does not replace that historical evidence. ACP did not perform the repair. Initial interactive sessions do not require Go lifecycle ACK machinery; that belongs to a later machine binding.

## 3. Evidence and alternatives

### 3.1 Two official distributions and supported use

| Distribution | Source facts checked on 2026-11-22 | Treatment |
| --- | --- | --- |
| CLI `agy acp` | Official editor entry with CLI agent/tools/settings/auth [S1, S4]. Latest observed release: 1.5.14, published 2026-11-21. Versions 1.5.10 and 1.5.13 fix integration-critical lifecycle, cwd and attached-content issues [S28]. | **Initial primary path.** Qualify exact official binary provenance, version, digest, arguments and account capabilities. |
| Registry `antigravity-acp` | Manifest identifies Google LLC; observed version 1.5.2; Google download URLs; Mac/Linux `agy_acp_server.par`, Windows `.exe`, with platform arguments and SHA-256 [S17]. | Official explicit alternative, separately qualified. Its version numbering, parameters and capabilities are not CLI's. |
| Third-party wrappers/internal hub RPC | Different from either official distribution. | Outside this primary path; never automatic fallbacks. |

Google documents Registry installation and arbitrary ACP editor/client integration [S1, S4, S18]. Both official distributions fit the supported product pattern of a client presenting the official agent. Terms §4 covers supervised/autonomous agents, while §6 and the FAQ restrict third-party use of personal sign-in; the FAQ also supports direct scripts/CI [S20, S21]. These sources do not establish a blanket ban on MCP, custom agents or official native collaboration.

The 2026-10-08 reply by Google-flair moderator chunduriv distinguishes direct scripts/CI and ACP editors from third-party coding agents wrapping consumer sign-in, and disclaims formal legal interpretation [S22]. Public profile data verifies this author's moderator/Google flair; the fetched profiles for Rebbanpalli_Naveen and Ambati_Rajendra expose neither marker [S33]. Their staff affiliation was not independently verified; missing markers alone do not prove that they are not staff. Treat the T3/OpenBot/DSH replies as contextual interpretations, not a substitute for formal terms or product documentation. An outer automated binding still needs its own use/authentication basis; choosing CLI instead of Registry changes no such boundary.

**Versioned model evidence and current admission.** CLI 1.5.1 release notes offer account-available models to every ACP client, whereas the editor guide retains a non-Gemini client allowlist [S4, S28]. Preserve that research discrepancy. The implemented personal entry deliberately admits only native-session-reported Gemini models, with honest client identity; a cached non-Gemini selection blocks prompting rather than being silently replaced. Broader model support is a later decision requiring its own evidence and acceptance, even if the runtime advertises those options. Do not guarantee Claude or every listed model, impersonate an editor, or extend the CLI release statement to standalone ACP. ACP v1 optional features require negotiation [S5–S8]; v2 preview is outside the initial client.

### 3.2 T3 and Pi: implementation evidence

| Pinned source | Actual implementation | Consequence |
| --- | --- | --- |
| T3 source `f10e1eb3825a773cf702569d2258edfdbc8b77dc` | Built-in provider downloads official CLI, runs `acp`, and uses that program's `acp --login`; a generic Registry provider also exists [S23, S24]. | Supports the CLI-first integration pattern. Source references preserve the original review snapshot; newer merged migration/fix evidence is listed below. |
| T3 merged #35409 and #38970 [S29] | Managed installs moved from standalone 1.4.5 to official CLI to support native `/goal` and CLI updates. #35409 records macOS arm64 migration to 1.5.7, history/login retention and image/model/permission/interruption smoke checks. Later T3 support removed its old standalone launch path. | Real integration evidence favors CLI. It does not verify our adapter, every platform or cross-runtime history, and T3's policy is not Google deprecation of Registry ACP. |
| T3 compatibility/quota code [S23, S25] | Legacy `zed` identity fallback, token/private-quota queries and synthetic SSH credential-storage changes. | Do not copy these choices. Official ACP on the main path does not make every auxiliary feature token-opaque or establish blanket compliance. |
| Community `zacbemis/pi-antigravity-acp-provider`, `6d6135d888cb50e2cb1c18bc2520a94ac7d26392` [S26, S34] | Official standalone Agent and native authentication; Pi stores a configured marker. Pi tools are offered over MCP, and their results resolve the still-pending ACP prompt. The official Agent retains inference and internal execution. | A provider label or MCP bridge is not evidence of a replacement model loop. Borrow client/tool-continuation design as needed; the first client omits host-tool bridging as an implementation scope, not a policy judgment. This is not Pi core. |
| Pi local auth health [S34] | Reads token-file structure to check refreshability, without using the extracted token to run model inference; it is not wholly credential-file-opaque. | Our client should obtain auth state through official runtime methods rather than copying this local-file probe. |
| Other Pi Antigravity extensions [S27] | Direct OAuth implementations also exist. | Name the exact repository/commit; do not generalize from the Pi name. |

### 3.2.1 Why CLI is preferred for this project

| Criterion | Official CLI ACP | Official standalone ACP | Project decision |
| --- | --- | --- | --- |
| Product direction | Google documents `agy acp` for general editors/clients; agent, tools, auth and settings come from CLI [S1, S4]. | Explicit Registry integration remains documented and current [S17, S18]. | Prefer the general CLI integration; retain the official alternative. |
| Features and correctness | Versioned model expansion, images/embedded context, native commands, history and subsequent lifecycle/cwd fixes [S3, S28]. T3 migrated for native goals [S29]. | Its own feature/configuration line; current 1.5.2 must be measured independently. | CLI has stronger evidence for the native capabilities this project targets; do not assume all terminal UI or commands are exposed over ACP. |
| Maintenance | Can use the official `agy` already needed by the project. One selected executable can support distinct ACP/headless adapters. | Adds another distribution, platform invocation and capability matrix. | Reduce duplicate installation/diagnostic work; keep transport records, sessions and acceptance separate. |
| Authentication | Native cached login, advertised `oauth-personal`, or official `agy acp --login`; exact same binary/profile context [S1, S4, S30]. | Its own advertised authentication; CLI login flags cannot be copied over. | Official-runtime ownership is preserved with either choice. CLI is not inherently a broader subscription authorization. |
| Performance/resources | Historical samples favor startup/RSS (§3.3); no current end-to-end result. | Historical samples use older standalone versions. | Prefer CLI on functionality/maintenance evidence; promise no fixed speed or quality gain. |
| Compatibility cost | Login helper, modes, background events, cancellation and self-update require real adaptation [S30–S32]. | Independent compatibility work; still useful for explicitly chosen Registry/editor environments. | Qualify one primary runtime first; add standalone only with a concrete need and its own tests. |

**Implemented admission and version rationale.** The source candidate accepts only the recorded official **CLI 1.5.14** version and binary digests. The original proposal identified **1.5.13 as a prospective minimum baseline** because it fixes session cwd and editor-supplied file contents; that rationale does not admit 1.5.13 or a general version range today. CLI 1.5.10 fixes post-turn child/background notifications and cancel/close cleanup; 1.5.1 broadens models and fixes session cancellation [S28]. These are release-note facts, not our live acceptance. Do not accept any `>=1.5.0` just because it has `acp`, or automatically admit every future version. Record exact tested version/bytes/platform and required capabilities; unknown or changed builds require requalification before business dispatch. This is not a minimum-version change to the existing headless integration.

### 3.3 What the performance reports establish

| Evidence checked on 2026-11-22 | Observation | Limitation and design consequence |
| --- | --- | --- |
| T3 discussion #28315, 2026-10-31 [S9] | One Linux x64 author compared standalone 1.4.1 with native CLI 1.4.12: cold initialize 3.03–3.50/0.26–0.50 s; warm session/new 1.39–1.48/0.030–0.055 s; RSS 453–578/215–279 MB. | Historical author-reported builds, not current 1.5.2/1.5.14 or a model/whole-task benchmark. Both sides are ACP; do not promise these ratios or attribute them to a headless-versus-ACP protocol difference. |
| 2BAB's 24-run comparison, 2026-11-11, updated 11-17 [S10] | Old ACP versus hub medians: text 11.79/9.21 s; file workflow 29.65/32.23 s. | Different versions and default context/tool policies; same-build native ACP latency was not rerun. No universal speed ranking follows. |
| Theo's X post, 2026-11-04 [S11] | Reports problematic bindings and roughly 5% fewer false reports after a CLI rerun. | No complete controlled evaluation; 5% concerns false reports, not latency or a protocol-wide quality coefficient. The X body was available through a public mirror, not a successful direct X read. |
| T3 issue #28172 [S12] | Old-server reasoning/stream completion problem; maintainer could not reproduce with native agy 1.5.12 and closed it on 11-20. | Historical symptoms must not be presented as a current universal native-ACP defect. Displayed reasoning is not an independent quality measure. |
| Upstream cache issue #1533 [S13] | Users report cache drops across interactive, headless and ACP use. | Backend explanations are not confirmed root causes; keep cache telemetry and do not promise immunity on either path. |
| Upstream timeout issue #1402 [S14] | Includes a native-ACP 1.5.5 timeout/recovery report on Windows 11 with Vertex AI/ADC, alongside TUI/headless reports. | That platform/auth path is outside the initial macOS/Linux personal-login scope. It motivates lifecycle tests, not a claim that our exact path reproduced the defect. |
| DSH user feedback, 2026-11-13 [S15] | Reports repeated work and lost context in an ACP integration. | Missing exact client/runtime/configuration evidence; not identified as this plugin. Test compaction/history rather than attributing it to the protocol. |

This research established no controlled quality benchmark across the current standalone ACP server, native CLI ACP and this headless adapter with comparable account, model/effort, context and tools. Different distributions cannot be described as identical executable bytes. Neither “ACP necessarily degrades quality” nor “the transports are proven equivalent” is justified.

### 3.4 Alternatives

| Choice | Decision |
| --- | --- |
| Preserve existing headless | Continue implemented paths without changing runtimes/defaults through this proposal. |
| Official CLI `agy acp`, directly operated by the user | **Initial recommendation.** Qualify 1.5.14 as the first candidate, native login, capabilities, permissions, lifecycle and UI. |
| Official standalone Registry `antigravity-acp` | Retain as an independently qualified explicit option; no silent substitution or unverified history migration. |
| Immediately add all automated Flywheel candidates | Outside the initial release; technical gates and supported use/authentication must both be settled separately. |
| Third-party wrapper/internal hub RPC | Not adopted here; do not group them with the official standalone artifact. |

## 4. Ownership and proposed interfaces

The initial path is `user → DSH/Flywheel external-agent session UI → standalone plugin ACP client → official agy acp`. The plugin owns the connection, process group, event correlation, session record and cancellation. Google's agent owns models, tools and internal orchestration. The UI sends user actions and displays facts, without a parent agent deciding or delegating the same work again.

### System instructions and the Zed comparison

This is the same **external-agent client architecture** documented for Zed, with our own implementation and honest client identity; it is not Zed code, a Zed built-in model provider or a claim of identical capabilities or terms [S4, S35]. Zed's own documentation separates external-agent configuration from its built-in agent profiles and allows some MCP forwarding and client-side permission handling. Those mechanics alone do not establish who owns an inference loop or whether a particular use is supported.

| Input or responsibility | Initial ACP contract |
| --- | --- |
| Official base system instructions | Preserve them. The host neither replaces nor appends system instructions through CLI flags, ACP metadata or hidden prompt prefixes. |
| DSH Presets/PTC roles and machine instructions | Do not inject them as system text, generated rules/agent files or instructions disguised as user input. Do not select a host-generated managed agent for this session. |
| User task, selected code and attachments | Send explicit user input and relevant content through supported ACP prompt/context fields; do not silently attach host control instructions. |
| Existing project rules and native configuration | Let the official runtime discover user-controlled configuration through its documented mechanisms. It may combine rules such as `AGENTS.md` into the system prompt [S36], so the final prompt is not promised to be byte-identical. The client does not rewrite or generate those files. Native customization is distinct from host-injected control instructions. |
| Tools and permissions | The official agent chooses its work. The client presents and correlates permission requests and user choices; initial scope has no DSH tool bridge. Authorized protocol I/O is not itself a second model loop. |
| Iteration and machine completion | Human turns interact with the official agent's internal loop. The client does not add a DSH parent agent, automatically schedule Plan/Execute/Review/JEV, or turn this session into a machine receipt. |

The preceding revision retained an ambiguous project-instruction extension point. This table makes the initial boundary explicit: no host system-prompt replacement or append. It does not claim that every native rule, skill or supported MCP integration is prohibited by Google. The existing automated headless path in §2 remains a separate architecture; this ACP proposal does not certify that path's use/authentication context.

### Process and workspace ownership

**Each interactive session owns one process/connection, with at most one pending prompt.** Human turns can reuse it; this is not a shared daemon across users/tasks. Root prompt completion, settlement of known child work and process exit are separate facts. Closing a session performs bounded shutdown/cleanup; each turn need not have an exit code, and a root response cannot certify the whole tree.

Honor physical workspace ownership and leases before allowing writes. A direct UI session cannot overlap an active Flywheel writer or bypass its human-takeover gate. Use an explicitly available workspace or the existing takeover process. Any later transfer of edits into a task uses normal reviewed-artifact and verification handoff; interactive completion never changes task state.

Advertise only client capabilities whose handlers are implemented, authorized and tested. The initial client does not automatically expose DSH tools as MCP or inject servers merely because `mcpCapabilities` exists. The runtime's own MCP, skills, custom agents and internal collaboration follow supported capabilities and user authority; this is a client implementation scope, not a general ban on official features. Unsupported client-hosted fs/terminal/elicitation requests fail visibly; permissions require a real handler.

Resolve the same official executable for preflight, sign-in and connection. Initial integration uses an explicitly selected local official CLI installation; missing/unqualified installations get an actionable setup result rather than a silent runtime download or replacement. Record release provenance/checksum where available, resolved executable identity/digest/version, platform/arguments, adapter identity, protocol and capabilities. A version string or `agy` filename alone does not establish provenance. A future managed installer needs its own reviewed source/checksum, atomic installation and upgrade acceptance; a Registry option additionally records its manifest and full platform artifact.

Use real `clientInfo`; never impersonate an editor. Set the documented `AGY_CLI_DISABLE_AUTO_UPDATE=true` only in our CLI probe, login-helper and session subprocess environments [S32], without changing global shell settings. This stops our processes initiating updates; it cannot prevent another CLI/user replacing the executable. Detect changes before new business dispatch/reconnect, preserve active-turn identity and unknown outcomes, and require requalification rather than replay. Never upgrade in the middle of a session or silently accept changed bytes. Identity records contain no credentials.

A future host-instruction extension or DSH-tool MCP bridge requires a separate design, runtime acceptance and supported use/authentication basis; it does not inherit approval from this initial personal-interactive client. The standalone changelog's `systemPrompt.append` proves neither CLI support nor suitability for a host-controlled harness, and the initial client does not send it [S3]. For a later CLI client-supplied HTTP MCP bridge, the official guide requires `--process-per-session`; validate child-process ownership and shutdown before enabling it [S4]. Do not copy standalone server arguments.

### Initial interface example

This retained example is a **future client-settings design, not accepted configuration today**. The implemented entry uses DSH `config.antigravity`, defaulting to `{}`/`transport:"acp"`; see the [current configuration guide](../en/configuration.md#antigravity-and-gemini). The example is neither a machine profile nor a complete installation configuration, and its general model-selection label does not broaden the current Gemini-only entry:

```json
{
  "externalAgentSessions": {
    "antigravity": {
      "mode": "personal-interactive",
      "runtime": {
        "source": "official-cli",
        "command": "agy",
        "args": ["acp"],
        "release": "<explicitly-validated-cli-release>"
      },
      "authentication": "oauth-personal",
      "modelSelection": "from-session-config"
    }
  }
}
```

| Surface | Initial contract |
| --- | --- |
| External-agent session | Explicit connect/login, source-workspace selection and an isolated worktree, native Gemini model/configuration options, tasks, permissions, review and stop. |
| Public interactive plugin API | Separately versioned connect/authenticate/new-or-load/prompt/cancel/close and observations; no second native ACP parser in the UI. |
| Session record | Owned session/turn/connection identities, input manifest, dispatch/terminal/activity/cleanup facts; no task-state writes or machine-success receipts. |
| Capabilities/configuration | Distinct advertised/local-verified/live-verified/unknown and requested/accepted/observed facts; an echo is not independent backend observation. |
| Existing machine API and role/model bindings | No initial ACP candidates. Preserve headless protocols and omitted-value behavior. |

A later machine binding needs its own schema, transport/distribution fields, machine/worker/lifecycle/ledger versions and migration; old readers must reject unknown ACP facts. The interactive API can reuse verified lower-level modules without acquiring machine eligibility. Existing Go success requires its terminal, exitCode 0 and cleanup contract; persistent interactive events must never masquerade as such a receipt.

## 5. Connection, official sign-in and model selection

The initial flow begins with explicit user actions:

1. Choose workspace and the official CLI installation. Check ownership/lease, binary provenance/version/digest, platform arguments and local configuration under the same native environment. A valid artifact proves neither account entitlement nor quota. Use the §3.2.1 candidate/version policy; do not treat old headless qualification as ACP qualification.
2. Launch the owned `agy acp` process, send `initialize` with honest `clientInfo` and implemented capabilities, and inspect actual authentication methods and capabilities. Missing required support prevents business dispatch.
3. Present Connect / Sign in with Google. The CLI may reuse its native cached login. When authentication is needed, the implemented client uses the official `agy acp --login` helper for the personal flow. Direct ACP `authenticate` remains a possible later route requiring separate validation. Use the same resolved executable, profile/settings arguments and environment for helper and session. Do not assume `$BROWSER` interception works: T3's migrated login needed an explicit helper [S30].
4. When the helper route is selected, run it as a separate, bounded authentication process, not on the active JSON-RPC stdout stream. Use only one authentication route per attempt. Present the official URL and pass the user's official code to that process; the CLI performs the OAuth exchange and credential storage/refresh. Neither authorization codes/URLs with sensitive parameters nor tokens enter durable logs/journals. Handle completion, cancellation, timeout and errors without changing accounts, API keys or billing paths. The standalone `.par` option must use its own advertised auth, not CLI `--login`.
5. After login, reconnect/revalidate under the same identity and create a session, or load an exact matching session selected by the user. Use runtime/transport-scoped records: shared native storage does not authorize loading an active headless conversation, selecting latest, migrating Registry history or resending a lost prompt.
6. Read the session's actual model/effort/mode/permission options and confirm the user's requested values were accepted [S7]. Apply the current Gemini-only admission before prompting or selecting a model; an advertised non-Gemini choice is not eligible. Do not hard-code `thought_level`/`thinking_level`, map standalone `yolo`/`auto_edit` onto CLI, or assume every account advertises `auto`. Absent required options produce an explained unsupported result rather than silent policy widening. Fixed effort remains fixed; if a runtime exposes planner/executor effort, record those values separately instead of filling in high.
7. Freeze the complete prompt/attachment identity and persist dispatch-possible before writing `session/prompt`. The interactive client's record owns this boundary; a future machine path additionally needs Go durable lifecycle ACK.

Google documents shared authentication across local surfaces [S19]. **Do not promise independent login caches per provider instance.** Any separate settings directory needs official support and validation and does not establish account isolation. Do not simulate SSH, copy tokens, read keyrings, register our own OAuth client, proxy private quota endpoints or substitute client identity. Native `/usage` or ACP usage can be used only when supported/validated; missing quota remains unknown.

Diagnostics use verified public interfaces. Probes that create history or auxiliary work declare their effects and use owned isolated directories without business prompts. Model/effort/authority changes apply only at confirmed safe boundaries; active turns retain their original requests and later observations. A source or credential-store similarity between runtimes does not prove a portable session.

## 6. Lifecycle, cancellation and recovery

### 6.1 Interactive sessions

Record client session/turn ID, adapter run ID, native Session ID, connection generation and JSON-RPC request ID. A client RPC ID is not a provider idempotency key. Permit at most one unresolved prompt; queue further user input. The next prompt requires a verified settled boundary for relevant native work. Known active children or unknown prior work keep it blocked; steering is a separately verified later capability [S16].

| Event | Required behavior |
| --- | --- |
| Failure before prompt bytes can leave | Mark proven not-dispatched only when durable records establish that this business prompt was never written and relevant pending requests have settled. A usable session process need not exit; this cannot clear earlier unknown requests. |
| Write attempted, disconnect, timeout, malformed/oversized frame | Preserve possible/accepted dispatch and unknown outcome; no automatic resend, runtime substitution or transport change. |
| Root prompt response | Show correlated stop reason/output and continue observing known post-prompt/child work. Root completion, known-work settlement and process closure are separate; no automatic Flywheel completed verdict. |
| User stops | Send `session/cancel`, reply `cancelled` to pending permissions, and drain a still-pending root request to its response. Sending the notification does not prove work stopped; use bounded TERM/KILL when needed and record cleanup/unknown remote work. |
| Children remain after root response | Continue owned events/permissions. There is no pending root response to prove cancellation now; use tested quiescence/shutdown observations. Killing a process does not prove child success. |
| Session close/host exit | Bounded shutdown of the owned process group, preserving history and unresolved records. Closing UI cannot discard dispatch facts. |
| Reconnect/history load | Exact owned session only; distinguish replay, current activity and new usage. Loading history cannot prove the lost prompt's outcome. Preserve unknown requests; never resend automatically. |
| Partial output then refusal/cancel/resource cutoff/error | Retain partial evidence and native reason, without promoting it to success or safe retry. |

A turn may have a terminal/activity state while its process lives; process exit/cleanup belongs to session closure. Protocol-permitted post-root activity is not itself a violation [S8], but cannot be ignored when contradicting previously claimed final quiescence. Missing child observation cannot prove a quiet whole tree. ACP cancellation, local cleanup and cancellation of unknown remote work remain distinct; they do not set existing `providerWork.cancelSupported`/`querySupported` to true.

Associate sessions with physical workspace, runtime source/version, account/configuration references, authority, model and input identity. Shared authentication is not shared conversation. Cross-runtime/transport history migration needs explicit validation and cannot retire unknown requests. Compaction/replay cannot override confirmed user requirements; test constraint retention, and preserve unknown accounting/compaction metadata.

### 6.2 Additional contracts for a future machine binding

This is not an initial interactive-UI capability. A later automated Flywheel proposal still freezes invocation identity, persists dispatch-possible and obtains Go lifecycle ACK before the prompt, binding Flywheel request ID, role, target, task/packet and budget. Recovery consumes a valid durable terminal once; missing outcome/cleanup blocks cold starts and budget resets.

Choose and validate either a process per invocation or reuse with sufficient quiescence proof; initial interactive residency is no machine terminal guarantee. Receipts distinguish outcome, terminal, cleanup, model/authority, collaboration, images, usage and provider-work uncertainty. Independent reviewers do not inherit executor history; full/verified-incremental policy remains. Future steering needs separately versioned authorization/input revisions, not a bypass around frozen input.

## 7. Authority, intended use and evidence-only

### Initial personal sign-in and permissions

The user directly operates the official agent and completes its browser/terminal login. Bind permission requests and responses to the right session, turn and request. A `selected` outcome uses an offered `optionId`; cancellation returns `cancelled`. Explain scope/persistence instead of granting allow-always or widening directories for the user. Without a verified approval bridge, do not dispatch tasks needing one. Plan/accept-edits labels do not prove OS isolation.

Honor the chosen personal auth and applicable subscription controls; reject unauthorized API/ADC/GCP/custom-provider/credits fallback. Verifying a control is a property of this runtime, not an assumption copied from headless configuration. Auth failure requires explicit user recovery, not account rotation. Entitlement, verified local controls, quota and actual billing remain distinct; absent public quota evidence stays unknown.

Choosing personal interactive scope does not prohibit all automation. Official native tools, MCP, custom agents and Orchestrator/Swarm have their supported contexts; an outer third-party agent driving consumer sign-in needs separate support review [S20–S22]. API/enterprise auth is also a separate use and billing choice, never an automatic replacement for personal login.

### Future machine authority and evidence-only

Automated ACP role admission requires both supported use/authentication and technical evidence. New approvals beyond granted authority are reported, not automatically accepted or left waiting indefinitely. Reimplement equivalent, verifiable environment filtering, credits controls and configuration checks for the selected runtime; matching setting names are insufficient.

JEV remains a fresh, tool-free decision under the single `jev.enabled` policy. ACP cannot give it delegation or Orchestrator/Swarm. Evidence-only needs proof that the active session uses the managed agent, immutable configuration and run ID/digest, with tools/MCP/skills/plugins/commands disabled and no actual tool/child steps. A nonempty catalog is not an executed tool.

Whether ACP supplies that active-agent proof remains unresolved; `--agent` or checking a file is insufficient. Until accepted, ACP is ineligible for JEV/investigation; the separately repaired, verified headless path remains available. Never remove `evidenceOnly` to pass admission. This is a role-specific restriction, not a ban on normal interactive sessions or official custom-agent features.

## 8. Images and native collaboration

### Original-image input

Verify image prompt capability and the actual adapter/model path before interactive image input [S5, S6]; let the user select and preview authorized files. Preserve format, size/count, ordinary-file and authorized-root checks; freeze MIME, bytes and SHA-256 before dispatch. Send image bytes rather than treating a path, resource URI or existing chat attachment as delivery.

Record manifests/delivery evidence without base64 or credentials in progress logs. Distinguish serialization, prompt acceptance and live visual acceptance; use images not answerable from filename, text or metadata. Verified root delivery does not establish child delivery.

**Packet admission is a later, separate gate.** Change the current Claude requirement only after a shared engine/UI capability check is accepted. Otherwise preserve existing rules: fail unsupported input before dispatch, without deleting image declarations or degrading to text. Interactive attachments do not automatically enter task packets.

### Orchestrator, subagents and Swarm inside the official agent

These are native runtime capabilities, distinct from an external parent agent using Antigravity as its model/tool backend. The current personal client admits native-reported Gemini models and presents actual native configuration and activity; it does not implement a general Orchestrator/Swarm model-and-launch matrix. The following rows are further qualification requirements, not claims that each capability is enabled or live-tested. CLI terminal features and UI labels do not by themselves establish ACP delivery. Ordinary native mode does not promise zero children. CLI 1.5.14 documents local planner/executor effort configuration; expose it through ACP only when that session actually supports it. Existing headless fixed-effort machine profiles remain unchanged [S28].

| Capability | Interactive acceptance | Additional future Flywheel machine requirements |
| --- | --- | --- |
| Orchestrator | Not independently enabled by the current Gemini-only entry. Any later addition needs exact-model admission, actual fixed/configurable planner/executor effort, accepted values and honest internal-collaboration observations. | Preserve role/model eligibility; evidence-only cannot delegate. |
| Explicit subagents | Verify real delegation and ownership; missing observation stays unknown, not an all-successful tree. | Required work meets collection/receipt contracts. |
| Swarm | Require explicit user initiation using the verified native mechanism; 1.5.13 fixes unrequested Swarm starts [S28]. Slash-looking text alone is insufficient. | Execute + Full + reviewed independent collaboration/writes + input eligibility. An initial interactive session does not acquire these qualifications. |

Child-session presentation is optional; an agent may delegate without exposing children [S8]. Record requested mode, actual child IDs, root completion and child outcomes separately. Unknown models, usage, state and remote cleanup stay unknown. An observed-count guard is not a hard native concurrency limit. Pin schema/SDK; advertise `subagents` only with an implemented handler, separately gate draft variants, and accept child events only after verified ownership links them to this root.

Retain the future Swarm packet image exclusion until reviewed contracts express which workers need the original and delivery is accepted. Text-only downstream steps may use reviewed derivative specifications; deleting a real image requirement is not a migration.

## 9. UI, configuration and diagnostics

The source candidate adds a dedicated Antigravity external-session entry, defaults to official CLI ACP, and uses an isolated worktree plus personal login. **It does not add ACP to existing execution provider/model candidates.** ACP is the connection protocol, Antigravity the agent identity, and eligible model options are the Gemini choices reported by the established session. The following UI requirements remain acceptance criteria; actual completed checks are in the validation record.

The UI must:

- Show disconnected/connecting/auth-required/signing-in/ready/running/waiting-permission/cancelling/unknown states, with explicit official login and cancellation.
- Explain that native sign-in may be shared across local Antigravity surfaces. Disconnect is not Google logout; perform logout only when supported and explicitly requested.
- Show distribution, runtime/protocol/adapter versions and integrity, available models/effort and advertised/tested/unknown capabilities; discovery is not entitlement.
- Separate user selection, agent-accepted configuration and last actual call. Use readable names and expandable raw IDs; do not invent effort defaults.
- Present permissions, actual tools and observed children for review/interruption; native self-assessment never becomes a Flywheel Review pass.
- Preview attachments, frozen identity and root/worker delivery distinctions; state whether any attachment entered a task packet.
- Correlate asynchronous auth/model results with exact session, runtime, target and revision. Prevent unsafe edits or implicit retry around pending/unknown work and enforce workspace ownership/takeover.

A later machine configuration can add transport/distribution, effective task scope/source, JEV explanations and engine-shared eligibility, rereading effective values after save. Keep these distinct from `native-profile`, model, effort and JEV's switch. This later design does not alter existing tasks, approvals or initial defaults.

## 10. Implementation, release and rollback

1. Retain #62's merged receipt/packaging repair and its original headless source/registry/live evidence. Those fixes belong to #62, not this ACP implementation; current regression checks are separate dated results and do not rewrite that history.
2. The standalone 0.3.0 source now implements official CLI ACP, the native login helper, exact 1.5.14 admission and the public interactive API. Machine protocol version 1 and existing automated candidates remain unchanged.
3. Flywheel 0.39.0 now connects its independent Host/UI through that API. Record offline text/permission/lifecycle/recovery and candidate-installation checks separately from still-pending real-account login, Gemini inference and native background-cleanup acceptance.
4. Publish a tested plugin only under separate release authorization, pin exact package identity, and verify the same candidate installation/documentation. Desktop sessions default to ACP; legacy standalone task/tool use requires explicit headless, while Flywheel's existing machine path remains separately selected. Reject unsupported runtime/adapter combinations instead of substituting silently.
5. Typed-image input is implemented, but real visual delivery and native collaboration require independent acceptance. The Registry runtime, broader model support and later machine use/authentication/technical contracts remain separately scoped proposals before any schema, bridge, Go-owner, ledger, UI or migration change.
6. Before closing/removing the interactive entry, handle owned connections and retain unresolved records. Disconnect does not log out or delete shared credentials. Keep old journal readers; do not replay unknown ACP turns through headless or another distribution.

Initial human turns share their session connection. Cross-session server sharing, parallel sessions in one server and mid-turn steering still require independent tests. Future automated feedback respects frozen inputs and adoption boundaries; interactive buttons cannot bypass the Go state machine.

## 11. Verification and comparative evaluation

### 11.1 Staged correctness gates

| Area | Initial acceptance | Additional future machine acceptance |
| --- | --- | --- |
| Source/protocol | CLI provenance/version/binary digest/arguments and drift; Registry additionally needs manifest/artifact identity. Unsupported protocol/options, bounded Unicode/JSON framing and capability differences. | Identity in frozen bindings/receipts/recovery; strict old-reader rejection. |
| Model admission | Accept only native-reported Gemini models in the current personal entry; reject non-Gemini configuration and block prompting on a cached non-Gemini selection without silent replacement. Broader runtime catalogs are research evidence, not implemented support. | Separately qualify each role/model and any expanded supported range. |
| Instruction/agent ownership | Inspect launch arguments, ACP metadata, prompt/context and file writes: no host system-prompt replacement/append, Preset/PTC or managed-agent injection, DSH tool bridge or automatic outer loop. Preserve explicit user input and runtime-owned project configuration. | Separately establish the supported use/authentication context and review any changed instruction/tool/loop ownership. |
| Auth/permissions | Actual `authMethods`, the implemented separate official `agy acp --login` helper, same verified binary/profile context, success/cancel/expiry/failure, no credential extraction or sensitive logs, shared-cache disclosure; wrong-session/late/denied approval. Direct ACP authentication is a later independent gate. | Supported auth/use context, bounded handling of insufficient authority, verified subscription controls. |
| Lifecycle | Turn versus process exit; cancel during tools/permissions/children, disconnect, timeout, host death, SIGKILL, malformed/duplicate/late terminals, owned-group cleanup and no next turn before settlement. | Go ACK crash windows, one-time valid consumption, unknown blocked, preserved budgets and required-work proof. |
| History/accounting | Exact session load, no automatic replay, compaction retaining requirements, cumulative counters not summed per turn, unknown missing/negative/inconsistent deltas. | Role isolation, full/incremental input, independent reviewer and task/packet/target identity. |
| Images/collaboration | Frozen bytes, changed-source rejection, real visual task, child ownership, Orchestrator effort, actual Swarm mechanism and unknowns. | Shared image admission, Full/reviewed work/write scopes and required worker image delivery. |
| Evidence-only | No initial claim to these roles. | Active managed agent, no tools/delegation, immutable identity, actual steps versus catalog. |
| Distribution/UI | Clean public import, version rejection, same Linux/macOS candidate, session/permission states, asynchronous auth/model correlation, workspace lease/takeover and docs. | Machine/ledger migration, effective-config sources and consistent task/role eligibility. |

Implementation follows applicable repository gates: 100% measured Go statement coverage per function for Go changes, plugin/bridge behavior checks and actual candidate installation for release changes. The original documentation-only revision ran static checks. The subsequent implementation follows these gates and records actual results separately. Offline fixtures prove neither account access, login, visual quality nor performance; live records must name runtime, platform, account context and tested scope.

### 11.2 Comparison design and attribution

Freeze tasks, scoring and acceptance margins first. Prefer a **same-CLI-build `agy acp` versus headless protocol experiment** for common supported inputs, alongside a separately labelled **standalone Registry ACP versus CLI ACP product comparison** when both are qualified. The product comparison includes different runtime bytes/configuration; do not attribute all outcomes to ACP. Record artifact identities, account/auth, model/effort, settings/rules/plugins, authority, workspace, input and history/compaction policy. A same executable does not by itself make the session defaults identical.

Randomize/alternate order; use at least ten paired repetitions per case for an exploratory report and retain every failure. Cold trials use fresh matched sessions; warm trials use equivalent histories. Label uncontrolled backend caches honestly. Run only in authorized disposable workspaces within supported use/auth boundaries. Initial evaluation is user-triggered and does not enable an outer unattended agent loop for benchmarking.

Cover text edits, deterministic tests, multi-file repair, long-context continuation/compaction, images and verified native collaboration. Evaluate evidence-only and automated packets only after later admission. Unsupported cases are capability findings, not numerical victories. Record separately:

- Process start, initialization, session create/load, dispatch, first event/visible text, turn terminal/post-turn activity and close/cleanup latency; login latency separately.
- Accepted model/effort, observed model/tool counts, scoped tokens/cache/compaction/quota. Absent public telemetry remains unknown; do not probe with extracted tokens.
- Deterministic tests, correctness, constraint retention, repeated work, missed tools and failed/unknown outcomes.
- Cancellation/recovery, process count, peak RSS, cold/resident costs; missing displayed reasoning does not establish model ability.

Report medians, tails, paired differences, uncertainty, raw distributions and failure counts. No comparative quality or performance benchmark has been run. The user-authorized personal desktop default is now ACP; this does not change automated-task model/subscription defaults or establish a performance win. Future performance claims or scope expansion need the predeclared correctness/resource margins. Residency experiments must include warm headless stdin, warm ACP and the current per-invocation baseline, avoiding attribution of all residency gains to the protocol.

## 12. Milestones and open decisions

| Milestone | Implementation status and remaining acceptance |
| --- | --- |
| M0 — confirm merged prerequisite | #62's receipt/packaging/cross-layer repair is merged; preserve its historical evidence separately from this ACP implementation's regression results. |
| M1 — official CLI ACP client | Implemented in companion 0.3.0 source with exact 1.5.14 identity, updater control, honest identity, official login helper and text/permissions/lifecycle/recovery. Actual offline checks are recorded separately; real-account login/inference remain untested. |
| M2 — interactive UI and delivery | Implemented in Flywheel 0.39.0 source: direct external session, native Gemini options, permissions/stop/unknown states, public API and owned worktrees. Exact package/candidate-installation evidence is separate; no new automated candidates. |
| M3 — individual capabilities | Typed root-image input is implemented. Live image delivery, native collaboration and complete child cleanup still need independent acceptance. Registry and broader model support remain future work. |
| M4 — evaluation and later decisions | Distinguish runtime/protocol/residency costs. Review future machine use/auth support and complete technical contracts separately; M2 does not automatically enable it. |

Settled implementation choices and remaining decisions:

1. CLI 1.5.14 provenance/digests and invocation are fixed in the implementation. Live personal-account Gemini/configuration acceptance remains pending; Registry and any broader model range need independent qualification.
2. The client implements the official login helper. Real login, shared-cache/logout effects and any future direct ACP authentication route still require the corresponding acceptance.
3. Human API/session records use version 2 with stable ACP v1. Offline lifecycle/unknown-result checks and native live background/close guarantees are separate evidence; the latter remain pending.
4. Typed image bytes and their identities are implemented. Live root delivery and required-worker delivery remain distinct acceptance questions; worker/packet rules are future machine scope.
5. For proposed automation, separately prove supported use/authentication, active-agent evidence-only, Go durable ACK/recovery and role/Full/collaboration/input eligibility, then allocate machine/ledger versions.
6. Preregistered quality/latency/resource measures and whether practical benefits justify another runtime or broader use.

The user's subsequent authorization resulted in the initial ACP implementation and personal desktop default in this source candidate. It does not by itself authorize publication, establish account-level compliance or live acceptance, or enable ACP for existing automated tasks. Those boundaries retain their separate evidence and decisions.

## Sources and verification boundary

Research sources were checked on 2026-11-22 and remain historical observations. Official docs/Registry establish public paths; pinned source establishes implementations; issues/forum posts establish particular reports. A project's existence does not establish permission for every use. Record verified profile markers and unverified affiliations separately; missing markers alone do not identify someone as non-staff, and forum interpretations do not replace formal product documentation or terms. The subsequent implementation's source, offline and candidate-installation checks are recorded separately. Real-account ACP login/Gemini inference, live image propagation, native detached-task cleanup, comparative quality/performance and personal-machine acceptance remain untested.

| ID | Source | Use |
| --- | --- | --- |
| S1 | [Google: CLI ACP](https://antigravity.google/docs/cli/acp) | Official native entry and editor integration. |
| S2 | [Google: headless mode](https://antigravity.google/docs/cli/headless) | Automation, persistent stdin, text-only input. |
| S3 | [Google: changelog](https://antigravity.google/docs/changelog) | Versioned CLI ACP history and standalone `systemPrompt.append`; do not assume an extension applies to both runtimes. |
| S4 | [Google: ACP editor integration](https://antigravity.google/docs/ide/extensions/acp) | CLI ACP authentication, flags and session features; these cannot all be assumed for the standalone server. |
| S5 | [ACP v1 initialization](https://agentclientprotocol.com/protocol/v1/initialization) | Version/capability negotiation. |
| S6 | [ACP v1 content](https://agentclientprotocol.com/protocol/v1/content) | Image and other typed prompt content. |
| S7 | [ACP v1 session configuration](https://agentclientprotocol.com/protocol/v1/session-config-options) | Runtime option IDs and accepted values. |
| S8 | [ACP v1 subagents](https://agentclientprotocol.com/protocol/v1/subagents) | Optional child-session exposure and ownership. |
| S9 | [T3 discussion #28315](https://github.com/pingdotgg/t3code/discussions/28315) | Single-author old/native ACP startup comparison. |
| S10 | [2BAB comparison](https://2bab.me/en/blog/2026-11-11-antigravity-hub-vs-acp/?source=bokelianmeng.com) | Task-dependent old-ACP/hub timings and limits. |
| S11 | [Theo original X post](https://x.com/theo/status/2118107369448276371), [readable mirror](https://nitter.gq/theo/status/2118107369448276371) | Firsthand bindings/false-report claim; direct X body unavailable in this review. |
| S12 | [T3 issue #28172](https://github.com/pingdotgg/t3code/issues/28172) | Historical streaming bug and later native-runtime retest. |
| S13 | [Antigravity issue #1533](https://github.com/google-antigravity/antigravity-cli/issues/1533) | Cross-mode cache reports, not confirmed root cause. |
| S14 | [Antigravity issue #1402](https://github.com/google-antigravity/antigravity-cli/issues/1402) | Versioned timeout/session recovery reports. |
| S15 | [DSH community feedback](https://linux.do/t/topic/3173768) | Uncontrolled context-loss experience report. |
| S16 | [ACP v1 prompt turn](https://agentclientprotocol.com/protocol/v1/prompt-turn) | Prompt, updates, replay distinction and terminal/cancel semantics. |
| S17 | [ACP Registry: antigravity-acp manifest](https://raw.githubusercontent.com/agentclientprotocol/registry/main/antigravity-acp/agent.json) | Observed standalone 1.5.2 on 2026-11-22, Google URLs, platform commands/arguments and SHA-256; not an accepted default version. |
| S18 | [Google: Android Studio](https://antigravity.google/docs/ide/extensions/android-studio/) | Official agent installation through ACP Registry. |
| S19 | [Google: Authenticate](https://antigravity.google/docs/install) | Official sign-in and shared local authentication; no promise of independent credential caches. |
| S20 | [Google: Terms of Service](https://antigravity.google/terms) | Section 4's supervised/autonomous agents and responsibility; section 6's third-party restrictions. |
| S21 | [Google: FAQ](https://antigravity.google/docs/faq) | Direct headless scripting/CI versus third-party agents using personal sign-in. |
| S22 | [Google-flair moderator chunduriv, 2026-10-08](https://discuss.ai.google.dev/t/is-google-ai-pro-antigravity-cli-automation-with-codex-permitted/187241/2) | Lists ACP editors as supported and distinguishes third-party coding-agent wrapping; not a legal ruling. |
| S23 | [T3 official CLI download code](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityCliRelease.ts), [spawn configuration](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityAuthSupport.ts), [generic Registry resolution](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/packages/provider-acp-registry/src/server/AcpRegistrySupport.ts) | Pinned main source; built-in provider uses the CLI, distinct from the generic Registry provider. |
| S24 | [T3 sign-in implementation](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/Drivers/AntigravityDriver.ts) | Official `acp --login` process receives the user's code. |
| S25 | [T3 client identity](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/acp/AntigravityAcpSupport.ts), [quota probe](https://github.com/pingdotgg/t3code/blob/f10e1eb3825a773cf702569d2258edfdbc8b77dc/apps/server/src/provider/antigravityUsageLimits.ts) | Legacy `zed` fallback and token/private-quota access; excluded from this design. |
| S26 | [zacbemis Pi ACP community extension](https://github.com/zacbemis/pi-antigravity-acp-provider/tree/6d6135d888cb50e2cb1c18bc2520a94ac7d26392), [runtime.ts](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/runtime.ts), [MCP bridge](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/mcp/bridge.ts) | Official standalone agent/auth and pending ACP turn resumed with MCP tool results; provider naming does not establish a replacement inference loop. Not Pi core. |
| S27 | [Another Pi Antigravity extension](https://github.com/Rahularya01/pi-antigravity) | Direct OAuth and ACP integrations must be distinguished by repository. |
| S28 | [Google CLI 1.5.1](https://github.com/google-antigravity/antigravity-cli/releases/tag/1.5.1), [1.5.10](https://github.com/google-antigravity/antigravity-cli/releases/tag/1.5.10), [1.5.13](https://github.com/google-antigravity/antigravity-cli/releases/tag/1.5.13), [1.5.14](https://github.com/google-antigravity/antigravity-cli/releases/tag/1.5.14) | Versioned model/lifecycle/cwd/attached-content changes and current qualification candidate; release notes are not our live acceptance. |
| S29 | [T3 merged #35409](https://github.com/pingdotgg/t3code/pull/35409), [merged #38970](https://github.com/pingdotgg/t3code/pull/38970) | Migration for native goals, author's macOS arm64 smoke record and later T3 support policy; not Google retirement of standalone ACP. |
| S30 | [T3 merged login fix #38953](https://github.com/pingdotgg/t3code/pull/38953), [login report #36034](https://github.com/pingdotgg/t3code/issues/36034) | Explicit official CLI login helper and limits of browser-environment interception. |
| S31 | [T3 merged lifecycle fix #38916](https://github.com/pingdotgg/t3code/pull/38916), [background-session report #36105](https://github.com/pingdotgg/t3code/issues/36105) | Client handling of post-root activity, permissions and stop; upstream fixes alone do not qualify our client. |
| S32 | [Google: CLI troubleshooting](https://antigravity.google/docs/cli/troubleshooting) | Documented `AGY_CLI_DISABLE_AUTO_UPDATE=true`; other processes can still replace a shared executable. |
| S33 | [chunduriv profile](https://discuss.ai.google.dev/u/chunduriv.json), [Rebbanpalli_Naveen profile](https://discuss.ai.google.dev/u/Rebbanpalli_Naveen.json), [Ambati_Rajendra profile](https://discuss.ai.google.dev/u/Ambati_Rajendra.json) | Public moderator/Google-flair markers observed on 2026-11-22 and limits of affiliation verification; absent markers do not prove non-staff status. |
| S34 | [Pi provider.ts](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/provider.ts), [auth health code](https://github.com/zacbemis/pi-antigravity-acp-provider/blob/6d6135d888cb50e2cb1c18bc2520a94ac7d26392/src/acp/antigravity.ts) | Configured auth marker versus native credential-file health inspection; no claim that the extension never reads credential files. |
| S35 | [Zed: External Agents](https://zed.dev/docs/ai/external-agents) | External-agent versus built-in profile/configuration ownership, optional MCP forwarding and client/native permissions; architectural comparison, not identical implementation or a compliance guarantee. |
| S36 | [Google: Rules](https://antigravity.google/docs/rules) | Native discovery and cumulative instructions; `always_on` rules enter the system prompt. Host non-injection does not imply that the final runtime prompt never changes. |

[C1]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/internal/drivers/antigravity.go
[C2]: https://github.com/ckanner/flywheel/blob/291c0a02f9042d522d3e0f3c62977f63149465cd/plugins/dsh/src/antigravity-bridge.js
[C3]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/runner.js
[C4]: https://github.com/ckanner/dsh-subagent-antigravity/blob/a20eb63bbfa2849df63043595475957bf4a4f04d/lib/machine.d.ts
[C5]: https://github.com/ckanner/flywheel/blob/4d83ad350504411e0d413638bc95a43eeffadb44/internal/drivers/antigravity.go
[C6]: https://github.com/ckanner/flywheel/blob/4d83ad350504411e0d413638bc95a43eeffadb44/internal/drivers/antigravity_evidence.go
