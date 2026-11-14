# Changelog

This file records source changes. Published package identities and validation
results are recorded separately in the [release runbook](docs/en/releasing.md)
and [validation index](docs/validation/README.md). Historical entries retain
their original language.

## 0.34.0 — Native harness capabilities

- Align native companion fixtures and provenance with published Codex 0.10.0, Claude 0.5.0, LCU 0.5.0 and ChatGPT 0.1.20; verify the exact registry Claude tarball in CI and release jobs. Document separate desktop, worker and SDK dependency trees.

- Add versioned native profiles across loops, routes, model profiles, second
  opinions and allowed execution bindings, including workbench selection,
  frozen history identity, durable native receipts and import-only diagnostics.
- Run DSH PTC composition from the shared generator with actual behavior probes,
  native subagents/Team/Workflow and managed LCU. Serialize desktop ownership,
  retain epoch-safe handles and durable media, and prove journal/physical cleanup.
- Add an owned Kimi Server worker for Agent/Swarm/Tower. Use independent Tower
  Git candidates, preserve dirty/untracked/binary changes and transfer only after
  completion, cleanup and source/contract checks.
- Integrate Codex Pro 0.10.0 and Claude Code Pro 0.5.0 native controls, child events
  and truthful accounting. Add independent Review/interactive Team job commands;
  unknown cloud work, child usage and cleanup never become inferred success.
- Preserve role independence, explicit accounts/permissions and existing JEV
  controls. Keep one complete npm package, bundle generator 0.4.0 and record the
  reviewed companion source artifacts without claiming registry publication.
- Refresh the bundled generator to create a profile's first preset patch or a
  new output file, preserving existing settings and concurrent file creation.

## 0.33.1 — Strict desktop read codecs

- Fix task discovery and configuration reads rejected by the actual DSH Client
  Gateway. Supply strict schema factories for session identity, read requests and
  result envelopes; consume the dynamic namespace through an owned injected
  scope, use the host Agent registry's `agentId` wire field, and preserve session
  isolation, teardown and silent reads.
- Exercise the shipped Client descriptor through the real public Client and Host
  Gateway on both supported host fixtures instead of mocking Remote mounting.

## 0.33.0 — One complete npm distribution

- Consolidate the CLI, DSH host/client, workers, embedded skills and four native
  engines into one `@ckanner/flywheel` npm tarball. Resolve and verify bundled
  native files locally; do not download or borrow platform packages at runtime.
- Migrate the old desktop package through the official DSH CLI, verifying the
  replacement before removal and retaining explicit rollback/cancellation state.
  Keep user configuration, modified skills and task histories intact.
- Publish one immutable candidate through OIDC, test its registry installation,
  then promote its single latest tag. Historical split packages remain unchanged.

## 0.32.1 — DSH compatible-series upgrades

- Admit compatible DSH 0.2-series rc and patch upgrades from 0.2.1-rc.5, while
  keeping the 0.3 series (including prereleases) outside the compatibility boundary.
  Replace per-version host/SDK allowlists, retain permission and cleanup checks,
  and validate the locked 0.2.2-rc.3 host alongside the legacy host fixture.

## 0.32.0 — Session-scoped workbench and bounded runtime decisions

- Bound unused DSH prewarm cleanup receipts to 8 KiB of serialized UTF-8, including
  JSON escaping and the trailing newline. Truncate only oversized diagnostic text
  while preserving identity, dispatch and cleanup evidence and normal short errors.
- Synchronize prewarm expiry fixtures after prepare admission before testing safe
  cold fallback. Cover expiry before admission separately, retaining bounded
  cleanup and rejection of fallback without a trustworthy receipt.

- Use accessible switches for the JEV master setting, DSH Review preparation and
  automatic Worktree management. Keep dispatch as a policy selector with explicit
  eligibility, preserve unrelated settings, and discard only invalid hidden drafts
  when disabling a feature. Clarify JEV live-disable behavior in the Settings form.

- Add optional ready-queue dispatch for independent Full packets in automatically
  managed Worktrees. Refill bounded Execute/L1 lanes as they finish; keep Review,
  Plan, shared-candidate validation and Git integration behind quiescence barriers.
- Add optional DSH SDK initialization during serial packet verification. Send the
  real Review prompt only after normal admission, preserve scoped history, and
  bind unused-worker cleanup to durable receipts and a version 3 session ledger.
  Other harnesses retain their public startup paths. Defaults remain batch
  dispatch and no prewarming pending live benefit acceptance.
