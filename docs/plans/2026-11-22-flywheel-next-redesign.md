# Flywheel Next：不背历史包袱的架构、模型路由、跨仓库修复与 DSH UI v2 重构方案

[方案目录](README.md) · [文档首页](../README.zh-CN.md) · [当前架构](../design.md)

- **创建日期**：2026-11-22
- **状态**：已对齐拍板的设计方案与跨仓库实施清单（本 PR 仅提交完整方案与审计记录，不宣称以下改动已在 `main` 实施或发布；实施按第六节分阶段独立 PR 推进并分别验收）。
- **审阅基线**：
  - `ckanner/flywheel` `main` @ `4d83ad3`（v0.38.1）及在途 [PR #63](https://github.com/ckanner/flywheel/pull/63)
  - `ckanner/dsh-subagent-antigravity` `main` @ `ec98b4c`（v0.2.1）及在途 [PR #1](https://github.com/ckanner/dsh-subagent-antigravity/pull/1)（`feat/official-acp-default-2026-11-22`）
  - `dsh-subagent-claude-code-pro` `origin/main` @ `e8dcd14`（v0.7.0；本地 checkout 停在 `v0.6.1`）
  - `dsh-subagent-codex-pro` @ `69e180a`（v0.10.2）
  - `dsh-plugin-chatgpt` @ `afec4ca`（v0.1.23）
  - `dsh-plugin-lcu` @ `3361042`（v0.5.1）
  - `dsh-preset-generator` @ `c35c100`（v0.4.0）
  - `dsh-plugin-github-issues` @ `5b622a5`（v0.8.9，DSH 多插槽与 TS/TSX UI 工程标杆）
  - `dsh-plugin-alert-triage` @ `ce0889d`（v0.3.1）

---

## 一、重构目标与核心决策（已拍板）

本次重构不背历史 UI 与旧式 API 计费模型的包袱，围绕**“Go 确定性中枢 + 四家订阅各展所长 + 官方 ACP 默认接入 + 零技术债 DSH 原生工作台”**一次性对齐以下 7 项核心决策：

1. **双重独立设计（Dual-Draft）**：仅在 `Full` 架构级任务中启用一次 **`Gemini 4 Carbon (high)` + `Claude Opus 5.5 (high)`** 双盲独立出方案 → 逐条对比核实并合成统一方案（标明采纳来源与取舍理由）→ **`Codex GPT-6.2 Astra (read-only)`** 独立风险审查 → 人类一次拍板；`Quick` 保持单模型紧凑准备（`Hydrogen-medium` + `Sol-medium`）。
2. **前端交互与微文案独立复审（Copy & UX Pass）**：将 **“Gemini 4 Neon 审交互与微文案 → Claude 取舍改稿”** 设为 `frontend` 路由的**默认内置步骤**，根治 Claude/GPT 产出的“工程师天书式冗长 UI 文案”。
3. **Kimi Code 完整订阅能力全面解锁**：核实 `kimi-code` 源码与官方文档确认，`managed:kimi-code` OAuth 订阅原生包含 `WebSearch`、`FetchURL`、`Agent` 与 `AgentSwarm`。在核验官方 OAuth 与禁止 `KIMI_API_KEY / KIMI_WEB_*_API_KEY` 覆盖的前提下，**取消过度保守的工具与子代理禁用**，彻底释放已年付的高档 Kimi 产能。
4. **Antigravity 默认走官方 `agy acp`（ACP 协议）**：不仅个人桌面会话走官方 `agy acp`，Flywheel Go 引擎的 Antigravity 自动化调用也将升级为以 **ACP 为默认传输层**（修复 PR #1 的 5 项缺陷并补充 machine ACP runner），顺带解锁 **Gemini 原生图片输入** 与零工作区污染的只读/无工具控制。
5. **砍掉旧式美元 / 每日 Token 硬预算与单价估算**：在 100% 纯订阅制下移除 `budget.enforce (off/soft/hard)`、`daily-token-limit` 与 `pricing` 单价表，替换为 **轻量防失控熔断器（调用/返修次数上限 + Watchdog 超时）+ 只读用量/耗时观测**。
6. **DSH 内使用入口与父模型明确分工**：
   - **主力用法**：直接从 Flywheel 独立左侧栏主工作台发起/审批/控制任务，**零消耗 DSH 对话父模型**；
   - **对话辅助用法**：在 DSH Chat 里讨论需求并调用 `/flywheel-task`、`/flywheel-new` 时，选用 **`dsh-plugin-chatgpt` 的 `GPT-6.2 Sol`（`low` / `medium`）**，走 ChatGPT Pro 订阅（零 DeepSeek 余额扣费、原生支持贴图与工具调用）。
7. **彻底删除旧 `conversation.view` UI 插件与全部前端技术债**：废除现有 8 个拼接纯 JS 文件、`:has([data-composer-seat])` 隐藏输入框 Hack 以及通过 `/flywheel {...}` 斜杠命令污染聊天记录的做法；完全按 `dsh-plugin-github-issues` 标杆以 **TypeScript + TSX + esbuild + DSH 原生 4 插槽 + 人类专属 Direct Remote RPC** 从零重构。

---

## 二、模型特性画像与新版四订阅分工矩阵

综合官方资料、X/Reddit 社区基准（Artificial Analysis Coding Agent Index、FrontierCode 2.0、Terminal-Bench 4.0）、卡兹克多模型协同实践及用户四家实际订阅结构（Google AI Ultra 5x、ChatGPT Pro \$200→\$100、Kimi 年付高档、Claude Pro \$20）：

### 1. 四大模型家族真实性格

| 模型家族 | 核心优势 | 已知短板 | 最佳职责定位 |
|---|---|---|---|
| **Gemini 4**<br>(`Carbon` / `Neon` / `Hydrogen` + `Orchestrator`) | 人类可沟通性 #1、文案与交互洞察 #1；`Carbon` 架构与长程规划极强；`Neon` 为 Agent 编程性价比甜点位；`Orchestrator`（自动规划/Hydrogen 落盘交接）大幅提速降耗；Ultra 5x 额度宽裕；经 `agy acp` 支持原生图片输入 | 粗心、代码分析偶发迎合/幻觉（须配确定性验证或 Codex 交叉核验）；原生 headless 体验较弱 | 架构主脑、双重设计独立脑、波次实施规划、日常主力编码、`complex` Orchestrator、UI 文案/交互复审、只读证据分析、轻量路由 |
| **Claude 5.x**<br>(`Opus 5.5` / `Fable 5.2`) | UI/视觉审美与前端质感断层 #1；原生图片/PDF 视觉诊断极强；长程上下文稳定、步数最省；Agent SDK 优雅 | 偶发漏改或未核实即宣告完成（需 L1 + `git grep` 兜底）；当 Reviewer 易陷“完美主义死循环”；**Pro \$20 额度极紧**；无 OS 沙箱 | **商业 UI / 前端实现、带图多模态设计与实现、Full 架构双重设计对撞与统稿、高危终审可选第二意见** |
| **Codex / GPT-6.2**<br>(`Astra` / `Sol`) | 执行严谨度与精准度 #1；后端/算法/并发/Debug/Browser 自动化顶级；Code Review 检出率碾压（Sol 84%、Astra 96%，Sol 仅 ~1/5 成本逼近 Astra）；**唯一原生支持 OS 级 `read-only` / `workspace-write` 沙箱** | 沟通体验差（高密度天书输出）；UI 审美差；前提错误也会 100% 盲从执行；高 effort 易过度挑刺 | **全部独立 Design/Plan/Code Review、里程碑 Judge、后端/数据/API 实现、Hard 难题攻坚** |
| **Kimi K3.1 Pro**<br>(`kimi-code` Native + Swarm) | 终端/Shell 可靠性顶级（Terminal-Bench 4.0 ~65.7%）；1M 大上下文；原生 `AgentSwarm` 横向并发极强；内置 `WebSearch`/`FetchURL`；已年付高档套餐 | 单步链式思考偏慢；无清晰 Spec 时集成胶水层易缺漏；Swarm 须有明确写入边界 | **明确 Spec 下的多文件批量重构、全仓测试与覆盖率攻坚、大规模迁移、仓库巡检与 `garden` 守卫沉淀** |

### 2. 新版全阶段默认分工矩阵

| 阶段 / 路由 | 新版默认绑定 | 权限 / 沙箱 | 关键变化说明 |
|---|---|---|---|
| **系统设计 · 双重设计主脑** | `gemini-4-carbon-high` (Antigravity ACP) | `full-access`（规划只读校验） | 保留 Carbon 架构强项，ACP 解锁视觉输入与干净协议 |
| **系统设计 · 双重设计对撞与统稿** | `claude-opus-5-5` / high (Claude Pro) | `full-access` | 仅 `Full` 架构级触发一次；带原图设计任务优先由 Claude 主导视觉设计 |
| **系统设计对抗审查 `design-review`** | `gpt-6.2-astra` / high (Codex Pro) | **`read-only`（原生 OS 沙箱）** | 修复旧版审查裸奔在 `full-access` 的能力浪费 |
| **里程碑与波次计划 `plan`** | **`gemini-4-neon-high`** (Antigravity ACP) | `full-access` | **从 Claude Opus 迁出**，保护 \$20 Claude Pro 额度；自动产出并行 Wave 与写入范围 |
| **计划审查 `plan-review`** | `gpt-6.2-sol` / medium (Codex Pro) | **`read-only`（原生 OS 沙箱）** | 快速核验依赖、验收门禁与写入边界 |
| **紧凑设计与联合审查 `compact`** | `gemini-4-hydrogen-medium` + `gpt-6.2-sol` (med) | `full-access` / **`read-only`** | `Quick` 轻量单轮准备 |
| **Execute · `default`** | **`gemini-4-hydrogen-medium`** | `full-access` | `medium` 起步降延迟，失败升级 `high` / `orchestrator-neon` |
| **Execute · `complex`** | **`gemini-pro-orchestrator-neon`** | `full-access` | **扶正双模型 Orchestrator 为默认项**，零人工拆分门槛 |
| **Execute · `frontend` / `multimodal`** | `claude-opus-5-5` / medium + **默认 `gemini-4-neon-medium` 交互/文案复审** | `full-access` | 审美 + 视觉 + 人类可读微文案三位一体 |
| **Execute · `backend`** | `gpt-6.2-sol` / high (Codex Pro) | `workspace-write` / `full-access` | 成本与质量最优平衡点 |
| **Execute · `hard`** | `gpt-6.2-astra` / high（难例升 `xhigh`） | `full-access` | 疑难并发、底层算法、跨仓深重构 |
| **Execute · `batch` / `garden`** | **`kimi-code/k3.1-pro` / high（开启完整 Native Agent / Swarm / Web）** | `full-access` | 全面解禁年付 Kimi 产能 |
| **Code Review（单元 & 终审）** | `gpt-6.2-sol` / high（高危路径自动升 `xhigh` / `astra`） | **`read-only`（原生 OS 沙箱）** | **两档分级（`blocking` / `advisory`）+ 最多 2 轮 + 同 PR 防复发守卫** |
| **里程碑终验 `judge`** | `gpt-6.2-astra` / high | **`read-only`（原生 OS 沙箱）** | 高风险可按需唤起 `Fable 5.2` 第二意见 |
| **调查分析 `investigate`** | `gemini-4-neon-high` (Antigravity ACP, `evidence-only`) | `read-only`（零工具） | 临时空目录运行，绝不污染用户仓库 |
| **智能分流与卡死救援（轻量 JEV）** | **`gemini-4-hydrogen-low`** (Antigravity ACP, `evidence-only`) | `read-only`（零工具） | 压低决策延迟，仅介入首轮修正与连续失败 2 次切策略 |

---

## 三、流程减阻：扫清 7 大高门槛与过度防御

1. **废除手填 2–8 项互斥分工才能开并发的 Swarm 门槛**：Packet 级并行由 `plan`（Neon）自动按依赖分层生成 Wave，Go 自动核验目录互斥并开 Worktree；Packet 内部复杂工程默认用零门槛的 `gemini-pro-orchestrator-neon`；Swarm 仅作为可选草案由 Planner 生成、用户一键确认。
2. **调查“一句话启动”+ 可视化追加轮**：用户只需输入问题现象（可选选定文件/日志），引擎自动规划有界只读探索步骤；追加轮次采用可视化表单，淘汰手写 12 行 `preparation.schema.json` JSON。
3. **带图任务全链路打通**：借助 `claude-code-pro` 与 `agy acp` 的原生图片输入能力，拖拽或选定仓库图片后后台自动校验，设计、执行阶段均可直接消费原图。
4. **Review 两档分级 + 2 轮封顶 + 防复发守卫**：
   - Findings 严格区分为 `blocking`（有明确故障路径：资金、丢数据、越权、误发消息、线上故障、功能未满足验收）与 `advisory`（仅记录不阻断）；
   - 最多 2 轮返修，第 2 轮仍未过直接升级人类在 UI 卡片一键拍板（采纳返修 / 忽略放行 / 接管）；
   - 被真实 Review 问题打回的修复，要求在同一返修中补充回归测试或机械守卫检查。
5. **预算瘦身为安全熔断器**：移除 `budget.enforce`（`soft`/`hard`）、`daily-token-limit` 与 `pricing`；保留单 Packet 执行上限、Review 轮次上限、单任务总调用上限、Watchdog 超时与只读 Metrics 观测。
6. **下沉并发与状态机防御仪式**：`revision / digest / queued / applied / rebase` 等并发保护由 Go 与 UI Store 自动处理，仅在真正不可自动消解的冲突或未知副作用时呈现清晰人话弹窗。
7. **Post-PR 自动交付轨道**：支持在任务创建或拍板时选择交付终点（本地提交 / Draft PR / PR + 自动审查与 CI / 检查全绿自动合并），每一步继续严格绑定候选指纹与 `--match-head-commit`。

---

## 四、Antigravity ACP PR Review 与全部 DSH 跨仓库前置修复清单

### 1. Antigravity ACP 专项 Review（`dsh-subagent-antigravity#1` & `flywheel#63`）

实测本机已安装的官方 `agy 1.5.14`（`agy acp --help`）确认：**`agy acp` 是 Google 官方推荐的标准协议**，原生支持 `--stdio`、`--dangerously-skip-permissions`、`--sandbox`、`--agent`、`--add-dir`、`--process-per-session`、`--login`，并原生支持 **base64 图片输入**、实时 tool/diff/thought 流与标准 JSON-RPC 2.0 生命周期管理。

审阅 [dsh-subagent-antigravity#1](https://github.com/ckanner/dsh-subagent-antigravity/pull/1) 与 [flywheel#63](https://github.com/ckanner/flywheel/pull/63) 后，发现 **6 个必须处理的问题**：

| # | 严重度 | 位置 | 问题描述 | 修复方案 |
|---|---|---|---|---|
| **A1** | 🔴 P0 | `dsh-subagent-antigravity` `assets/acp-client.js:184`（Codex Review P2） | 客户端写入 `acpOperations` 后，若服务端派发前拒绝（如超 1 MiB 文本），错误分支从未清理 `acpOperations`，导致该会话的发送按钮**永久锁死**。 | 派发前对齐服务端校验；收到明确 `not-dispatched` 拒绝时立刻释放操作锁，并提供明确重置出口。 |
| **A2** | 🔴 P0 | `dsh-subagent-antigravity` `lib/acp/sessions.js:406`（Codex Review P2） | `canPrompt` 在异步 workspace/runtime 预检前检查，异步 `await` 期间若收到新的工具/子任务/权限请求或模型切换通知，派发前未复查，导致竞争绕过准入。 | 异步预检完成、写入 operation journal 前重新校验 `canPrompt`。 |
| **A3** | 🔴 P0 | `dsh-subagent-antigravity` `lib/acp/runtime.js:81-83` & `cli-1.5.14.json` | 每次探测都对 230MB `agy` 二进制做全量 SHA-256 并**硬锁死 `1.5.14` 精确哈希**。一旦 `agy` 升级 `1.5.15`，整个 ACP 立即报 `ACP_UNVERIFIED_BINARY` 罢工。 | 默认改为检查 `agy --version >= 1.5.13` + 真实 ACP `initialize` 协议能力校验；精确二进制哈希锁改为显式可选 `strictBinaryHash: true`（默认 `false`）。 |
| **A4** | 🔴 P0 | `dsh-subagent-antigravity` `lib/runner.js:22` & `lib/evidence-agent.js:8-25` | PR #1 未修复遗留 headless Bug：① `executionMode === 'default'` 时传非法空值 `--mode ""`；② `evidence-agent.js` 直接往用户 Git 仓库写 `.agents/agents/dsh-evidence-<runId>.md`，崩溃残留污染仓库且并发 `rmdirSync` 存在竞态。 | ① 仅在 `executionMode !== 'default'` 时传 `--mode`；② `evidence-only` 改在 `mkdtemp` 临时空目录放置受管 agent 并启动 `agy`，对用户 Git 仓库做到**零写入、零污染**。 |
| **A5** | 🔴 P0 | `dsh-subagent-antigravity` `./machine` & `flywheel` `antigravity.go` | PR #1 与 PR #63 **只把 ACP 用在了手动交互桌面 Tab**，Flywheel Go 引擎 `harness: "antigravity"` 仍停在旧 `stream-json` headless（无法看图片、走旧协议）。 | 在 `dsh-subagent-antigravity/machine` 增加默认 **`transport: 'acp'`**（无头自动化透传 `--dangerously-skip-permissions` / `--sandbox`，`evidence-only` 通过 `mkdtemp` + `--agent` + 拒绝全部 `requestPermission` 实现，支持原生图片与实时流），保留 `transport: 'headless'` 作显式兼容选项。 |
| **A6** | 🟡 P1 | `dsh-subagent-antigravity` `assets/acp-client.js` & `flywheel#63` `/flywheel-antigravity` | PR #1 / PR #63 又在 `conversation.view` 挂了一个裸 HTML/无样式的旧式 Tab，与 Flywheel DSH UI v2 四插槽架构冲突。 | Flywheel 不再向 `conversation.view` 挂载旧 Tab；Antigravity 交互会话作为 Flywheel 主工作台/伴随栏的原生能力整合，或由独立插件按需单独启用。 |

### 2. 其余 DSH 生态仓库前置修复清单

| # | 严重度 | 仓库 / 文件 | 问题描述 | 修复方案 |
|---|---|---|---|---|
| **B1** | 🔴 P0 | `flywheel/plugins/dsh/package.json` & `dsh-subagent-claude-code-pro` | `claude-code-pro@0.7.0` 是默认配置核心依赖，但仅为 `optional peerDependency` 未随 Flywheel 打包（开箱即报 `ADAPTER_UNAVAILABLE`）；本地仓库停在 `v0.6.1`（落后 `origin/main` 3 个提交）。 | 本地仓库同步至 `v0.7.0`；将 `dsh-subagent-claude-code-pro@0.7.0` 纳入 Flywheel 打包或受管安装流，实现四订阅真正开箱即用。 |
| **B2** | 🟡 P1 | `dsh-subagent-claude-code-pro` `lib/subscription.js` | 强依赖内部实验 API `usage_EXPERIMENTAL_MAY_CHANGE_DO_NOT_RELY_ON_THIS_API_YET`，且 `fastMode: true` / `fallbackModel` 会直接硬拒。 | 在 `doctor` 中前置清晰诊断与修复建议，并在接口变动时区分“SDK 接口漂移”与“extra usage 已开启”。 |
| **B3** | 🟡 P1 | `dsh-plugin-chatgpt` `src/models/served.ts` | `catalog.json` 已支持 `gpt-6.2-sol` / `gpt-6.2-astra`，但静态兜底表 `SERVED_MODELS` 仍停在 `2026-10-04` 的 6.0/6.1；Flywheel dev fixture 滞后在 `0.1.20`。 | 补齐 `gpt-6.2-sol` / `gpt-6.2-astra` 至 `SERVED_MODELS` 并同步 Flywheel fixture 至 `0.1.23`。 |
| **B4** | 🟡 P1 | `flywheel/plugins/dsh/src/codex-subscription-proxy.js:154` & `dsh-subagent-codex-pro` | 代理层 `child.stderr.on('data', () => {})` 吞掉了全部原生错误，排障困难；本地 `codex-pro` 仓库停在 `review-pr8` 分支。 | 切回 `main`；对脱敏后的 stderr 尾行做有界留存（长期将订阅校验原生合入 `codex-pro/machine`）。 |
| **B5** | 🟡 P1 | `dsh-plugin-alert-triage` `package.json` & `dsh-plugin-lcu` | `alert-triage@0.3.1` 缺少 `^0.2.3-0 || ^0.3.0-0` DSH peer 范围；Flywheel 引用 `lcu@0.5.0` 滞后于 `0.5.1`；多仓库存在 `.agents/` 未跟踪残留目录。 | 补齐 `alert-triage` DSH 0.3 peer 范围；对齐 `lcu@0.5.1`；清理残留 `.agents/`。 |
| **B6** | 🟡 P1 | `flywheel/plugins/dsh/src/subscription-controls.js` & `kimi-native-bridge.js` | Kimi 订阅守卫一刀切禁用 `WebSearch`、`FetchURL`、`Agent` 与 `AgentSwarm`。 | 在核验 `managed:kimi-code` OAuth 且无 `KIMI_*_API_KEY` 覆盖的前提下，默认开放 Kimi 完整订阅能力。 |

---

## 五、Flywheel DSH UI 插件 v2：零技术债完全重构设计

### 1. 前端工程彻底重写（对齐 `dsh-plugin-github-issues`）
- **删除**：旧 `conversation.view` 注册、8 个拼接纯 JS 文件（`client*.js`）、旧 `client.module.css`（及 `:has([data-composer-seat])` hack）。
- **新建**：`plugins/dsh/src/client/` **TypeScript + TSX** 模块化工程（`store/`、`remote/`、`components/`、`views/`、`styles/`、`locales/`），`esbuild` + `lightningcss` 打包输出标准 `window.__ModuleLoader__.load` 懒加载模块，内置 `marked` + `dompurify`，零新增运行时宿主依赖。
- **通信层升级**：宿主暴露**人类 UI 专属 Direct Typert Remote RPC**（不依赖聊天 `sessionId`，不再往聊天流写 `/flywheel {...}` 命令；LLM 工具 `flywheel_control` 仍严禁调用 `approve`/`abandon` 等人工操作）。

### 2. DSH 原生四插槽集成架构

| DSH 公开插槽 | 定位 | 核心体验 |
|---|---|---|
| **`main` + `sidebar.panellist`** | **Flywheel 全屏主工作台** | 左侧导航栏独立入口，不依赖空白会话，完整桌面宽度，零 CSS Hack。 |
| **`sidebar.right.pane.tab`** | **对话右侧伴随监视器** | 在 Chat 中讨论/发指令时，右侧同步显示当前任务阶段条、活跃节点、待拍板提醒，一键跳转主工作台。 |
| **`conversation.chat.commandview`** | **对话内联命令卡片** | 把聊天记录中的 `/flywheel` 输出渲染为紧凑交互卡片，消灭百行原始 JSON。 |
| **`settings.section`** | **DSH 原生设置分区** | 融入 DSH 自身设置体系，彻底终结 5 层导航套娃。 |

### 3. 核心界面与视觉体系

- **工作台壳层**：左侧可折叠 240px 任务轨（项目分组 + 状态呼吸灯 + `+ 新建任务`）+ 右侧单行 52px 极简顶栏（标题、分支/元数据 Click Popover、阶段 Tabs、唯一动态 Primary CTA）。
- **方案对比与拍板台（Dual-Design & Review Studio）**：
  - 支持 **`最终合成方案`** / **`双方案逐条对比矩阵`** / **`上一版 Diff`** 三视图切换；
  - 左侧 62% 精美 Markdown 阅读器（带决策来源徽标 `采纳 Gemini` / `采纳 Claude`）；
  - 右侧 38% 分级 Review Findings（红色 `阻断` / 中性 `建议`），每条支持一键「引用回复」；
  - 底部吸底拍板条（`仅批准` / `批准并开始实施`），免责边界文字全收进 `?` Tooltip。
- **双形态执行视图（波次泳道流水线 + 精致交互式 DAG 图谱）**：
  - **默认 · 波次泳道流水线**：按依赖波次（Wave 1 / Wave 2 / 集成 / 交付）展开，零 6 节点分页限制；每张 Packet 卡片展示 4 层高价值信息（状态与标题、4 段微进度条 `Exec·L1·Review·Git`、厂商色标+模型档位、耗时/失败一行根因）。
  - **切换 · 精致交互式 DAG 图谱**：自动分层拓扑布局（消除死坐标与分页）、244×96px 语义色条浮卡、运行中流动虚线边、**Hover 高亮上下游完整血缘链路（其余节点淡化至 35%）**、Packet 节点支持内联展开 4 步子链、触控板双指/滚轮围绕光标平滑缩放（`0.4x – 1.8x`）、右下角玻璃态悬浮工具条 + Minimap。
- **520px 宽滑出节点抽屉（4 干净 Tab）**：`概览与操作` / `验收与变更`（L1/L2/L3 折叠卡片 + 失败输出高亮）/ `会话接管`（气泡对话流 + 交回按钮）/ `高级与账本`。
- **30 秒新建任务 Modal & 假设调查看板**：三意图卡 + 拖拽/选仓库图片自动校验 + 一句话启动排查；调查界面采用“左假设卡片 × 右证据时间线” + 可视化追加轮表单。
- **交付与指标页 & 三层渐进设置页**：Post-PR 交付轨道 + 阶段耗时甘特条；设置页采用 `生效链路条` → `4 订阅健康灯 + 快捷预设` → `分组角色表` → `独立高级策略与源码`。
- **视觉规范与文案纪律**：100% 使用 DSH `--dsw-alias-*` / `--dsw-radius-*` 令牌（统一 `8px / 12px / 16px` 圆角，以 Layer 背景层差与留白替代满屏 0.5px 边框套娃），全量中英文文案按精简人话标准重写。

---

## 六、分阶段实施计划与验收门禁

| 阶段 | 范围 | 核心交付物 | 独立验收门禁 |
|---|---|---|---|
| **阶段 0：跨仓库前置修复与 Antigravity ACP 升级** | `dsh-subagent-antigravity`、`dsh-subagent-claude-code-pro`、`dsh-plugin-chatgpt`、`dsh-plugin-alert-triage`、`flywheel` 依赖与代理 | 修复 A1–A5（PR #1 两项 P2 + 取消硬锁 1.5.14 哈希 + `runner.js`/`evidence-agent.js` + `./machine` 默认 ACP 含图片支持）；同步并打包 `claude-code-pro@0.7.0`；修复 B3–B6（含 Kimi 解禁） | 各子仓库 `npm run check` / `npm test` / `test:package` 全绿 + 离线 ACP/machine 桥接全绿 |
| **阶段 1：Flywheel Go 引擎重构** | `internal/config`、`internal/contract`、`internal/machine`、`internal/drivers`、`internal/prompts`、`templates/` | 新版四订阅模型矩阵 + Codex 审查 `read-only` 沙箱；Dual-Draft 双重设计；`frontend` 默认 Gemini 文案/交互复审；Review 两档分级与 2 轮封顶 + 防复发守卫；移除美元/Token 硬预算改为安全熔断器；Antigravity 默认走 ACP；降低调查与 Swarm 门槛 | `make quality-gate`（vet + fmt + 100% 覆盖率）+ `plugins/dsh` 测试 + 跨进程桥接测试 + `npm test` |
| **阶段 2：DSH UI 插件 v2 零技术债重构** | `plugins/dsh/src/client/`（全新 TS/TSX）、`plugins/dsh/src/index.js` & `console-read.js`（升级为完整人类 Remote RPC） | 删除旧 `conversation.view` 与拼接 JS；落地 4 插槽架构、Dual-Design 拍板台、波次泳道 + 精致交互式 DAG 图谱、520px 抽屉、30 秒新建 Modal、假设调查看板与三层设置页 | `tsc` + 客户端单测/渲染测试 + `plugins/dsh run check` + `npm run build:npm` + `test:install` + 真实 DSH Desktop 视觉/交互验收 |
| **阶段 3：Post-PR 自动交付轨道与文档收尾** | `internal/machine/delivery.go`、UI 交付轨道、`docs/design.md`、双语指南、`skills/` | Post-PR 交付终点策略与轨道可视化；同步更新中英文文档、设计契约与内嵌技能 | 全量 Go / Node / sessions / candidate install 门禁通过 |