- Honor `--pause-before review` after successful parallel batches and ready
  queues. Preserve verified candidates for resume before any Review or Git
  integration, without dispatching their Execute work again.
- Recheck current activity deadlines and process completion before applying
  queued prewarm timeouts. Stale callbacks cannot cancel a recently active Review,
  overwrite a completed result, or turn failed preparation into a cold call.
- Keep ready-queue replacements from reconciling a still-running sibling's
  invocation. The coordinator retains live-call ownership, shared accounting and
  history checks; genuinely unfinished prior calls still require recovery.
- Preserve proven DSH initialization error codes when consuming a prepared
  runtime, so authentication and invalid-request failures keep their normal
  retry rules instead of becoming transport failures.
- Preserve lowercase and mixed-case JEV credential variable names when freezing
  investigation contracts. Keep the existing configuration length limit and
  compatibility with previously valid preparation records.
- Extend bounded investigation with explicitly authorized reproduction commands
  in private input copies and exact GET/HEAD requests. Record physical outcomes,
  output limits and cleanup without presenting host processes as OS sandboxes.
- Add real failure-action decisions under the same JEV master switch: bounded
  repair, approved feedback checks, replanning and existing allowed model/effort
  bindings. Preserve final gates, role isolation and unchanged provider defaults.
- Persist implementation decision intents, effects and usage across cancellation,
  recovery, investigation handoff and parallel workers without resetting budgets
  or replaying unknown actions. Document engineering results separately from
  deferred live model, performance and desktop acceptance.
- Unify JEV under one default-off `jev.enabled` setting shared by configuration,
  desktop, investigation preparation/resume and explicit online evaluation. Remove
  the automatic shadow observer and separate legacy request client; preserve a
  migration error for the retired command and read-only legacy config migration.
- Automatically freeze enabled investigation policy, choose approved evidence
  actions, defer redundant collection and hand evidence to the analyst early.
  Record exact decision/evidence bindings without treating collection choices as
  proven root causes or completed implementation.
- Recheck current activation and policy before dispatch/adoption, preserve unknown
  request/action recovery and cumulative budgets, and show actual adopted/baseline
  behavior with the last observed JEV status. Retain local offline replay.
- Update Task/New/Run guidance and paired JEV/configuration/investigation guides;
  preserve Quick/Full eligibility, verification and provider/model defaults.
- Rebuild the console as a canvas-first workspace with separate Settings, fixed-height
  navigation, compact node actions, actual session receipts, and separate PR/Merge nodes.
- Expose inline role-history overrides, reconcile explicit Kimi Plan history choices,
  and hide token thresholds when budgets are off without disabling usage accounting.
- Dispatch ordinary revision-bound node actions directly; keep reason/confirmation
  for reopening completed work and retain all backend admission and recovery gates.

- Bind the DSH console to the current conversation, remove cross-session task selection,
  and ignore legacy passive status logs when rebuilding task associations.
- Move console reads to a session-scoped read-only Remote so polling no longer adds
  JSON command receipts to chat; keep durable records for explicit mutations and recovery.
- Show locations and action groups directly, align buttons, separate prepared-task
  launch from continuation, and move diagnostics out of the configuration form.

## 0.31.1 — Feedback-driven workflows and DSH rc.2 compatibility

- Declare DSH `0.2.2-rc.2` alongside `0.2.1-rc.5` and `0.2.2-rc.1`; the host gate
  now admits the current desktop release without an exact-version exemption.
- Keep all six Flywheel package versions and exact internal dependency pins at
  0.31.1. The vendored Codex Pro 0.9.0 candidate and optional Claude Code Pro
  0.4.0 peer remain unchanged; preserve prior test results in the validation record.
- Regenerate the lockfile against rc.2 dev dependencies; `npm ci --ignore-scripts`
  still resolves the reviewed `vendor/dsh-subagent-codex-pro-0.9.0.tgz`.

- Add versioned, Go-owned bounded investigations with background control, token-bound
  cancellation, evidence reports, original-scope repair handoff and nonduplicating usage.
- Preserve Task, New, Run and direct stages while retaining Auto as a Quick/Full selector.
- Reuse trusted persisted Plan checkpoints; provide structured comparable feedback and
  conservative progress detection without weakening complete acceptance or review.
- Add opt-in complete execution-binding allowlists and verified same-role incremental
  context; retain default providers/models, cold-start identity checks and unknown blocking.
- Add paired Choice/Noul decisions with separate data opt-in, durable unknown recovery and
  atomic diagnostic-action adoption; move legacy automatic shadow HTTP off the task path.
- Measure observable engine phase intervals with unknown coverage and separate cumulative
  durations from wall-time interval unions. Real model speed/quality remains uncalibrated.

- Make English the default root and DSH plugin README, with matching Simplified
  Chinese editions and compatibility links from the previous English paths.
- Organize bilingual guides, architecture, plans, validation records, and
  archives behind documentation indexes; reconcile current workflow and
  workbench descriptions with the implementation.
- Adopt the MIT License for Flywheel source and all six npm packages, retain
  third-party notices, and include both language editions in distributions.
- Define the AI agent change workflow, documentation ownership and update rules,
  bilingual review checklist, and evidence requirements in AGENTS.md and the
  contribution guides.
- Align skill instructions with task selection and read-only association, remove
  retired path-form guidance, and distinguish configured scheduling, unmanaged
  execution directories, and serial versus parallel Worktree failure behavior.

## 0.31.0 — 项目归属与执行目录分离

- Session 归属原仓库，执行、验收与交付仍绑定实际任务目录；提示词和界面分别展示两层位置。
- DSH SDK Session 使用稳定项目 header 和公开目录切换；桌面 Host 通过 Workspace 注册表补齐归属，探测会话使用临时持久化目录。
- 自动管理 Worktree 默认开启，与并行策略解耦；新默认允许并行，最大并发 2，仍核对任务依赖及写入范围。
- 共享目录并发采用批次验收：所有执行结束后验证完整共同候选，返修导致旧证据失效时重验，全部通过后统一提交并原子完成整组；保留未知调用和清理阻断。
- 关闭自动管理后允许并发，也支持执行者在实现前提交结构化目录提案；核对同仓库、基线、分支、干净状态和独占归属后登记，后续执行／Review／交付跟随该目录，外部 Worktree 保留。
- 旧回执先按原身份核对；同项目的旧会话可继续使用，不重写历史头部。无法分离 workspace-write 沙箱根的后端明确拒绝该权限组合，不静默扩大权限。
- 插件测试按文件串行运行，避免并发启动的测试进程挤占用于故障注入的真实截止时间；未放宽断言或 timeout。

六包正式发布、官方 registry 安装、全部 latest 与 Mac 桌面升级均已验收；详见 [0.31.0 发行记录](docs/validation/2026-11-11-release-0.31.0.md)。

## 0.30.2 — 画布优先的流程工作台

- 大画布、原生专注视图、任务子图、跨分页定位与窄窗口列表；节点详情按进展／会话／本次配置组织。
- 配置自动读取、中文阶段与推理选项、分钟／秒换算、相邻保存按钮；保护未保存草稿，恢复缓存时重新核对文件 revision。
- 重试与运行确认绑定原 revision，待交付依赖不可绕过；受管会话续接命令使用原会话工作目录。
- Quick 验证同时核对 L1 与终验结果；关闭的 PR 不冒充合并，旧远端快照在状态变更后失效。
- 运行记录以列表与详情呈现，对照对象不改变指标查询目标；费用缺失不显示为零。
- 保持 Go 控制边界和默认配置语义；JEV 继续默认关闭、只提供可选影子建议。

本条描述发行内容；六包 registry 完整性、latest 与真实桌面安装分别验收。

## 0.30.1 — 任务发现与可选影子评估

- 画布以任务选择为入口：按 DSH 会话 ID 找回 CLI 运行，旧记录一键选择；删除路径输入框，有实际位置时自动显示只读信息。
- 单阶段回执独立呈现；Review 默认展示 L1 → Review、记录评分和整体交付边界，完整流程可展开。
- 单阶段技能补充只读状态关联，区分加载技能、准备任务和实际启动。
- JEV 默认关闭，可选全局／项目开关；开启只做有界影子评估，失败不阻断原工作流，不参与实际 Quick／Full 选择。
- 修正 Claude 关闭 stdin 故障用例的额外子进程时序，并保留逐函数 100% 门禁。

源码、开发候选与正式发布分别验收；0.30.0 安装不会自动获得本轮变更。

## 0.30.0 — 流程控制台

- DSH 横向画布：规划／拆分、调度、任务、集成、终验、PR、Merge；节点运行、重跑与完整证据。
- 全局／项目配置编辑、逐阶段权限、Provider／模型／thinking 映射及角色历史策略。
- CLI 运行登记、多任务关联、运行归属与按 invocation 对账的成本／时延／验收统计。
- 受管 Provider session 交互、核对回执、验证交回与保持暂停退出；会话账本 v2 防止旧引擎忽略接管。
- 显式依赖／写入范围下的并行 Execute/L1、集中 CAS 与预算、串行 Review／合成候选复验。
- 明确授权的 PR 推送／创建和精确 HEAD 合并；保留未知或排队中的远端结果。

源码门禁与同候选双平台安装已通过；npm 可用性以六包 registry 完整性、公开安装及 latest 复核为准。

## 0.29.1

- 完成会话 P3 源码：Codex Pro 0.9.0／Claude Code Pro 0.4.0 公开 lifecycle v1、独立 durable journal 与只读 recovery，经 worker v3 接入 Go。派发检查点先 fsync 再 ACK；恢复终态仍绑定原候选和角色 guard，未知不盲目重放。
- 完成 P4 策略与实验工具：可按角色覆盖 fresh/scoped，按确认轮数、Execute 候选停滞和实测单次输入做有界重建；默认阈值关闭。固定源码与独立 oracle 比较三种策略，覆盖六场景并重复配对；离线 native fixture 不被当作真实模型质量或节省证据。
- 恢复可归属的迟到 usage 时写独立 correction，保留原调用／provider／日期，不重复计数或复活已过期日桶；metrics 与状态成功前同步文件与目录，持久化失败可安全重试。
- Codex 0.9.0 以已审查、真实 npm tarball 随 DSH 插件 bundle，保留源码与摘要，安装不依赖未发布 registry 版本。Claude 0.4.0 保持精确 optional peer；默认模型、loop 和账号渠道不变。

- 新增可选 `claude-code-pro` harness：通过 `dsh-subagent-claude-code-pro@0.4.0` 公开 machine v1 运行本地 Claude Code CLI，明确模型／thinking、full-access、fresh／同角色 scoped 续接和原始调用回执。添加可选 profile、worker、无推理 doctor、安装路径和双语指南，暂不改变任何默认 loop／route。0.4.0 是配套源码版本，本条不表示它或 Flywheel 已发布 npm。
- 修正已跟踪调用的派发前拒绝／取消：确认未启动就保存已知未派发与零消耗，配置修复后可继续；未知结果仍阻断。Intake／Garden 对有结构化事实的未知派发或结果也停止重试，进程清理不被误当作业务完成。
- 校准显式 pricing 估算：Claude／Codex Pro／DSH 的总输入包含缓存量，估价前拆成不相交桶，避免缓存 token 被按普通输入重复计价；legacy pi 的原分桶语义不变，缺失／矛盾数据继续 unknown。
- 实现 Auto／Quick／Full：宿主先研究并准备契约，Go 校验资格并持久化策略。Quick 保留 Execute、全部必需验证、独立最终 Review 与同一候选的 Git 交付；合格时 Plan／Judge 记为不适用。旧任务继续 Full，发现风险或验证缺口只允许向 Full 升级，保留预算、工作区和审计历史。
- 新增 `flywheel-task` 统一准备启动 skill、`flywheel_launch` 模型工具及 `control launch --mode auto|quick|full`。通过 DSH 公开会话投影关联确切任务，覆盖会话重放、fork 隔离与未确认操作；视图显示请求模式、生效策略、阶段适用性和升级原因。
- 补齐 11 个内嵌 skills 的安装校验、双语工作流／配置／入门指南，以及引擎、控制接口和真实 DSH 发布包的离线回归。模型、账号与权限配置继续沿用现有选择。

- 按任务、范围和角色复用 Codex Pro、DSH 和 Kimi 会话。对已保存的终态回执进行对账，避免重复调用和计量。旧任务配置保持原有默认行为。
- 修复 macOS 上临时目录别名导致的配对评估误拒，并校正 CLI、DSH 和 worker 测试夹具。Kimi 取消测试使用单进程夹具，避免子进程回收时序干扰断言。

## 0.28.5

- 将 Flywheel 自定义会话视图排到内置“对话／轨迹／统计”之后，保持内置视图顺序；不修改宿主 DOM，不改变任务、模型和权限行为。六包统一 0.28.5。

## 0.28.4

- DSH 兼容声明覆盖已核对的 0.2.1-rc.5 与 0.2.2-rc.1，并同步 Cordis 4.0／4.1 和 host peer 范围；未知版本继续由宿主拒绝，不需要版本豁免。
- SDK 权限事件移至 Session 创建公告完成后的公开 agent/created 阶段，并通过公开 approval.overrideOf 读取回执，保留显式 never、沙箱探测及缺失回执时不发送模型请求的约束。
- 使用新版实际发布的 renderer 和锁定的 rc.5 renderer 验证同一构建的插件生命周期；以真实 DSH 版本门禁检查包声明。六包统一 0.28.4，现有账号配置保持。

## 0.28.3

- 发行复核补齐 DSH 清理回执保护：worker 已启动但缺少可信终态时记录清理未确认，保留协议诊断并停止本次运行，避免自动重试重叠执行；启动前拒绝不误判为遗留进程。

- 将 DeepSeek Flash 默认与显式 subscriptions 预设改为 DSH 已登录的 `deepseek-account`，复用同一 DSH home 的账号授权；保留用户明确配置的 `deepseek-official` API Key。已有配置不随升级改写，新增从已发布 0.28.2 API 预设预览、备份并迁移到账号的回归。
- 将 DSH 账号未登录、token 失效及精确的账号退出取消事件归入既有 AUTH，账号额度不足保持 QUOTA，保留原始诊断并说明恢复方式；结构化码贯穿 Go 消费端，进程失败与清理未知优先，不把认证错误当成可重试服务器故障。默认预设没有 API 后备，保留用户显式配置的后备策略。
- 桌面控制入口省略项目目录时读取 DSH `workingDirectory.get(session)` 的当前目录，跟随会话切换 worktree；显式目录优先，读取失败在 Go 派发前返回可确认错误，不回退到过时的 `session.header.cwd`。
- 修正 DSH `/flywheel-*` 与 Kimi `/skill:flywheel-*` 的宿主语法区别，说明 Skill 主流程无需预填面板，以及任务资料目录和项目目录的作用。记录各模型实际支持的推理等级、high 基线依据和未做项目 A/B 的边界；不自动调整档位。

统一六包版本为 0.28.3。升级保留用户配置；采用新的账号预设需要显式预览并应用迁移。发行完成以六包 registry 完整性、安装和 latest 复核为准。

## 0.28.2

- 修复 macOS 迁移故障测试的平台假设：使用路径解析 I/O 注入缝稳定验证无法解析相对全局路径时的无写入行为；生产路径解析仍使用 `filepath.Abs`，不跳过测试或放宽 100% 覆盖率门禁。

- 新增显式 `config migrate --preset subscriptions`：预览并备份更新实际生效的旧模型配置，保留非模型设置，清理旧 Z.AI 后备路径，采用 ChatGPT／Kimi Code K3.1 Pro／DeepSeek Flash 与约定的 thinking；安装报告明确升级会保留旧配置。Kimi 缺省模型同步为 K3.1 Pro。
- 新模板与显式迁移使用 `permissions.mode: full-access`，通过 DSH／Codex Pro 公开接口落实所有阶段完全权限和无需逐步确认；旧文件缺省保留 role-based。编排、预检、恢复身份与实际权限回执一致，保留冻结验收、候选身份和进程清理门禁。
- 将 UI 路径和排障文案改为“任务资料目录”“项目目录（可选）”“检查安装与配置”“检查 DSH 接入”，说明任务由对话助手准备、项目目录可沿用会话、检查不发送模型消息，配置表展示可理解的权限名称。

## 0.28.1

- 将 DSH 桌面控制界面迁入顶部原生 `conversation.view` 的 Flywheel 标签，移除遮挡对话和输入框的底部大浮层。按外部 plain-JS 插件规范只加载 React，控件、样式和快照由插件持有，通过公开 StoreHandle 与槽位接入宿主会话生命周期；使用宿主 locale 与语义颜色。配置按可换行卡片展示，桌面对话、轨迹、输入框和原生侧栏继续由 DSH 管理。
- 补齐界面异常恢复：单会话单个在途请求、启动／停止时锁定目标、忽略旧路径的迟到响应，显示最近确认时间；确定未派发的错误可修正后重试，执行结果未知则查询原目标确认后才允许新的变更。界面开关不自动启动、停止或探测任务。独立 SDK worker 的能力边界与桌面 UI 分开说明。

- 新增仓库内 `release:local` 交互发布入口。维护者可从已固定的 release tag 在本机构建六包，再依次完成候选安装测试、远端 tag／源码／完整性复核、候选上传、registry 安装及 `latest` 提升，无需下载外部候选归档或人工复制发布适配器。新入口位于维护工具 checkout，支持独立的已打 tag 源码 worktree；保留既有标签与 CI 流程，失败时复用原候选。

## 0.28.0

Codex Pro 默认接入。安装和迁移见 [配置指南](docs/zh-CN/configuration.md#codex-pro-与旧配置迁移)；源码、测试、CI 和 registry 发布证据分别记录，不用离线通过代替用户 Mac 实测。

- 新增 `codex-pro` Go driver 与独立 Node worker，直接使用 `dsh-subagent-codex-pro@0.8.0` 的公开 machine API。默认 hard 路由使用此插件；DSH Flash 默认执行、Kimi CLI 规划／前端、DSH 原生 GPT 评审保持原角色。旧 `codex` CLI 仅作为用户显式配置的兼容入口。
- 完整传递模型、effort、目录、权限、取消与 deadline。每次使用 ephemeral 新会话；结果必须同时满足外层 worker 和插件终态／清理要求，正常 app-server 主动终止不冒充调用方取消。
- 指标保存完整 `codex-pro-receipt`；usage 只有 complete 才进入已知预算，partial/unknown 与美元未知成本保持可见。effort accepted 不伪造为已证实实际强度，不用账号 channel 冒充实际 provider。
- npm 自动安装精确插件依赖并解析同版本 worker，保持 hoisted dependency、GUI 启动入口、用户配置与 skills 保护；统一六包版本 0.28.0，Node 最低 22.18.0。
- 本地 doctor 验证插件公开 API、精确版本、Node 和 Codex 命令，不调用模型。配置错误、缺依赖与版本不符明确失败，不自动改走旧 CLI。
- worker 意外退出或清理回执缺失时明确记录清理未知；所有模型循环保存升级诊断、保留工作区并停止本次运行，禁止自动重试／降级与 Plan 冷启动，避免 guardian 清理期间出现重叠执行。正常错误且清理已确认时仍可重试。
- 明确 0.8.0 权限边界：插件在运行前验证模型／cwd／权限模式，完整 sandboxPolicy 由 Flywheel 在运行后核对；事后拒绝交付不能阻止自定义 Codex 配置已允许的访问。需运行前完整策略 guard 的环境仍待上游能力。
- 更新设计、安装／迁移教程、协议和相关技能按钮，增加协议、计量、生命周期、真实 npm 安装与跨进程回归。
- 补齐 10 个内置 skills 的任务 worktree、分支与绝对路径约定，区分任务集成目录和 packet 候选目录；恢复复用原任务，状态查询／停止保持可用，PR 合并仍由获授权宿主处理。
- 新增英文 README、入门／配置／工作流、DSH 插件说明、贡献和发行指南，并将英文用户文档纳入 npm 包。补充 CI 不可用时经维护者授权的手动发行流程，沿用精确源码、同一候选、六包 integrity 与 registry 安装门禁。

## 0.27.3

当前统一 npm 发行目标。本节记录修复内容，不代表本版门禁、registry 发布、latest 提升或用户 Mac 实机验收已完成。0.27.0／0.27.1 未上传 npm 包；0.27.2 的发行在验证阶段主动取消，未上传包，旧 tag 均保留不移动。

- 兼容 npm 12 `view --json` 的单元素数组结果，同时保留 npm 11 标量结果；多结果、错误结构与缺失 integrity 仍拒绝，只有结构化 E404 可表示版本不存在。
- 增加真实 npm CLI 对隔离本地 registry 的 `view` 协议回归，覆盖 latest、integrity、缺失字段及 E404／E403；不调用发布接口或真实账号。
- CI、六包候选构建、双平台验证与 publish 统一固定 Node 24.22.1／npm 12.2.1，并记录实际工具版本，避免测试与发布使用不同 npm 协议；原有权限、候选身份和全部门禁保留。
- 六个包、精确同版依赖、插件锁文件、构建入口和当前安装教程统一到 0.27.3。

## 0.27.2

[PR #19 CI](https://github.com/ckanner/flywheel/actions/runs/41388914605) 与 [main CI](https://github.com/ckanner/flywheel/actions/runs/41389529756) 均通过。随后预核真实 npm 12 的 `view --json` 发现单值结果变为数组，[0.27.2 发行 run](https://github.com/ckanner/flywheel/actions/runs/41389716920)在 preflight／build 成功后的 verify 阶段主动取消；未执行 npm 上传，不是 CI 失败。原 tag 保留不移动，后续修正版使用 0.27.3。

- 控制面测试夹具通过原子替换发布完整状态 JSON，消除模拟引擎直接覆写文件时被并发读取到半写内容的竞态；控制接口和生产状态文件契约不变。
- 发行构建兼容 npm 11 的单元素数组及 npm 12 按完整包名索引的单包对象 `pack --json` 输出，继续严格核对对象键、包名、版本、文件名及候选 integrity。
- 六个包、精确同版依赖、插件锁文件、构建入口及当前安装教程统一到 0.27.2；保留旧版本的实际验证证据和失败发行记录。

## 0.27.1

修复私有 GitHub 仓库的 npm 发行路径。`v0.27.0` 的首个 publish run 在上传前因裸 `git fetch` 缺少认证而停止，未发布任何 0.27.0 npm 包；该 tag 保留为源码和验证记录，不移动。随后 [0.27.1 发行 run](https://github.com/ckanner/flywheel/actions/runs/41387321980) 的 macOS 验证通过，但 Ubuntu 验证因控制面测试夹具的状态文件写入竞态失败，publish job 跳过，未上传任何 0.27.1 npm 包。0.27.1 tag 同样保留，后续修正版使用 0.27.2。

- 发布 job 在工具链准备后，通过官方 checkout 的短暂只读认证刷新 source 与完整 refs，随后下载候选并检查精确 tag；不再执行凭证已清理后的远端 fetch。
- 发布命令要求显式仓库可见性。public 保留 provenance；private 使用 `--provenance=false`，符合 npm 不支持私有源码仓库证明的限制，未知值在 registry 请求前失败。仓库可见性、六包 integrity、完整门禁和 latest 提升顺序不变。
- 增加私有仓库发布协议回归，实测显式参数覆盖 npm 环境默认值；六包、锁文件、构建入口和当前用户教程同步到 0.27.1。

## 0.27.0

DSH 桌面集成、可靠交付闭环与 npm 一条命令安装。此节记录对应版本的源码变更，不单独证明 npm 已发布或用户 Mac 已验收；用 `npm view @ckanner/flywheel version` 核对实际发行。旧 0.22.1 不包含本节新增安装和 DSH 接口。安装与迁移见 [入门教程](docs/zh-CN/getting-started.md)。

- **启动期间的停止快照**：修复真实子进程已经运行、`Cmd.Start` 尚未返回登记时，`KillActiveChildren` 可能取得空快照的竞态；启动与登记由同一把快照锁保护，失败路径也释放锁。新增真实进程的确定性回归，原验收停止测试及 3 秒期限保持不变。正常 machine SIGTERM 路径已有 context 取消兜底，本修复补足独立 kill snapshot 的已启动进程契约。
- **npm 一条命令安装**：新增 `npx -y @ckanner/flywheel@latest install` 与 `flywheel setup`，全局安装精确同版 CLI、平台二进制、worker、10 个内嵌 skills，再通过已有桌面 DSH 的正式接口注册并核验插件。支持 prefix／project／DSH 命令／profile、显式跳过、超时和 JSON 输出；普通 npm 包安装不通过 postinstall 修改用户目录。
- **软件安装与运行准备分开报告**：本地 doctor 有效失败保留完整检查项，允许 `ok:true, ready:false`；账号状态保持 unknown，不自动安装／登录其他 harness，不发送模型 prompt。取消、超时、损坏协议和版本漂移阻止安装成功，报告已完成步骤。
- **Mac GUI 自动入口**：生成使用绝对 Node／CLI 路径的 launcher 和带 SHA-256 的安装记录，桌面插件自动发现并验证；用户显式 `flywheelBinary` 优先。损坏或改动的入口明确失败，不静默使用其他版本，无需手写 GUI 启动脚本。
- **skills 更新保护**：快照清单校验内容、类型和模式，用户修改、新增／删除文件及未归属目录保留并报告冲突；逐 skill 暂存、rename 和失败回滚。旧纯文本标记仅在完整内容匹配时迁移。新增 `install-skills --json` 的完整逐项结果。
- **npm 启动器生命周期**：异步启动真实 Go 进程，传递取消信号并等待清理完成；核对原生包／worker 的精确版本，兼容依赖提升，避免启动旧二进制或留下 wrapper 的子进程。
- **六包发行门禁**：meta、四个平台包和 DSH 插件统一版本。先构建 worker，在隔离副本构建并打包一次，再于 Linux／macOS 验证同一候选的真实 npm 安装。发布要求 HEAD 精确 tag 与干净来源；已存在版本核对 integrity，只有明确 404 视为不存在；全部包与精确版本安装验证通过后才更新 latest，且不倒退标签。
- **GitHub 可信发布**：参考现有 DSH 插件的 Actions 流程，固定发布工具链为 Node 24.22.1／npm 12.2.1，支持 OIDC 的 publish 与 dist-tag；无 token 时使用独立空 npmrc，有 token 时保留后备认证。补充六包逐项授权、精确 tag 触发及原候选重试的维护者指南。

- **配置与模型 profiles**：新增可复用 `profiles`、阶段引用与显式覆盖；DSH `provider` 和账号 `channel` 分离，所有角色和渠道降级均传递实际 provider；指标分别保留两者。严格拒绝未知字段、无效引用和 provider 冲突，兼容旧内联绑定与合法显式零值。
- **只读预检**：新增 `config inspect`、`doctor` 和相应 control actions；检查有效配置、工具和任务所需路由。可选 SDK probe 验证配置、目录、沙箱与关闭，零推理，整体验证有界；账号、额度与实际模型能力明确标为未验证。
- **桌面可观察性**：面板展示配置、逐项诊断、完整 packet 状态与未知计量；区分完成、暂停、停止和升级，路径切换后忽略迟到的旧响应；增加宿主 React 18 对应的真实 DOM 交互回归。
- **DSH 入口与 worker**：新增 `control start/status/stop`、桌面面板、人工命令与模型工具。Go 继续独占状态；worker 校验 provider/model/cwd/effort、权限、可靠终态及 SDK 关闭。
- **模型路由**：默认 DeepSeek Flash 实现，Kimi CLI 规划／前端／整理，DSH ChatGPT provider 评审／终验，独立 Codex CLI 处理困难任务。Kimi 不使用 ACP；外部 Codex Pro adapter 与原生 LLM provider 分开，当前路由不隐式消费外部 subagent 工具。
- **可靠性**：验证证据绑定候选和契约；契约修订跨重启失效旧证据；L1、Review、里程碑 gate 与 Judge 前后检查身份；提交及合并后确认持久化结果，失败保留 worktree。
- **发布前复核**：修复从子目录漏读项目配置；持久化检查独立核对执行位与链接类型；验收输出截断不能因退出码符合预期而通过。CI 增加 Linux／macOS 插件、跨进程和 npm 安装检查；Actions artifact 用于传递和审查候选，正式用户从 npm 安装。
- **Mac CI 修复**：任务目录和 Git 执行根统一使用物理路径，避免 `/var`／`/private/var` 别名造成账本、报告和恢复判断失真；补充真实符号链接启动／恢复回归，并消除 Kimi 路径断言与 Zcode fixture 的写入／退出竞态。进程组清理探测的暂态 `EPERM` 在原宽限内重试，仍须确认 `ESRCH`；持续权限错误继续阻止交付。
- **生命周期与计量**：按运行 token 停止、运行 lease 防重入、超时／取消／清理失败严格区分；未知 usage/cost 不再当成零。
- **测试与文档**：普通单测／回归可随实现修改，冻结验收显式保护；保留逐函数 100% Go statement coverage；更新全部内嵌 skills 的控制接口、单环语义与配置说明，补充完整 Mac 安装和首个任务教程。
- **兼容边界**：旧配置不自动迁移；任务／项目／全局配置选择首个完整文件，不深度合并。Mac 桌面渲染、Seatbelt 和真实账号调用仍需用户环境验收；离线通过不等于实机通过。

## 0.22.1 (2026-09-14)

修复 0.22.0 meta 包缺 bin shim：`bin/flywheel.js` 此前被 .gitignore 的 `/bin/`（postinstall 下载目录时代的遗留规则）挡在 git 之外——CI checkout 无此文件，发布的 meta 包只有 3 个文件、`flywheel` 命令不存在。本版将 shim 入库并全量 bump 版本；旧 0.22.0 meta 请勿使用。

## 0.22.0 (2026-09-12)

三厂商 harness 定稿 + 完全自包含分发（安装用法人人一致，无需源码仓库）。

- **Harness 矩阵（DESIGN v0.22）**：Execute default/multimodal/Intake = zcode（app-server 协议，按会话指定模型与思考档）；Review + Judge 复核面板 = Codex（GPT-6-Astra）；Plan/frontend/hard/Judge 主判/Garden = kimi code + K3。跨厂商制衡升级（GLM 执行 ↔ OpenAI 评审）。
- **自包含分发**：契约 schema、skills 快照、§13 法定配置表全部内嵌二进制；`flywheel install-skills` 默认从内嵌快照物化（`--link` 为开发机源码树软链专用）；`flywheel template` 打印内嵌模板。
- **配置四级发现链**：任务目录 → 项目根 → 全局 `~/.config/flywheel/flywheel.config.json`（首装自动物化，可直接编辑）→ 内嵌法定表兜底。
- watchdog 交互钩子（协议驱动）、multimodal 执行路由、`FLYWHEEL_CONFIG` 覆盖。

更早历史见 [历史设计](docs/archive/design-v0.24.md) 的变更记录。
