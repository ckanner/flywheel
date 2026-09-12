# Flywheel（飞轮）— Loop Engineering 系统设计（v0.23 讨论稿）

> 状态：核心已建（2026-08-22：**Go 版全量迁移完成**，`go test ./...` 全绿、覆盖率门禁 100%；Node 版实现随迁移删除，DESIGN 语义不变）。2026-09-10 三厂商 harness 定稿（v0.22）；2026-09-12 分发形态与配置发现链定稿（v0.23，随 npm 0.22.0 落地）。
> Flywheel：编排器如飞轮——把间断、不稳定的 agent 运行，通过磁盘状态储能为连续平稳的推进。
> 本文档是一套多 loop agent 编排系统的完整设计。目标读者：项目维护者本人。
> 设计依据：Addy Osmani《Loop Engineering》、Anthropic/OpenAI/Cursor 官方案例、文档协议驱动 agent 循环的实战经验，以及 2026-08 对 kimi code / pi / oh-my-pi / oh-my-openagent / zcode 五个 harness 和 Kimi K3 / DeepSeek V4 / GLM-5.3 系列模型的调研（来源见附录 A）。
> - v0.23 — **分发形态与配置发现链定稿（用户裁决，2026-09-12，补记——代码先于本文档落地，随 npm 0.22.0 发布；过程违纪教训：变更记录应先行）**：① **分发 = 开发，人人一致**——任何人（含无源码仓库的分发用户）安装用法与维护者完全相同：`npm i -g @ckanner/flywheel`（**平台二进制包模式**，esbuild 先例：四平台交叉编译二进制各自成 npm 包，meta 包经 optionalDependencies 按 os/cpu 自动选取——无 postinstall、无网络下载、无 checksum 步骤，npm 自身包完整性承担校验；仓库可见性不再是分发依赖。原 lark-cli 式"从 GitHub release 下载"因私有仓库匿名 404 而退役，GH release tar.gz 保留为非 npm 直装渠道）。② **安装自包含**：契约 schema、10 个 skills 快照、§13 法定配置表、模板全部 go:embed 内嵌二进制；`flywheel install-skills` 默认从内嵌快照**复制物化**（`.flywheel-snapshot` 标记支持升级刷新；真目录拒绝自动删除的 P1-11 语义保留），`--link` 为开发机源码树软链的显式 opt-in；`flywheel template <name>` 打印内嵌模板（flywheel-new 经它取格式，不再读仓库路径）；仓库定位锚文件（flywheel-home.txt）退役并清理遗留。③ **配置四级发现链**（用户指定架构）：任务目录 `flywheel.config.json` → 项目根 → **全局 `~/.config/flywheel/flywheel.config.json`**（首装从内嵌法定表物化为可编辑的真实文件，用户改全局即改默认）→ 内嵌法定表兜底（HOME 不可写不卡死工具）；`FLYWHEEL_CONFIG` 覆盖缝。④ skills 正文与 compatibility 元数据全量去仓库化（npm 优先安装指引 + 各角色当前 harness 依赖），对外命令面收敛为 `flywheel` CLI + 目标仓库操作（git）+ intake 可选 gh。⑤ 发布工具链：`scripts/build-npm.sh`（交叉编译→注入→五包打包）、`release-preflight.js`（tag = meta = 四平台包版本一致性）。
> - v0.22 — **三厂商 harness 定稿（用户裁决，2026-09-10）**：① **zcode 转正为主链路 Execute default + Intake 的 harness**——经其内置 **app-server 协议**驱动（"ZCode Protocol" stdio；2026-09-10 本机逆向实证：NDJSON 行信封 `{"id","method","params"}`，`session/create` **按会话**指定 `model{providerId,modelId}` + `thoughtLevel`、`session/send` 投递、`state.updated`/telemetry 流式事件——一次性 `--prompt` 入口"全局单模型 + stdout 静默"两大限制就此消解，idle 看门狗语义对流式事件恢复有效）；协议无公开文档，**握手与方法名随桌面自动更新可能漂移**（`--model` flag 失效即前车之鉴）——driver 做防御性握手（失败报清晰错误、绝不静默降级），版本探测进首跑验证清单；默认二进制用 app 内置 runtime `/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs`（与桌面同步更新，第三方 wrapper zcode-app-cli 弃用——vendor runtime 已滞后于 app）。② **Execute default 模型升 GLM-5.3（旗舰跑量）**——依据用户原则"质量/效率/速度优先，成本不计入"（§11.6 记录）：更强 agentic 能力 → 连败与 hard 升级更少；知情代价：订阅配额消耗加速（放量档系数高于旗舰档）、单元时延上升（实测首应答 26s vs flash 12s）。③ **Review + Judge 复核面板 → Codex + GPT-6-Astra**（chatgpt 订阅 OAuth，`codex exec --json` 流式 + `-m` 按次选模型 + usage/缓存明细全暴露；npm CLI ≥0.154 或 app 内置 0.153 二进制——0.151 及更早不支持 gpt-6-astra，实测 400）；跨厂商制衡升级：executor GLM（智谱）↔ reviewer GPT-6（OpenAI）异厂商，§4.3 同族自偏好已知风险消解；hard 路由维持 K3（若 hard 也用 GPT-6 则与 Review 同族，自偏好恰好落在最难单元）。④ **思考档定稿**：zcode `thoughtLevel=max`（GLM 档位 [low,high,max]，5.3 默认即 max，app-server state 实测）；Codex `model_reasoning_effort=max`（GPT-6-Astra 阶梯 low→medium→high→xhigh→max→ultra，默认 medium 故必须显式传；**ultra = max+自动任务委托**，与"单会话只做一件事"纪律冲突，裁决角色不用，留待 Execute 场景评估）；kimi max 不变。⑤ **桌面可见性双轨**：zcode 双触发 `--open-workspace` 注册（v0.20 机制，2026-09-10 复验有效）；Codex 会话原生进桌面统一线程库（`~/.codex/state_5.sqlite`，source=exec）+ 项目注册 `open -a ChatGPT <folder>`（app 声明 Folder 文档类型与 `codex://` scheme）——侧边栏对 exec 线程的即时呈现待 app 重启验证（遗留项）。⑥ **multimodal 路由预留**：GLM-5.3-Flash 为 zcode 目录中唯一多模态档（图像/PDF/视频输入、1M 上下文），routes 表新增 `multimodal` 路由（机制现成，packet 标 route 即走）；Intake 保持 flash 档（与 Execute 不同模型经同一 zcode app-server 驱动，按会话指定，无冲突）。⑦ **omp 退出主链路**（driver 保留为 B 通道：多模态过渡/协议漂移应急）；智谱国内 Coding Plan 随之退居 B 通道，主链路 GLM 配额转 Z.AI Coding Plan（独立订阅池，zcode 用户侧认证）。⑧ 实证遗留：app-server 协议稳定性跟踪、Codex exec 线程桌面呈现、ultra 档语义评估、三家缓存前缀语义首跑复核。
> - v0.21 — **渠道迁移智谱 + GLM 换防（用户裁决，2026-08-27）**：① **OpenCode Go 渠道移除**，非 Kimi 系全部改走**智谱（国内 bigmodel）GLM Coding Plan**（channel `zhipu-coding-plan`，`env-key: null`——凭证留在用户侧 omp provider 配置，编排器**不注入 key**，与 kimi 订阅同款委托模式；无 fallback 语义不变——RATE_LIMIT 同渠道退避、QUOTA 升级 inbox）；② 模型换防：**V4 Flash → GLM-5.3-Flash**（Execute default 路由、Intake），**V4 Pro → GLM-5.3**（Review、Judge 复核面板），slug 前缀 `zhipu-coding-plan/` 与用户 omp provider 名一致；③ **harness 事实同步**：PR #4 已把 pi 系角色切到 **omp**（oh-my-pi，pi 官方分叉，CLI/事件流同构，usage 额外暴露 reasoningTokens/duration/ttft）——§7/§13 两张表此前的 pi 漂移本版一并修正；④ zcode harness 保持可选臂（独立配额池 + 桌面会话可见性，v0.20；模板不预置绑定——启用需手写渠道并把 loops/routes 绑定指到 harness=zcode，模型 slug 走 zcode 自身 zai/ 命名空间；zcode CLI 无 thinking 参数，effort 由其运行时默认）；⑤ **双模型 live 冒烟通过**（2026-08-27，omp 走用户自有凭证，零 env 依赖）：`glm-5.3-flash` 12s / `glm-5.3` 26s 应答，usage 暴露 input/output/reasoningTokens/cacheRead（5.3 探针 cacheRead=3264，缓存生效），`thinking efforts=[low,high,max]` 原生支持全系统 max 档；⑥ 实证遗留：effort 透传的定量验证、缓存前缀语义（§8-5）与编辑格式 A/B 结论（附录 C，V4 Flash × pi 时代数据）对 GLM-5.3 × omp 的适用性均待首跑——编排器以 L1+Review 闸门兜底，不依赖榜单背书。
> - v0.20 — **zcode 桌面可见性（用户裁决"桌面端完整看到会话过程"，2026-08-27）**：zcode headless CLI 会话与 ZCode Desktop 任务索引（`~/.zcode/v2/tasks-index.sqlite`）底层同源但**不实时互通**——本机九组实测确立最终语义：① GUI 按 workspace_key 分组，未注册路径的 workspace 连条目都不显示；② 桌面二进制（非 npm 包装器）解析 `--open-workspace <path>`，**每次处理都幂等注册该路径 + 导入 CLI 存储中归属它的会话**——暖态经 Electron single-instance second-instance 转发 argv 实测同样生效（`open -a` 热运行丢 argv 实测弃用；纯冷启动不带参不扫、`tasks-index.sqlite` 无同步水位表）；这是唯一导入触发器，且**对 key 归一化敏感**：`/tmp/x` 注册成功但永不导入，`/private/tmp/x` 立即导入——driver 用 `filepath.EvalSymlinks` 归一化后该边缘关闭（实测验证）。③ 因此 `RunZcode` **spawn 前后各触发一次**（默认二进制 `/Applications/ZCode.app/Contents/MacOS/ZCode`，env `FLY_ZCODE_BIN` 覆盖）：前触发注册工作区并导入该路径历史会话（覆盖 resume/重试——全新路径此时存储为空，仅注册不导入）；后触发在 watchdog 返回后把本会话（成功全文或失败部分对话）立即导入 GUI。语义边界如实化：执行**进行中**的会话不可见——导入是瞬时快照，桌面无实时流式；单元完成时刻可见性增量累积。降级矩阵：非 macOS / 未装 Desktop / 超时静默跳过，执行语义零改动；测试隔离 `disableDesktopRegistration` 闸 + opener 注入缝。
> - v0.18 — **全量评审修复落地**（2026-08-24，用户裁决"双 judge 做完整版、C7/C9 也修、全部修好出 PR"）：① **里程碑 Judge 升级双 judge 面板**（K3 主判 + V4 Pro 异厂商复核，config `judge-panel`；一致按共识、分歧 → inbox——单 judge 偏差实证见 §4.3/附录 A）；裁决出口契约强化（verdict 必带 score 且 reasons ≥1 条引用可复核证据，缺失按 unparseable 重试）。② Plan 修订 acceptance 指纹留痕（§4.1 已知风险段：planner 可弱化验收的自利风险补录 + 机械缓解）。③ P2×3：兜底成本估算补 CacheWrite 计价（铁律 9）；评审 diff 补 untracked 新文件内容（§4.3"真实 git diff"承诺闭环，greenfield 场景关键）；缺陷注入账本守卫改 ledger 单一信源匹配。④ C 级小修批量：execute 失败路径空转签名用权威 diff、kimi resume 捕获失败告警、pi 错误事件累积保留、jitter 显式 0 可达（`*float64` 指针化）、done-report 多余键拒绝、state.json CAS 加 flock 文件锁闭 TOCTOU、porcelain rename/引号路径解析、judge gate 类型守卫、status 停滞提示消费 LastProgressAt（§6 如实化）。⑤ SKILL.md 规范订正：arguments 改 kimi 支持的字符串数组并接线正文占位符；`/skill:` 调用形态归属订正（§13）；frontmatter 归属按 kimi/pi/Warp/agentskills.io 四方官方文档精确化（AGENTS.md 同步）；附录 A 补 Warp/agentskills.io/kimi·pi skills 文档四来源；flywheel-new 新增验收命令提交前试跑硬规则；skillsembed 门禁扩展 agentskills.io 标准约束。⑥ §4.1 规划职责两段式如实化、§4.4 L2/L3 失败处置回写、§8-4 KIMI_NOW resume 注记、§9 凭证卫生边界声明。评审全文见 docs/reviews/2026-08-24-v0.18-review-fixplan.md。
> - v0.19 — **渠道收敛（用户裁决四条，2026-08-24）**：① Kimi 系模型（Plan/Garden/Judge 主判/frontend/hard 路由）全部只走 **kimi code 订阅**（OAuth，无 key 注入），kimi-coding API 渠道移除；② **全系统取消 fallback provider**——OpenCode Zen 与 kimi-coding 降级链删除，每角色单一渠道；超限语义对齐代码：RATE_LIMIT 同渠道退避重试、QUOTA 不重试直接升级 inbox（机制代码保留为 config 驱动，配置无链即惰性失效）；③ Judge 里程碑终验主判改走 **kimi code harness**（headless `-p` 每次天然全新会话，满足 fresh-context 要求）；④ 双面板复核维持 pi+V4 Pro 不变。附：kimi `-p` 非交互模式**默认即 auto 权限**（官方 flag 冲突规则禁止与 --prompt 组合显式 --auto）；`/swarm` 斜杠命令族在 `-p` 下不被解析（本机 kimi 0.38.0 三探针实证：伪造命令与 /swarm 行为一致），swarm 需 TUI 或上游提供 headless 接口后再评估。
> - v0.17 — **全量 Go 迁移 + 审计修复落地**（2026-08-22，用户裁决"按计划一步步完整实现"）：编排器从 Node.js 单进程移植为 Go（Go 1.27；cobra/yaml.v3/jsonschema v6/testify），包结构 `internal/{errs,config,ledger,childenv,fallback,contract,state,donereport,exitcontract,metrics,circuitbreaker,prompts,verify,watchdog,drivers,machine,defectinjection,modules,skillsembed,cli}` 与 Node 模块一一对应；分发改 goreleaser 单二进制 + npm 安装器（lark-cli 模式）。契约校验从手写校验改为 **JSON Schema 库单一信源**（消灭 schema↔代码漂移；done-report-format 恰好 4 键、must-not-touch 字符串数组、boundaries minProperties:1、spec.title 必填——P2 深度校验缺口关闭）；RE2 无 lookbehind/惰性量词，statusCode/TIMEOUT/frontmatter 三处正则以等价改写+测试锁定语义。**审计修复**：P1 kill switch（RunTask 注册 SIGTERM → 杀子进程组 → quiescence → exit 143，Node 定义未注册）；P1 skills frontmatter YAML 非法（4 处 compatibility 加引号，skill-format-check 由纯正则假绿改为 skillsembed 真 YAML 解析测试强制 whenToUse/arguments/metadata.version）；P2 in-place 模式 planner 修订后 base-ref 重锚；P2 空转检测改用 L1 boundary 权威 git diff（不信 executor 自报）；P2 单元先 commit 成功再标 completed；P3 resume 补缺陷注入采样、stdout 日志与 JSON 分离、install-skills actionText 含 stale、verdict exit 检查查 signal/stderr。100% 单测覆盖率反向驱动可测性：machine 注入 sleep/now/env/log；drivers 以 fake pi/kimi 二进制 + PATH 前置做真实 exec 测试；verify/watchdog 用真实 git 与真实子进程组。
> - v0.16 — **单元级 worktree 隔离**（2026-08-21，用户裁决，关闭 P1-1 WIP caveat）：执行单元在独立 git worktree 中运行（checkout 在仓库旁的 `<repo-parent>/.flywheel-worktrees/<repo>-<slug>-<packet-id>/`——放进 `<repo>/.git/` 会与 git 的 per-worktree 管理目录冲突；分支 `flywheel/<slug>/<packet-id>`，从单元 base-ref 创建）——executor 只见干净工作区，主 checkout 的用户 WIP 不可能被卷入或冲突。L1 与缺陷注入在 worktree 内跑；契约防篡改检查同时盯主 checkout 的 taskDir（executor 知道其绝对路径）。闸门全绿后：worktree 内提交代码 → 主 checkout `ff-merge` → 删除 worktree 与分支 → 编排器在主 checkout 只提交契约/账本文件（限定 `docs/<slug>/`）。done-report 仍由 executor 以绝对路径写回主 checkout 账本区。崩溃恢复复用既有 worktree（路径确定性）；非 git 仓库或 worktree 不可用回退原地执行（原串行模式，脏工作区告警仅在该模式生效）。config `worktree.enabled`（默认 true）。
>   - 已知窄路径（2026-08-23 复审注记，记录不修）：interrupted 单元恢复重执行期间若 planner 常规修订落库，该单元 base-ref 不重锚——修订提交只触 `docs/<slug>/`（账本区），防篡改扫描按 ledger 排除规则忽略之，语义无害；若未来修订提交含账本区外文件，合并侧由 ff-only 失败进 inbox 兜底，绝不产生错误合并。
> - v0.15 — 用户裁决（2026-08-20）：**移除 DeepSeek 直连降级链**（不需要；降级链 = opencode-go → opencode Zen 按量，OpenCode 体系内兜底）；**缺陷注入从可选开关改为常备机制**（无 enabled 开关；缺 patch/test-cmd 配置 = 契约缺陷进 inbox 提示人工配置）；**Plan（kimi CLI）token 不可观测接受现状**（stream-json 无 usage，不为此前置降级 harness）；真实 403 报文验证关闭（不专门处理，模式表误判按 retro 修正）。
> - v0.14 — 全代码设计符合性审计后修订（2026-08-20）：**验收测试改由 Plan Loop 编写（DC1=B，executor 不写/不改任何测试文件）**；verification-gap 一律退回 Plan 加强验收后复验、永不以 gap 完成（DC2）；缺陷注入判 fail → inbox 叫人（DC3）；空转检测语义明确为"错误码+失败命令相同且变更文件 Jaccard ≥ 0.7"（DC4）；缓存命中率逐 provider 阈值报警、soft 警告（DC5）；§9 凭证清单补 KIMI_API_KEY/kimi OAuth（DC6）。kimi code 可用性（含 frontend 路由）经用户确认，不再列为风险项。
> - v0.13 — §11.6 关闭（2026-08-20）：`hard` 升级通道首选 **K3**（V4 Pro 留作该路由降级渠道）。用户确立长期决策原则：**质量/效率/速度优先，成本不计入**。
> - v0.12 — §11.3 关闭（2026-08-20）：Judge（里程碑终验）定为 **Kimi K3**（与 DeepSeek 系 executor 异厂商），经 pi `kimi-coding/k3` 驱动，config 默认已更新。
> - v0.11 — 目标仓库定为 mimir（§11.2 关闭）：首批任务取其头部（S4-0/S0-00/M1-1），替代原"维护型任务"定位（选择理由是调试期变量控制，不是能力限制）；packet 契约新增 `needs-live-credentials` 字段（§5.1，live 凭证任务人工确认后按需注入）；§12 明确冻结点 = 人审契约，不要求源素材预先定稿。
> - v0.10 — 安全模型决策（2026-08-19）：**现阶段不用容器沙箱**，Execute/Review 本机直跑（信任模型同日常 kimi code auto 模式）；测试/boundaries 完整性改为 git diff 事后判定；凭证纪律修正为"executor 环境只注入低价值 executor key"（pi CLI 需要 OPENCODE_API_KEY，原"容器不注入任何 key"不成立）；容器化推迟到接入不可信输入前再评估（§9/§4.2/§4.4/§10/§11.7）。
> - v0.9 — 实证门槛 §10-2 出结果（附录 D）：**OpenCode Go 透传 `reasoning_effort` 确认**（max 档推理量为 low 档 2.4–3.4 倍），Execute/Review/Judge 保留 Go 通道；发现 max 档 usage 缺 `reasoning_tokens` 细目的坑（计量回退规则）；pi L0 冒烟通过。
> - v0.8 — 吸收 deepseek-harness 工程经验（2026-08-18 调研其官方仓库与文档）：编排器新增错误分类与重试策略、流 idle 超时看门狗、出口契约（§6）；state.json 增加 CAS revision、完成状态证据来源记录、interrupted 标记（§5.2）；门槛 §10-2 增加 L0 冒烟与 fail-loud 校验。全文移除 cycle-runner 引用（§1.3 删除）。
> - v0.7 — 设计评审修订（2026-08 全文事实核证后）：state.json 写入权收归编排器（agent 只写 done-report，§5.2）；缺陷注入改为编排器确定性脚本执行的抽检（§4.3）；新增 Plan 长驻会话冷启动重建协议（§4.1）；Composio 引用措辞修正（原"最快、token 最少"与原文数据不符，§4.2）；#1911 状态更新（仍开放、k2.6 复现、API 通道亦中招，§10）；降级链路提为首批建设（§10）；新增开放问题：容器形态选型（§11.7）。
> - v0.6 — 新增 §13 Skills 清单与触发矩阵：一个 skill 触发全部（`/skill:flywheel-run`），每个 loop 各有一个 skill 单独触发；skill 只是按钮，不含 loop 逻辑；Loop×Harness×Model 法定配置表落定。
> - v0.5 — 使用方式定稿（§12）：任务生成改为"问意图、验事实、停下来给人审"；启动/状态/停止全部封装为 harness 内 skill（编排器以独立进程拉起）；支持手动触发指定 loop（--only/--pause-before）；新增执行路由（§4.2）：packet 带 `route` 字段，`frontend` → kimi code + K3，`default` → pi + V4 Flash，`hard` → 升级通道（首选待定，§11.6）。
> - v0.4 — 取消分期实施路线（v0–v3），改为"核心一次建成 + 实证门槛并行 + 外挂模块留槽位"的装配顺序（§10 重写）；全文阶段引用同步清理。
> - v0.3 — 项目定名 Flywheel；**全部模型统一使用最高档 thinking**（K3 max、V4 Flash max、V4 Pro max），reasoning_effort 透传因此从"待实测"升级为硬门槛（§7/§10）；明确系统形态：编排器 = 确定性代码，角色与知识 = skills，契约 = 静态模板（§11.5 关闭）。
> - v0.2 — 按首轮评审意见修订：K3 改 max 起步；预算熔断改为可配置开关（默认 soft）；执行驱动改为 pi CLI 优先；新增执行保真机制（§4.7）与 100% 单测驱动设计纪律（§4.2/§4.4）；补充 worktree 提交语义（§4.2）；编辑格式 A/B 实测见附录 C。

---

## 1. 背景与目标

### 1.1 要解决的问题

把"人逐轮提示 coding agent"替换为"人设计系统、系统驱动 agent"。系统接收**已冻结的需求**，自动完成：规划 → 执行 → 评审 → 分层验证 → 交付，全程可无人值守，异常时升级到人。

### 1.2 明确的非目标

- 不做需求发现与产品判断（伪需求、优先级、取舍是判断类难题，留在 loop 外由人负责）。
- 不做大规模并行 agent 编排（编码任务可并行性有限，并行度由任务可分解性决定，不由 agent 数量决定）。
- 不绑定单一 harness 或模型厂商（角色→模型映射走配置，可替换）。

---

## 2. 设计铁律

以下每条都有实证或官方案例背书，整个设计从它们推导，不得在实现中妥协：

1. **编排器不是 agent**。编排器是一个普通进程（while 循环），读状态 → spawn 会话 → 校验推进 → 决定下一步。它的决策被限制为**可机械检查的状态转移**（state.json 是否推进、退出码是否为 0、重试与预算是否耗尽）。需要智能的判断（计划质量、代码质量、失败归因）委托给专门 agent（Plan/Review）或人——**谁来监督监督者**的问题由此消解：agent 在边缘，确定性在驾驶座。先例：Ralph 是 bash while；oh-my-openagent headless runner 用机械退出条件；Cursor 的 loop 驱动是 harness、judge 才是 agent。
2. **状态全部落盘，交接只走文件和 git**。跨会话、跨 harness、跨模型的唯一可靠介质是外部持久状态。session 是临时的，工作流状态活在 git 里（Gas Town GUPP 原则）。
3. **每个执行单元 = 全新上下文 + 只做一件事**。每轮以干净上下文启动，不依赖会话记忆；记忆在磁盘上（Ralph loop / Cursor scaling agents 的共同结论）。
4. **验证优先确定性 oracle，LLM 评审在其后**。凡是能写成命令的验收必须写成命令：命令给出 ground truth，便宜、瞬时、无法被执行者说服。LLM judge 在代码正确性上双向失效（放行使坏代码、误杀好补丁，arXiv:2604.20202）；LLM 评审只负责命令表达不了的东西（约定、意图、安全）。顺序定死：**命令全绿之后，LLM 才开口**。
5. **做判分离**。执行者与评审者/验证者永远是不同会话；评审者看不到执行者的推理链和自述，只看契约 + git diff + 测试结果。实证：agent 自写测试 80.2% 是弱/无断言（arXiv:2606.18168）。
6. **契约显式字段化并带版本号**。委派契约必须包含：目标、上下文、边界、验收命令、升级路径、产出格式（Anthropic multi-agent 实证：缺这些要素子 agent 即跑偏）。
7. **会话级模型隔离**。任何会话从生到死只绑定一个模型。K3 官方警告不要中途换模型且必须完整回传 thinking 历史；DeepSeek 带 tools 请求必须回传 `reasoning_content` 否则 400。跨角色只传递提炼后的文本产物，绝不共享消息历史。
8. **模型分级看行为特征，不只看跑分**；且 executor 卡壳必须有 escalation 路径，否则 loop 在执行层死循环。
9. **成本必须可观测，熔断可配置**。token/成本记录永远开启（失控 loop 是静默烧钱的，事后至少要能复盘）；预算熔断是开关（`off | soft | hard`，默认 soft：警告不停机）。多 agent 系统 ≈ 15× 聊天的 token 消耗，hard 档留给长时间无人值守场景。
10. **缓存是一等工程指标**。各厂商缓存全是精确前缀匹配；稳定内容前置、易变内容只放末尾消息；编排器逐 provider 记录命中率。

---

## 3. 总体架构

```
┌────────────────────────────────────────────────────────────┐
│                    orchestrator（普通进程）                  │
│  读 state.json → 决定下一单元 → spawn 会话 → 校验推进        │
│  L1/L2/L3 验证执行 / 预算熔断(可配) / 超时看门狗 / 重试 / 升级 │
│  指标：token、成本、缓存命中率、403 计数、推进时间戳           │
└────────────────────────────────────────────────────────────┘
     │ spawn               │ spawn               │ spawn
     ▼                     ▼                     ▼
┌───────────┐      ┌───────────────┐      ┌───────────────┐
│ Plan Loop │      │ Execute Loop  │      │ Review Loop   │
│ kimi code │      │ zcode +       │      │ Codex +       │
│ + Kimi K3 │      │ GLM-5.3       │      │ GPT-6-Astra   │
│ (max)     │      │ (max·新会话)  │      │ (max·新会话)  │
│ 长驻会话   │      │ 每单元新会话   │      │ 每单元新会话   │
└───────────┘      └───────────────┘      └───────────────┘
     │                     │                     │
     └──────────┬──────────┴─────────────────────┘
                ▼
      契约层（git 仓库内的文件，全部已提交）
      spec.md / task-packet-*.md / state.json / retro.md
      + 三层验证环境（L1 确定性 / L2 集成 / L3 端到端）
```

三条不可协商的隔离规则：

- **会话级模型隔离**（铁律 7）：planner / executor / reviewer 各自独立会话体系，交接物只有契约文件。
- **执行与评审会话一次性**：每个执行/评审单元 spawn 全新会话，干完即弃；重试 = 新会话重读契约文件，零恢复成本。
- **评审者信息节食**：评审会话只读契约 + git diff + L1 测试结果，不读 executor transcript（避免被执行者自述带偏）。

---

## 4. Loop 清单

### 4.1 Plan Loop（规划）

| 项 | 内容 |
|---|---|
| 职责 | 两段式（v0.18 如实化）：**初始规划在冻结前完成**——`/skill:flywheel-new` 生成 `spec.md` + task packet 序列并提交人审，冻结点 = 人审契约（§12）；**运行中本 loop 是修订器/加强器**——审计退回的裁决（修订 packet、加强验收）、executor 升级"此路不通"的判定。**范围变更只能由本 loop 做出**（executor 发现需要改范围必须升级，不许自行调整）；随修订**编写/修订验收测试**（executor 不写测试，§4.2） |
| 模型 | Kimi K3（reasoning_effort=**max**，全系统统一最高档 thinking。注意：kimi code 订阅侧默认 high，必须显式配置 max；已知 #1911 报告 K3 max 在 CLI 层有挂起案例，接入验收清单第 3 项必须实测此路径） |
| Harness | kimi code，**长驻会话**（`kimi --session` resume，保缓存保语境；不用每轮新会话） |
| 渠道 | kimi code 订阅（Allegretto ¥199/月起，解锁 1M 上下文） |
| 频率 | 低频：每里程碑 1 次，修订时才再调。频率 = 成本分配：K3 贵（输出 $15/M），只在定方向和重规划时调用；执行的量全由便宜的 Flash 承担 |
| 边界 | system prompt 写死授权范围：只产出契约文件，不实现代码，不扩大需求范围（K3 官方自述"过度主动"，需围栏） |
| 会话重建 | 长驻会话死亡（挂起/7 天过期/断链）时，编排器以 spec.md + state.json + retro.md 冷启动全新 Plan 会话——契约层是唯一记忆，会话本身不承载不可替代的状态 |

需求原子化纪律（防漏做的源头）：spec.md 把需求拆成**编号的、可独立验证的原子项**（R-01…R-n）；每个 task packet 的 `requirements` 字段引用编号，验收命令必须映射到编号（Anthropic feature_list 模式：每项各自 `passes: true/false`，agent 只许改这个字段）。

已知风险（v0.18 补录）：planner 在修订时**有权修改 acceptance 命令本身**，DC2 只约束 verification-gap 必须加强，没有机制阻止其他升级路径上的"减弱"（executor 连败 → return-to-planner → 验收被改弱 → L1 绿 → 单元通过）。现有防线：`PlanRevisions < 1`（只许一次修订，之后 inbox 叫人）。机械缓解（不阻止、留痕可查——范围裁决权归 Plan，铁律 1）：编排器在每次 plan 修订落库后 diff 各 packet 的 acceptance 指纹（L1/L2/L3 命令+expect 的哈希），变化即写 retro 行 + state 警告，人经 `status`/retro 可见。

选型理由：K3 是强制 reasoning 模型，SWE Marathon / Program Bench 官方第一，定位即长程规划；kimi-code 的 plan mode、goal mode（complete/blocked/paused 状态机 + headless 退出码 0/3/6）、`kimi -p --output-format stream-json` 均为脚本驱动的一等接口。订阅限额（每 5h 约 300–1200 次请求、最高并发 30）对低频规划绰绰有余。

### 4.2 Execute Loop（执行）

| 项 | 内容 |
|---|---|
| 模型 | 按 packet 的 `route` 字段路由（见下表）。默认：GLM-5.3（thinking=**max**，全系统统一最高档；智谱 agentic 旗舰跑量——v0.22 依据"质量优先、成本不计入"原则由 flash 档升旗舰，知情代价：配额消耗加速、单元时延上升。编排器不依赖榜单背书——能力边界由 L1+Review 双闸实证兜底） |

**执行路由表**（配置文件定义，非硬编码；每个会话仍只绑一个模型，铁律 7 不受影响）：

| route | 执行者 | 说明 |
|---|---|---|
| `frontend` | kimi code + K3 (max) | 前端需求实现。K3 前端是其强项，限前端任务使用。附带收益：executor 为 K3 时 reviewer 是 GPT-6-Astra，评审与执行跨厂商 |
| `default` | zcode + GLM-5.3 (max) | 默认执行通道，旗舰跑量（v0.22 定稿） |
| `multimodal` | zcode + GLM-5.3-Flash (max) | 带截图/设计稿/PDF/视频输入的任务。Flash 是 zcode 目录中唯一多模态档（图像/PDF/视频输入、1M 上下文），同一 app-server driver 按会话指定模型，与 default 并存无冲突 |
| `hard` | K3 (max)（首选，§11.6 已决） | GLM-5.3 连续卡壳后的升级通道。选 K3 而非 GPT-6：GPT-6-Astra 已任 Review（若 hard 也用 GPT-6 则执行与评审同族，自偏好风险恰好落在最难单元）；K3 与执行器、评审器均异厂商 |

| 项 | 内容 |
| Harness | zcode（app-server 协议驱动，v0.22 转正），**每单元全新会话**，本机直接运行（v0.10 决策：现阶段不用容器沙箱；缓解措施见 §9；接入不可信输入前重新评估容器化）。桌面可见性：spawn 前后双触发 `--open-workspace` 注册（v0.20 机制） |
| 渠道 | Z.AI Coding Plan（凭证在 zcode 用户侧 config/OAuth，编排器**零 key 注入**；配额以订阅档为准；与智谱国内 Coding Plan 为独立订阅池——后者随 omp 退居 B 通道） |
| 驱动方式 | **app-server 协议**：spawn `zcode app-server`（默认二进制为 app 内置 runtime，env `FLY_ZCODE_BIN` 覆盖）→ NDJSON 信封 `session/create`（model + thoughtLevel 按会话指定）→ 应答服务器反向偏好请求 → `session/send` 投递 → 消费 `state.updated`/telemetry 流式事件至会话 idle → 关闭。协议无公开文档：握手失败即报错（绝不静默降级），漂移检测进首跑清单 |
| 升级路径 | 同一 packet 连续失败（见 §6 编排器）→ 回 Plan Loop 修订 → 仍失败 → inbox 叫人；难任务经 `hard` 路由升级 K3 执行（§11.6；kimi code 订阅） |

执行纪律（写进每个 packet 的工作指令）：

- **只实现，不写测试**（v0.14 裁决 DC1=B）：验收测试由 Plan Loop 随 packet 提供（或在仓库已存在）；executor 创建或修改任何测试文件 = 单元判失败（§4.4 git diff 判定）。**难以测试的实现视为设计缺陷**——不可测 = 停止并升级让 Plan 调整设计，而不是自己动手改测试。
- 发现设计与事实不符或需要调整范围：**停止并升级**，不自行扩大/缩小功能。

工作目录与提交语义：

- **单元级 worktree 隔离（v0.16，默认开启）**：每个执行单元在独立 git worktree（checkout 在仓库旁 `<repo-parent>/.flywheel-worktrees/<repo>-<slug>-<packet-id>/`——不能放 `<repo>/.git/` 下，那是 git 的 per-worktree 管理目录；分支 `flywheel/<slug>/<packet-id>`，从单元 base-ref 创建）中运行：executor 只见干净工作区，主 checkout 的用户 WIP 不会被卷入或冲突；崩溃恢复按确定性路径复用既有 worktree。L1/缺陷注入在 worktree 内跑；契约防篡改检查同时盯主 checkout 的 taskDir（executor 知道其绝对路径）。闸门全绿后：worktree 内提交代码（单元粒度可回滚、可审计）→ 主 checkout `ff-merge` 该分支 → 删除 worktree 与分支 → 编排器在主 checkout 只提交契约/账本文件（限定 `docs/<slug>/`，state.json/retro/reports 照旧进 git）。done-report 由 executor 以绝对路径写回主 checkout 的账本区。**Planner 修订**（escalation 或里程碑起步的常规 plan）：契约变更以 contract-only 提交落盘（脏账本不卷入，避免移动 HEAD 破坏 ff-merge），单元 worktree **重建**（旧 checkout 丢弃，executor 按修订后契约重做，base-ref 重新锚定到修订提交）。
- **回退**：非 git 仓库或 worktree 不可用（config `worktree.enabled: false`）→ 原地执行（原串行模式）：单一 checkout，单元结束时提交一次；该模式下启动前未提交 WIP 仍会被单元提交卷入，编排器在 fallback 时发脏工作区告警。
- **契约层文件全部是已提交内容**（spec、packet、state）：并行期"先提交契约再 spawn"是硬规则；executor 的 worktree 从任务分支创建，编排器在闸门通过后合并。

实测支撑（**V4 Flash × pi 时代的历史参照**，GLM-5.3 × omp 换防后不直接适用——现行能力边界按 §4.2 由 L1+Review 双闸实证兜底）：Composio 30 任务跨 harness 实测（SaaS 工作流任务，非编码基准），V4 Flash 在 pi 上通过率第一（66.7%）、单次成功成本最低（$0.028）；编码场景的独立证据见 Databricks 基准（同模型同 effort 下 harness 间成本差 2×、质量持平，简单 harness 如 pi 表现最佳，附录 A）。

**编辑格式实测**：V4 Flash × pi 的 hashline vs 默认 edit A/B 已跑（附录 C），结论已纳入：**Execute Loop 默认使用 pi 原生编辑协议，不引入 hashline**。注意：该 A/B 是 V4 Flash × pi 时代数据，而"编辑格式 × 模型有交互效应，不能默认"——换防本身同样受此约束，GLM-5.3 × omp 的适用性列入 v0.21 ⑥ 实证遗留，首跑复核。背景证据：can1357 基准中 DeepSeek 系是唯一锚点编辑反而更差的模型。

### 4.3 Review Loop（评审）

| 项 | 内容 |
|---|---|
| 职责 | 对执行单元的产出做 LLM 评审：代码质量、项目约定遵守、设计意图符合度、安全隐患、**测试断言质量**（覆盖率只证明"执行过"，不证明"断言有效"）。**单测测不出来的东西归它** |
| 输入 | task packet + git diff + L1 测试结果（不含 executor transcript） |
| 模型 | GPT-6-Astra（thinking=**max**；OpenAI 旗舰，agentic 基准领跑者——BenchLM 综合分 #2/231、AutomationBench 69% 领先；判断密度最高的环节配最强模型。**跨厂商**：与 GLM 执行器异厂商，v0.21 时代"executor/reviewer 同族"的已知风险（§4.3 旧版记录）就此消解） |
| Harness | Codex（`codex exec --json`，每单元全新会话），rubric 打分（0.0–1.0 + pass/fail，单 judge；**里程碑终验为双 judge 面板，§4.4/§7**）。桌面可见性：会话原生进 ChatGPT 桌面线程库 + spawn 前后 `open -a ChatGPT <folder>` 项目注册 |
| 渠道 | chatgpt 订阅（OAuth 用户侧登录，编排器零 key 注入）——npm CLI 需 ≥0.154 或用 app 内置二进制（0.153+，与桌面同步），旧版对 gpt-6-astra 报 400 |
| 结论 | pass / 退回（附具体修改要求、失败证据、涉及位置、完成条件） |

特殊职责：

- **覆盖矩阵检查**（防漏做）：spec 的每个编号需求 → 至少一条验收 + 证据，缺一条即退回。
- **范围对照**（防改做）：git diff 对照 packet 的 `boundaries`，越界即退回。
- **缺陷注入抽检**（防假测试）：**常备机制，无开关**（v0.15 裁决）。按抽检触发（默认每 5 个执行单元 1 次），在 Review 之前由**编排器用确定性脚本执行**（patch 注入 → 跑测试确认变红 → 按 patch 文件清单精确还原并校验工作区干净），结果喂给 Review，reviewer 只读结果做判断；**注入判 fail（测试不变红 = 假测试实锤）→ inbox 叫人**（验收体系失效不是重试能解决的）。`patch`/`test-cmd` 随任务契约配置（仓库相关，无通用默认），采样命中但缺配置 = 契约缺陷，进 inbox 提示人工配置。纯验证缺口不退回执行轮：**退回 Plan 加强验收后复验，永不以 gap 裁决完成单元**（v0.14 DC2/DC3）。工程化形态可演进为 Stryker 类变异测试。

已知风险：v0.21 时代 executor（GLM-flash）与 reviewer（GLM-5.3）同厂商同家族的自偏好风险，**v0.22 随 reviewer 换防 GPT-6-Astra 消解**（default/multimodal 路由的执行器均为 GLM 系，frontend/hard 为 K3，与 OpenAI 评审全部异厂商）。缓解层级保留：确定性验证永远先行（L1 不过轮不到它说话）+ 全新上下文 + rubric 显式打分。**单 judge 的系统性偏差有实证**（2025 研究：position bias / self-preference / verbosity bias，单 judge 与 ground truth 一致率约 87.6%，bias-aware 多模型面板可达 98.1%——Verdict-Bench/Jury、IJCNLP 2025 position bias 系统研究、EMNLP 2025 DBG）。据此 v0.18：① 裁决出口契约强化——verdict 必须带 score 且 reasons ≥1 条、每条引用可复核证据（命令+退出码或文件:行号），缺失按 unparseable 重试；② **里程碑 Judge 升级为双 judge 面板**（K3 主判 + GPT-6-Astra 复核，config `judge-panel`，一致则按共识裁决，分歧 → inbox 叫人）；单元级 Review 维持单 judge（L1 先行 + 缺陷注入兜底已覆盖其大部分风险面）。

### 4.4 验证体系（不是 loop，是编排器内建的三层闸门）

| 层级 | 内容 | 执行者 |
|---|---|---|
| L1 快速确定性 | build、lint、typecheck、unit tests、**覆盖率闸门** | 编排器直接跑（不是 agent） |
| L2 集成 | 真实数据库/服务的集成测试、契约测试 | 编排器触发，隔离环境（形态届时定，§11.7） |
| L3 端到端 | 起真实服务，浏览器自动化/API 客户端走真实用户路径 | 编排器触发，执行者无权改测试 |

流程：**Execute → L1 → Review → （里程碑关闭前）L2/L3**。

- L1 结果作为 Review 输入。
- **覆盖率闸门**：变更/新增代码 100%（行+分支）；存量代码用 ratchet（覆盖率只许升不许降）。注意 100% 是必要不充分条件——它度量"执行过"而非"断言有效"，断言质量由 Review Loop 与缺陷注入兜底（§4.3）。
- **测试与边界完整性用 git diff 事后判定**（v0.10）：测试文件（新增或修改均算；executor 不写测试，§4.2）或 packet `must-not-touch` 路径出现在执行单元 diff 中 = 单元直接判失败。确定性检查，不需要容器挂载/权限钩子的事前拦截。
- 依据：Anthropic 实证"curl 式浅层验证"是第三大失败模式，浏览器自动化端到端验证才解决；验证必须在执行者控制之外。
- **L2/L3 失败的处置（v0.18 回写实现语义）**：里程碑闸门任一层红 → 任务升级 inbox 叫人，不重试、不回 Plan——闸门级失败多为环境/集成/契约问题，自动重试只是烧钱。这是有意选择的保守路径，不是遗漏。

### 4.5 Intake Loop（发现，可选，外挂模块）
定时扫 issue 跟踪器 / CI 失败 / 依赖升级 → 候选任务进 triage inbox → **人确认后才冻结成需求进入 Plan Loop**。模型：GLM-5.3-Flash（zcode app-server 驱动，与 Execute 同 harness 不同模型——按会话指定，无冲突）。这是判断类难题的人机分界线。

### 4.6 Garden Loop（记忆维护，可选，外挂模块）

低频（周级）清理 AGENTS.md/skills 腐烂内容；把 retro.md 反复出现的失败模式提炼成 skills。改的是 loop 自己的知识资产，需人审。

### 4.7 执行保真机制（防漏做 / 防改做 / 防假完成）

agent 执行的三联症，三层各防一个，前面各处已分布定义，此处汇总索引：

| 病症 | 机制 | 位置 |
|---|---|---|
| 漏做（omission） | 需求原子化编号 + packet `requirements` 映射 + 里程碑逐项核验 + Review 覆盖矩阵检查 | §4.1 / §4.3 / §5.1 |
| 改做（scope drift） | `boundaries` 边界 + Review 范围对照 + **范围变更只能 Plan Loop 做出** + retro 记录每次变更 | §4.1 / §4.3 / §5.1 |
| 假完成（false done） | 执行者从不宣布完成；完成 = 编排器跑验收通过 + Review verdict + 里程碑 L2/L3；测试只读；缺陷注入抽检 | §4.2 / §4.3 / §4.4 |

---

## 5. 契约层（系统的心脏）

全部进 git、全部已提交、全部带 schema 与版本号。

### 5.1 `task-packet-<id>.md`（Plan → Execute/Review 的交接）

```yaml
packet-id: M02-T03
contract-version: 1
goal: <一句话：此单元要达成的可观察结果>
requirements: [R-07, R-08]           # 映射 spec.md 的编号需求（防漏做）
route: default                       # 执行路由：frontend | default | multimodal | hard（见 §4.2 路由表）
needs-live-credentials: false        # true = 需 kubeconfig/密钥等 live 凭证：人工确认后按需注入（每单元一次）；默认 false（见 §9 凭证卫生）
context-files: [src/auth/*.ts, docs/spec.md#M-02]
cache-scope: [docs/spec.md]          # 进稳定前缀的文件（见 §8）
boundaries: {must-not-touch: [src/payments/], max-files: 6}   # 防改做
acceptance:                          # 编排器直接执行的确定性验收
  L1: [{cmd: "pnpm test auth", expect: "exit 0"},
       {cmd: "pnpm lint", expect: "exit 0"},
       {cmd: "pnpm coverage --changed --min 100", expect: "exit 0"}]
  L2: [{cmd: "pnpm test:integration auth", expect: "exit 0"}]
  L3: [{script: "e2e/auth-flow.ts", expect: "exit 0"}]
review-rubric: <Review Loop 使用的评审要点与 pass 线>
escalation: {max-retries: 3, on-fail: return-to-planner}
done-report-format: {requirements-covered: [], changed-files: [], commands-run: [], failures: []}
```

### 5.2 `state.json`（编排器专用，唯一信源）

当前里程碑、轮次计数、每 packet 重试次数、token/成本累计、缓存命中率、上次推进时间戳。**state.json 只能由编排器写入**：agent 每轮结束只写 done-report（做了什么、哪条命令过/没过），编排器校验"本轮是否合法推进"后自行更新状态——被监督者不得写监督者的账本（铁律 5 的延伸），未推进则重试或升级。状态更新带 revision 号做 compare-and-set（防编排器崩溃重启后基于陈旧状态决策）；每个"完成"状态记录证据来源（L1 通过 / Review 通过 / executor 自报，三者证据等级不同）；编排器重启后，"进行中"单元显式标记 `interrupted` 并保留已有产物，不静默重置为 pending。人可读的散文状态（设计文档头部、retro.md）另存，沿用成熟设计（里程碑、reopen 计数、收敛机制保留）。

### 5.3 `spec.md` / `retro.md`

计划与人可读运行记录。spec.md 内含编号需求表（R-01…R-n，每项可独立验证）与需求→里程碑映射。retro.md 只记可核对事实：轮次、结论、缺陷数、reopen 次数、**范围变更次数与原因**、预算消耗、人工介入次数（正常应为 0）、改进建议（≤3 条）。

---

## 6. 编排器设计（刻意做薄）

Go 单二进制（CLI 驱动，cobra；v0.17 起全量迁移，Node 实现已删除），核心状态机：

```
for milestone in spec.milestones:
    packet = plan_loop.issue(milestone)          # 低频 K3
    for attempt in range(packet.escalation.max_retries):
        result = execute_loop.run(packet)        # 新 pi 会话 + 看门狗超时
        l1 = verify.run(packet.acceptance.L1)    # 编排器直接跑
        verdict = review_loop.review(packet, result, l1) if l1.pass else fail(l1)
        if verdict.pass and verify.milestone_gate(milestone):  # L2/L3
            break
        if verdict.kind == "验证缺口": 加强验收后复验  # 不退回执行
    else:
        escalate(result)                          # 回 Plan Loop 修订 / inbox 叫人
```

关键行为：

- **看门狗**：双计时——**流 idle 超时**（默认 300s，对每次输出活动计时、有活动即重置：长任务不误杀，#1911 类静默挂起 5 分钟内发现）+ 整体 wall-clock 超时兜底；进程级 kill 覆盖整个子进程组，kill 后必须 await 到真正退出（quiescence）再重试或标记失败，防孤儿进程写坏工作区（pi 无内置步数上限；oh-my-pi #8272 与 kimi code #1911 均有 headless 挂起案例——超时必须在编排器侧，看门狗是必需品不是保险）。断链/停滞发现（v0.18 如实化）：cron 短进程模式下主通道是 pid 文件陈旧 + 进程不存在；`state.json` 的上次推进时间戳由 `flywheel status` 消费——距今超过 wall-clock 阈值即显式提示"疑似停滞/断链"；Plan 长驻会话断链后按 §4.1 会话重建协议冷启动。
- **错误分类与重试**：spawn 失败归一为稳定错误码（借鉴 DeepSeek 官方 deepseek-harness 的适配器契约）：`AUTH / QUOTA / RATE_LIMIT / SERVER / TRANSPORT / TIMEOUT / EMPTY_RESPONSE / INVALID_REQUEST / CONTEXT_WINDOW_EXCEEDED`。默认只重试 `EMPTY_RESPONSE / RATE_LIMIT / SERVER / TIMEOUT / TRANSPORT`（指数退避 500ms→10s + 10% jitter，尊重 provider 的 Retry-After）；`AUTH / QUOTA / INVALID_REQUEST / CONTEXT_WINDOW_EXCEEDED` 不重试，直接升级路径。**空产出（exit 0 但无交付物）映射为可重试的 `EMPTY_RESPONSE`，不是静默成功**。每次重试的错误码、策略、计划延迟落进 state.json 可审计。
- **出口契约**：单元成功的判定三者独立——进程终态正常 + stderr 无错误 + 交付物（done-report / 变更）存在；`timedOut / signal / exitCode` 正交记录进 state.json，"超时被 trap 成 exit 0"不得读成绿灯。
- **熔断（可配置）**：`budget.enforce: off | soft | hard`，默认 soft（警告并记录，不停机）；hard 档为日 token 预算硬顶。空转检测（同一 packet 连续失败且**错误码+失败命令相同、变更文件 Jaccard 相似度 ≥ 0.7**，v0.14 DC4）在任何档位下都触发升级——空转检测不是钱的问题，是 loop 健康的问题。token/成本记录在任何档位下都不关闭。
- **并发**：串行期不设限；引入并行模块时用文件锁原子 claim 任务，不做速率限制。
- **降级（v0.19 移除）**：用户裁决取消全部 fallback provider——每角色单一订阅渠道，配置中无 `fallback` 链。超限语义（**用户裁决 A，2026-08-24**）：RATE_LIMIT 同渠道退避重试（尊重 Retry-After），耗尽升级 inbox；QUOTA 不重试直接升级（订阅超额恢复以小时计，退避无意义；备选的“QUOTA 也退避”已否决）；AUTH/INVALID_REQUEST/CONTEXT_WINDOW_EXCEEDED 照旧直接升级。机制代码保留为 config 驱动——将来若要恢复备用 provider，加回渠道绑定即可，无需改代码。
- **升级路径**：executor 连续卡壳 → Plan Loop 修订 packet → 仍失败 → inbox 叫人。GLM-5.3 旗舰跑量的能力上限待首跑实证（V4 Flash 时代参照：DeepSWE 54.4 vs Pro 62.7 vs K3 67.5，留档），难任务经 `hard` 路由升级 K3 执行（§11.6 已决；kimi code 订阅）。
- **可观测**：每次 spawn 记录模型、harness、prompt 哈希、token、成本、缓存命中率、耗时、退出码。用途：① 失败归因（跨 harness 没有现成方法学，全靠这张表）；② 成本可见（DeepSeek 8-17 涨价后重算经济性）；③ 缓存命中率作健康信号（突降 = 前缀被破坏）；④ 协议改进的证据源（原则：改进必须基于多次运行复盘证据）；⑤ 停滞检测。**指标是人不盯着 loop 也能监督它的方式。**

---

## 7. 模型与渠道矩阵

| Loop | 模型 | Harness | 渠道 | 频率 | 备注 |
|---|---|---|---|---|---|
| Plan | Kimi K3 (max) | kimi code 长驻 | kimi code 订阅 ¥199/月 | 低 | 订阅侧默认 high，需显式配 max；stream-json 无 usage，token 暂不可观测（v0.15 接受现状） |
| Execute（default） | GLM-5.3 (max) | zcode app-server 新会话 | Z.AI Coding Plan | 高 | v0.22 旗舰跑量（质量优先原则）+ harness 转正 |
| Execute（multimodal） | GLM-5.3-Flash (max) | zcode app-server 新会话 | Z.AI Coding Plan | 按需 | 唯一多模态档（图像/PDF/视频，1M 上下文） |
| Execute（frontend） | Kimi K3 (max) | kimi code | kimi code 订阅 | 中 | 前端需求实现；K3 前端最强；路由见 §4.2 |
| Execute（hard） | Kimi K3 (max) | kimi code | kimi code 订阅 | 低 | 维持 K3：GPT-6 已任 Review，hard 若同用则与评审同族 |
| Review | GPT-6-Astra (max) | Codex 新会话 | chatgpt 订阅 | 中 | v0.22 由 GLM-5.3 换防；与全部执行器异厂商；ultra 档不用（自动委托与裁决纪律冲突） |
| Judge 终验·主判 | Kimi K3 (max) | **kimi code**（headless `-p` 每次全新会话 = 天然 fresh context） | kimi code 订阅 | 低 | v0.19 改 kimi harness；异厂商互证由双面板复核承接 |
| Judge 双面板·复核 | GPT-6-Astra (max) | Codex 新会话 | chatgpt 订阅 | 低 | 与主判异厂商；一致按共识、分歧 → inbox（v0.18 面板；v0.22 复核由 GLM-5.3 换防） |
| Intake | GLM-5.3-Flash (max) | zcode app-server | Z.AI Coding Plan | 定时 | 近乎零成本 |
| Garden | Kimi K3 (max) | kimi code | kimi code 订阅 | 周级 | 知识资产维护提案 |

> **统一 max 档**：全部模型 thinking 固定最高档，传法按 harness 定（v0.22 定稿）：zcode 走 `session/create` 的 `thoughtLevel="max"`（GLM 档位 [low,high,max]，5.3 默认即 max——app-server state 实测）；Codex 走 `-c model_reasoning_effort="max"`（GPT-6-Astra 阶梯 low→medium→high→xhigh→max→ultra，**默认 medium 故必须显式传**；ultra = max+自动任务委托，与"单会话只做一件事"纪律冲突，裁决角色不用，留待 Execute 场景评估）；kimi code 走每调用 env `KIMI_MODEL_THINKING_EFFORT=max`（不碰用户全局配置）；omp B 通道走 `--thinking max`。代价是输出 token 增加，成本影响可接受（用户原则：质量优先，成本不计入）；收益是每一环的推理质量拉满。"thinking 模式下 temperature/top_p 不生效"系 DeepSeek 时代实测行为，GLM/GPT 通道待首跑复核。

渠道事实与风险：

- **Z.AI Coding Plan**（v0.22 起主链路 GLM 渠道，经 zcode）：订阅制、独立配额池（与智谱国内 Coding Plan 互不相干）；**凭证委托用户侧**——zcode 用户侧认证（provider `zai`，Anthropic 兼容端点 `api.z.ai/api/anthropic`，key 或 Z.AI OAuth），编排器零注入；模型 slug 走 zcode 自身命名空间（`zai/glm-5.3[-flash]`）。app-server 协议驱动已实测（2026-09-10：create/send/流式事件/按会话模型+思考档全通）。风险：① 协议无公开文档，随桌面自动更新可能漂移（`--model` flag 已实证失效过）——driver 防御性握手 + 首跑版本探测；② 旗舰档跑量配额消耗快（放量档系数更高）——RATE_LIMIT 退避 / QUOTA 升级 inbox 语义不变；③ 自动化条款灰区同 kimi，用户知情裁决。
- **chatgpt 订阅**（v0.22 起 Review/复核面板渠道，经 Codex）：OAuth 用户侧登录（`codex login`），编排器零注入；`codex exec --json` 流式 + usage/缓存明细全暴露。**版本硬门槛**：gpt-6-astra 需 CLI ≥0.153（app 内置）或 ≥0.154（npm）——旧版 400"requires a newer version of Codex"。模型阶梯：gpt-6-astra（旗舰，272K）/ gpt-5.6-sol·terra（工作马）/ luna·reserve（快省）/ codex-spark（超快 128K）。风险：代理网络偶发 403（实测经 HKG 节点被拦，重试即过）；订阅配额以档位为准。
- **智谱（国内 bigmodel）GLM Coding Plan**（v0.22 起退居 B 通道，经 omp）：凭证委托 omp 用户侧 provider（`zhipu-coding-plan`），`env-key: null` 不注入；多模态过渡与 zcode 协议漂移应急时启用；live 冒烟 2026-08-27 已过。
- **DeepSeek 直连**（信息留档，v0.15 起不采用；v0.19 连 Zen 兜底一并移除）：2026-08-17 起峰谷定价。V4 Pro 峰值 $1.32/$3.96、谷值 $0.66/$1.98（北京时间 9-12、14-18 为峰）；缓存命中/未命中比从 1/120 缩至 1/30。若将来启用需在 pi 自配 custom provider + `DEEPSEEK_API_KEY`。
- **Kimi 订阅**：K3 需 Moderato ¥99（256K）、1M 需 Allegretto ¥199；所有会员功能共享 token 额度池 + Kimi Code 专属 5h/周窗口。`kimi login` 为 device-code OAuth，`kimi -p` 官方支持 CI。条款灰区：用户服务协议有"自动化脚本"条款——v0.19 起 kimi 侧无按量兜底，灰区风险以退避+升级消化，用户知情裁决。headless `-p` 默认 auto 权限（显式 `--auto` 与 `--prompt` 组合会被 CLI 拒绝）；`/swarm` 命令族仅 TUI 可用、`-p` 下不解析（kimi 0.38.0 三探针实证），swarm 并行能力待上游提供 headless 接口后再评估。
- **成本量级预估**：串行 + 分级 + 确定性验收的组合下，单任务总价应在 Ralph 的 $297 案例量级，而非 Carlini 的 $20,000 量级。杠杆排序：模型分级（省 40–60%）> prompt 缓存 > 重活隔离进子会话。
---

## 8. 缓存工程

各厂商缓存全是**精确前缀匹配**，loop 负载（每轮重复压入相同契约文件）使命中率成为最大账单杠杆。纪律如下：

1. **稳定内容前置**：spec/契约文件放前缀稳定段（system prompt 之后、首条 user 消息之前的固定位），`cache-scope` 字段显式声明。
2. **易变内容后置**：时间戳、轮次号、随机 ID、当日日期一律放最后一条消息（Kimi 团队公开建议同此）。
3. **每模型独立缓存**：缓存 model-scoped，planner/executor/reviewer 互不可享（隔离规则天然满足）。
4. **Plan Loop 长驻会话**：kimi code 的 system prompt 含 `KIMI_NOW` 会话级时间戳，跨会话前缀从开头就不同；长驻会话让规划语境和缓存同时保住。注意（v0.18 补录）：KIMI_NOW 在每次 resume 时刷新（kimi #446，PR #498 修复）——resume 后 system 前缀变化，跨 resume 的前缀缓存收益从 system 段起失效，同会话多轮内不受影响。
5. **前缀单元语义**（DeepSeek 时代实测，GLM 通道待首跑复核）：新前缀前两次必 miss，第三次起命中；公共前缀要等系统检测落盘。执行单元虽是新会话，但只要 spec 前缀逐字节不变，服务端缓存仍跨会话命中。
6. **compaction 是最大单点缓存杀手**：执行单元设计为单会话内可完成（小而独立），尽量避免触发 compaction；plan 长驻会话压缩时把摘要放尾部。
7. **命中率进指标**：编排器逐 provider 记录 `prompt_cache_hit_tokens/miss_tokens`，命中率低于阈值报警（实测参照：同会话 ~98% 为正常，跨会话场景是各家差异点）。

---

## 9. 安全与合规
- **执行隔离（v0.16 落地 worktree，v0.10 决策无容器维持）**：Execute 在单元级 git worktree 中本机运行（§4.2）；无容器沙箱，信任模型同日常 kimi code auto 模式。缓解措施：worktree 隔离 + 每单元 git 提交（可回滚）+ git diff 事后判定测试/boundaries 完整性（§4.4）+ 进程组 kill 到 quiescence（§6）。**触发重评**：接入不可信输入（Intake 外部 issue、依赖引入陌生代码、涉网任务）前必须重新评估容器化。

- **凭证卫生**：executor 环境只注入任务所需的最低价值 key——v0.22 起主链路三渠道全部**零 key 注入**：zcode 通道（Execute default/multimodal/Intake）凭证委托 Z.AI 用户侧认证（`~/.zcode/cli/config.json` 的 provider key 或 OAuth）；Codex 通道（Review/复核面板）走 chatgpt 订阅 OAuth（`codex login`，用户本机已登录）；Kimi 系环节（Plan/Garden/Judge 主判/frontend/hard 路由）走 kimi code 订阅 OAuth（v0.19 起）。omp B 通道同款委托（智谱凭证留用户侧 provider，`env-key: null`）。`needs-live-credentials` 任务经人工批准后按名单注入。一律无 SSH agent、无 git 推送凭证；编排器持有其余全部凭证。**边界声明（v0.18）**：白名单环境含 `HOME`/`XDG_*`（CLI 与测试运行器必需），因此 executor 及其测试代码**技术上可达** `~/.zcode/`、`~/.codex/auth.json`、`~/.kimi-code` 等本机 auth store——本机制是**防事故性继承**（任何 API key 都不进子进程环境），不是防恶意；防恶意属于容器化重评（§11.7）的触发范围，接入不可信输入前不得依赖本条。
- **kill switch**：编排器单点可停；预算 hard 档是长时间无人值守时的最后一道闸。
- **合规灰区台账**：智谱 GLM Coding Plan / Kimi 订阅的自动化条款风险记录在案（v0.19 起无降级链兜底——灰区以同渠道退避+升级消化，用户知情裁决；OpenCode Go v0.21 移除、DeepSeek 直连 v0.15 已裁决不采用）。

---

## 10. 建设顺序（一次建设，按最终架构）

不分实施阶段、不留过渡方案。核心系统一次建到位，外挂模块随时插入——**"先骨架后模块"不是分期付款，是装配顺序**。

**核心建设（一次完成，即最终形态）**：

- 编排器（读状态 → spawn → 校验 → 推进；看门狗、指标、软熔断、降级）
- 契约层模板（task packet / spec / state.json schema）
- 三个主 loop 的接入：Plan（kimi code + K3 max 长驻）、Execute（zcode app-server + GLM-5.3 max，本机直跑）、Review（Codex + GPT-6-Astra max）
- L1 验证（含覆盖率闸门）与 done-report 解析
- skills 族：任务生成器、角色工作指令
- 首批任务：**目标仓库 mimir**（§11.2 已决），取其头部任务（S4-0 关闭 ADR / S0-00 spike 脚手架 / M1-1 CI 门禁）经 `/skill:flywheel-new` 生成契约——需求稳定、验收命令现成、无需 live 凭证、单单元可完成；选择理由是调试期变量控制，不是能力限制

**实证门槛（与建设并行，结果会改写渠道/harness 配置，必须在硬编码前出结果）**：

1. ~~V4 Flash × pi 编辑格式 A/B~~（已跑，结果见附录 C：用 pi 原生编辑协议）。
2. ~~OpenCode Go 是否透传 `reasoning_effort`（硬门槛）~~（**已实测通过**，2026-08-18，数据见附录 D）：max 档推理文本量为 low 档的 2.4–3.4 倍，透传确认；Execute/Review/Judge 保留 OpenCode Go 通道（显式传 max）。两个实施注意：① max 档下 Go 返回的 usage 缺 `reasoning_tokens` 细目（low 档有），编排器计量不得假设该字段恒在，缺失时以 completion tokens + 响应推理文本量回退；② pi 侧 L0 冒烟已通过（`--thinking max` 下 reasoning 内容真实到达，usage 暴露 `reasoning`/`cacheRead`/`cacheWrite` 分项，满足指标采集）。通道启用时仍按 fail-loud 原则：不支持 max 就报错，不 clamp、不静默降级。
3. kimi code 长驻会话在 1M 上下文 + **max 档**下的稳定性（#1911 状态更新，2026-08 核证：仍开放、无 workaround；k2.6 亦复现，挂起疑在 CLI 请求层而非模型层；原始报告走 API key 通道——按量直连未必能绕开。实测结论直接影响 Plan Loop 可用性与 §4.1 会话重建协议的触发频率预期）。
4. ~~DeepSeek 直连降级链路演练（403 注入测试）~~（403 演练已完成；v0.15 裁决直连不采用，链路移除）。
5. **zcode app-server 协议稳定性跟踪（v0.22 新增，持续性门槛）**：协议无公开文档，随桌面自动更新可能漂移（`--model` flag 对当前 runtime 已实证失效）。driver 防御性握手（schema 校验失败即报清晰错误）；每次桌面大版本更新后跑协议探针（create/send/事件流三步）确认；漂移时 B 通道 omp 应急。
6. **Codex 首跑验证清单（v0.22 新增）**：① CLI 版本 ≥0.154（npm）或用 app 内置二进制（gpt-6-astra 硬门槛）；② exec 线程在 ChatGPT 桌面侧边栏的呈现语义（注册已验证，即时呈现待 app 重启复核）；③ thoughtLevel/effort 透传定量复核；④ 缓存前缀语义（cached_input_tokens 已暴露，命中率进指标）。

**外挂模块（架构已留槽位，核心跑起来后随时插入，无固定顺序）**：

- **L2/L3 验证环境**：依赖目标仓库的服务形态（真实数据库/浏览器），无法提前通用化建设；接功能开发任务时建。
- **Intake Loop**：分诊格式取决于核心 loop 实际能接的任务类型。
- **Garden Loop**：需要 retro.md 积累出真实失败模式后才有原料。
- **局部并行**（worktree + 原子 claim 任务队列）：收益最不确定、复杂度最高，只做评审确认可分解的任务。

---

## 11. 开放问题（待定）

1. ~~Review 量大时 V4 Pro 直连 API 的月预算上限设多少？~~（v0.21 失效：Review 已换防 GLM-5.3 走智谱订阅，无直连按量计费；若未来恢复按量直连渠道再启用此问）
2. ~~首批验证任务用哪个真实仓库？~~（已决，2026-08-19：**mimir**（`~/Documents/projects/mimir`，多租户云原生 AI SRE 平台，greenfield）。方案文档任务已编号（S0-S4/M1-M5）、附录 D 验收已命令化，适配度高；首批取其头部任务 S4-0/S0-00/M1-1，live 凭证任务（S2/S3）待核心链路稳定后接）
3. ~~Judge（里程碑终验）用 K3 还是 V4 Pro？~~（已决，2026-08-20：**K3**——与 DeepSeek 系 executor 异厂商，自偏好风险最低；~~经 pi `kimi-coding/k3` 驱动~~ v0.19 起改 kimi code harness + 订阅，异厂商互证由双面板 V4 Pro 复核承接）
4. 编排器进程形态：常驻 daemon 还是 cron 拉起的短进程？（倾向 cron 短进程 + state.json，重启零成本）
5. ~~资产形态与位置~~（已决，v0.3 定形态；v0.6 定独立仓库）：Flywheel 是"系统"而非单个 skill。**编排器必须是确定性代码**（做成 skill 等于让 LLM 解释执行协议，退化回 agent 接力棒，违反铁律 1）；契约模板是静态资产；任务生成器（需求 → spec.md + packets）与角色工作指令做成 skills。**独立仓库 `~/Documents/projects/flywheel`**（2026-08-14 从 agent-skills 迁出）：orchestrator 代码、契约模板、skills 同仓演进，保证 skill 参数与编排器 CLI 原子同步；agent-skills 保持纯 skill 集合，将来分发时复制 released 版本即可。
6. ~~`hard` 升级通道首选 K3 还是 V4 Pro？~~（已决，2026-08-20：**K3**——升级目标是换异厂商的脑子打破相关性失败，且最难单元自动获得 V4 Pro 异厂商评审；决策原则：质量/效率/速度优先，成本不计入）
7. ~~执行容器形态~~（v0.10 部分决策：现阶段无容器，本机直跑）。遗留问题推迟到接入不可信输入前回答：届时选 macOS Docker 桌面版 vs 微 VM（如 Gondolin）+ 网络管控策略（默认断网 + 白名单 vs 全通）。

---

## 12. 使用方式（怎么用它）

核心关系：**传统做法是 harness 里的 agent 接力驱动下一个 agent；Flywheel 是编排器在外面驱动 harness**。所以"在 harness 里启用"只对生成阶段成立——运行阶段编排器不属于任何 harness 会话（会话有生命周期：kimi code 的 cron 绑 session、7 天过期、fork 不继承 goal；编排器要跨会话、跨 harness、长时间无人值守，不能住在会死的宿主里）。

类比 CI：编辑器里有触发按钮，但 runner 住在外面。skill 是按钮，编排器是 runner。

使用全流程（核心建成后）：

1. **生成任务（在 harness 里，skill 形态）**：对 kimi code 或 pi 说 `/skill:flywheel-new "把 XX 需求做成 flywheel 任务"`；任务生成器调研仓库、生成 `docs/<slug>/`（spec.md + packets + state.json）并提交。kimi code 认 Project/User 级 skills（pi/Warp 共享 `~/.agents/skills`）。
   - 问答纪律：**意图类的它问你，事实类的它自己查证**。技术事实不许问人（自己读代码/跑命令验证，写进 spec 附证据）；意图歧义不许猜（停下来问清楚再生成）。草稿生成后停下，等人审。**冻结点 = 人审契约**：不要求源素材（目标仓库的设计文档等）预先定稿——你审过契约即冻结，此后范围变更只能走 Plan Loop 修订。
   - **验收命令试跑（v0.18 硬规则）**：提交前逐条实际执行每条 L1 命令，「命令+退出码+输出摘要」记入 spec 附表 `## Acceptance dry-run evidence`；无法执行的标 `unverified: <原因>`。允许红（代码未实现），不允许没跑过。
2. **人审契约（关键 checkpoint，不可省）**：过一眼 spec 与 packet 的需求编号、验收命令、边界、**试跑证据**。契约质量决定产出质量，审契约比审代码便宜得多。
3. **启动（在 harness 里扣扳机）**：`/skill:flywheel-run docs/<slug>`——skill 让 agent 把编排器**以独立进程拉起**（launchd/nohup，不依赖当前会话存活）、确认存活、报告状态入口；之后关掉 harness 窗口不影响运行。CLI `flywheel run` 是底层接口与后路。`/skill:flywheel-status` / `/skill:flywheel-stop` 同理。
   - **单 loop 手动触发**（默认全自动，旁路常开）：`flywheel run docs/<slug> --only review --unit M02-T03`（只重跑某单元评审）、`--only execute --packet M02-T03`、`--pause-before review`（逐步监督）。编排器是状态机，手动触发 = 只走一步状态转移。注意 `--pause-before` 与 `--only` 互斥（单步天然逐步执行，组合直接报错）；skill 侧经 `/skill:flywheel-run docs/<slug> [pause]` 的可选参数使用。
4. **观察与干预（在哪都行）**：`flywheel status` 或让 harness 里的 agent 读 state.json/retro.md 讲进度；干预 = `flywheel stop` / 改 state.json / 写 inbox.md——全是文件操作。
5. **交付**：分支/PR 留给人最终确认合并（外层循环归人）。

入口形态跟着"谁需要智能"走：需要智能与对话的环节（生成任务、异常裁决、最终验收）→ harness 里的 skill；不需要智能的环节（驱动循环、跑验收、卡预算）→ harness 外的代码。**harness 是驾驶舱，Flywheel 是自动驾驶仪——在驾驶舱里按按钮，但自动驾驶仪不住在驾驶舱里。**

---

## 13. Skills 清单与触发矩阵

一个 skill 触发全部，每个 loop 各有一个 skill 单独触发。**所有 skill 都只是"按钮"**：参数校验 + 调编排器命令 + 把结果翻译成人话。skill 内不含任何 loop 逻辑（否则违反铁律 1，变成模型解释执行协议）；改 loop 行为只改编排器与契约模板，不动 skills。**安装**（v0.23 分发定稿）：`flywheel install-skills` 从**二进制内嵌快照**复制物化 skills 到 `~/.agents/skills/`（kimi code、zcode、pi、Warp 共享扫描点），并同步物化全局配置 `~/.config/flywheel/flywheel.config.json`；`--link` 为开发机源码树软链的显式选项（非安装路径）。快照随二进制版本原子同步（升级重装即刷新）。调用形态按 harness 而异（v0.18 订正）：kimi code 与 pi 为 `/skill:<name>`；Warp 为 `/<name>`；Claude Code 经 Skill 工具加载——`/skill:` 不是 Agent Skills 标准本身，标准只约定 SKILL.md 文件格式。

| Skill | 触发什么 | 背后动作 |
|---|---|---|
| `/skill:flywheel-new "需求"` | 任务生成（对话式） | 调研仓库 → 问意图问题 → 生成 spec+packets+state → 停下等人审 |
| `/skill:flywheel-run docs/<slug> [pause]` | **全部 loop（全自动）** | 以独立进程拉起编排器，确认存活，报告状态入口；`pause=review` 为监督冻结点 |
| `/skill:flywheel-plan docs/<slug>` | 只触发 Plan Loop | `flywheel run --only plan` |
| `/skill:flywheel-exec docs/<slug> [packet]` | 只触发 Execute Loop | `flywheel run --only execute [--packet M02-T03]` |
| `/skill:flywheel-review docs/<slug> [unit]` | 只触发 Review Loop | `flywheel run --only review [--unit M02-T03]` |
| `/skill:flywheel-verify docs/<slug> [packet]` | 只触发验证闸门 | `flywheel run --only verify`（L1/L2/L3，无模型参与） |
| `/skill:flywheel-status docs/<slug>` | —（只读） | 读 state.json/retro.md/指标，用人话讲进度 |
| `/skill:flywheel-stop docs/<slug>` | 停止 | 停编排器进程，状态定格 |
| `/skill:flywheel-intake` / `/skill:flywheel-garden` | 外挂模块（建成后同法触发） | 同构 |

使用时的注意点：在 kimi code 里说 `/skill:flywheel-run` 时，当前会话的 agent 只负责"拉起并确认"，**它自己不执行任何 loop 工作**——真正的 Plan 会话是编排器另行 spawn 的另一个 kimi code 会话。因此会看到多个 harness 会话并存：你对话的是驾驶舱，被 spawn 的是工人。

### Loop × Harness × Model 法定配置

绑定关系写在编排器配置里（route → harness+model+channel），不在 skill 里；skill 只传任务路径和 loop 名。

| Loop | Harness | Model（全部 max 档） | 渠道 |
|---|---|---|---|
| Plan | kimi code（长驻会话） | Kimi K3 | kimi code 订阅 |
| Execute（default） | zcode app-server（每单元新会话，本机直跑） | GLM-5.3 | Z.AI Coding Plan |
| Execute（multimodal） | zcode app-server（每单元新会话） | GLM-5.3-Flash（唯一多模态档） | Z.AI Coding Plan |
| Execute（frontend） | kimi code | Kimi K3 | kimi code 订阅 |
| Execute（hard 升级） | kimi code | Kimi K3（§11.6 已决） | kimi code 订阅 |
| Review | Codex（`exec --json`，每单元新会话） | GPT-6-Astra | chatgpt 订阅 |
| Judge（里程碑终验） | kimi code 主判（fresh `-p`）+ Codex 复核面板 | K3 主判（§11.3 已决）+ GPT-6-Astra 复核（v0.18 面板；v0.22 换防） | kimi 订阅 + chatgpt 订阅 |
| Verify | **无 harness，编排器直接跑命令** | 无模型 | — |
| Intake（外挂） | zcode app-server | GLM-5.3-Flash | Z.AI Coding Plan |
| Garden（外挂） | kimi code | K3 | kimi code 订阅 |
| B 通道（应急/过渡） | omp（driver 保留） | GLM-5.3[-flash] | 智谱国内 Coding Plan |

---

## 附录 A：关键调研来源

**方法与思想**

- Addy Osmani, [Loop Engineering](https://addyosmani.com/blog/loop-engineering/)（五大构件 + 状态层）
- Addy Osmani, [Long-running agents](https://addyosmani.com/blog/long-running-agents/) / [Agent Harness Engineering](https://addyosmani.com/blog/agent-harness-engineering/)
- Anthropic, [Effective Harnesses for Long-Running Agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)（one-shotting / 过早完成 / 浅层验证三大失败模式）
- Anthropic, [Effective Context Engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)（context rot，最小高信号 token 集）
- Anthropic, [Multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system)（委派契约四要素；agent ≈ 4× 聊天 token，多 agent ≈ 15×）
- Anthropic, [Building a C compiler](https://www.anthropic.com/engineering/building-c-compiler)（确定性 oracle 第一原则）
- Cursor, [Scaling agents](https://cursor.com/blog/scaling-agents)（planner/worker/judge；扁平协调证伪；按行为特征选模型）
- Geoffrey Huntley, [Ralph](https://ghuntley.com/ralph/)（每轮全新上下文 + 文件系统状态）
- OpenAI, [Harness engineering](https://openai.com/index/harness-engineering/)（给 agent 地图而非说明书；doc-gardening）
- OpenAI, [Unrolling the Codex agent loop](https://openai.com/index/unrolling-the-codex-agent-loop/)（内层 agent loop 机制）
- Agent Skills 官方规范：[agentskills.io/specification](https://agentskills.io/specification)（字段权威定论：必填仅 name+description；可选 license/compatibility/metadata/allowed-tools；`skills-ref validate` 校验器）
- Warp, [Skills for agents](https://docs.warp.dev/agents/capabilities/skills/)（v0.18 补录，第三个 Agent Skills 实现：必填同上、参数走正文 `$ARGUMENTS`/`$N` 占位符；`.agents/skills/` 与 `~/.agents/skills/` 为推荐扫描点——佐证 Flywheel 共享安装点选型）
- kimi code, [Agent Skills 官方文档](https://moonshotai.github.io/kimi-code/en/customization/skills)（目录型必填 name+description；whenToUse 驱动自动调用；arguments 支持字符串数组/空白串；`${KIMI_SKILL_DIR}` 变量）
- pi, [Skills 文档](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md)（全局扫 `~/.pi/agent/skills/` 与 `~/.agents/skills/`；按 Agent Skills 标准校验）

**Harness 调研**

- pi：[earendil-works/pi](https://github.com/earendil-works/pi) + [Mario Zechner 设计原文](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/)（<1000 token 内核、四运行时模式、RPC/SDK、session 树）
- oh-my-pi：[can1357/oh-my-pi](https://github.com/can1357/oh-my-pi) + [The Harness Problem](https://blog.can.ac/2026/02/12/the-harness-problem/)（hashline 基准；DeepSeek 反例）
- oh-my-openagent：[code-yeongyu/oh-my-openagent](https://github.com/code-yeongyu/oh-my-openagent)（category 语义路由、IntentGate、完成纪律外部化、mailbox + claim）
- kimi code：[MoonshotAI/kimi-code](https://github.com/MoonshotAI/kimi-code) + [官方文档](https://moonshotai.github.io/kimi-code/en/)（goal mode、print steer 闭环、secondary_model、REST/WS API）
- zcode：[zcode.z.ai](https://zcode.z.ai/en)（闭源、GLM 深度耦合、headless 无官方文档；结论：不进主链路）
- deepseek-harness：[deepseek-ai/deepseek-harness](https://github.com/deepseek-ai/deepseek-harness)（DeepSeek 官方 agent 运行时；v0.8 的错误分类/流 idle 超时/出口契约/重试策略来源；全仓无 judge/verifier，完成靠 worker 自报——反向印证 L1+Review 分层是市场空白）

**模型与渠道**

- [Kimi K3 官方博客](https://www.kimi.com/blog/kimi-k3) / [K3 Pricing](https://www.kimi.com/resources/kimi-k3-pricing)（$3/$15，缓存 $0.30；thinking 历史硬契约；过度主动警告）
- [Kimi Code 会员权益](https://www.kimi.com/code/docs/kimi-code/membership.html)（档位、额度池、5h/周窗口）
- [DeepSeek 官方文档](https://api-docs.deepseek.com/)（V4-Flash-0731 / V4-Pro-0813；reasoning_content 契约；[峰谷定价 2026-08-17 起](https://api-docs.deepseek.com/zh-cn/quick_start/pricing)；[context caching](https://api-docs.deepseek.com/zh-cn/guides/kv_cache)）
- [OpenCode Go 文档](https://opencode.ai/docs/go/)（$10/月、$12/5h、$30/周、$60/月；Pro/K3 仅 $15/月）
- Composio harness 实测：[V4 Flash 跨 harness](https://composio.dev/content/best-agent-harness-deepseek-v4-flash)（pi 第一）、[K3 跨 harness](https://composio.dev/content/best-ai-agent-harnesses)（omp > kimi code > pi）
- [Databricks coding agents 基准](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase)（harness 成本差 2×，质量持平）
- [Harness-Bench, arXiv:2605.27922](https://arxiv.org/abs/2605.27922)（能力应按"模型×harness 配置"粒度报告）
- [Codex issue #35300](https://github.com/openai/codex/issues/35300)（缓存实测 0%↔98.6%，断点与稳定 key 的价值）

**失败模式实证**

- [MAST 多 agent 失败分类, arXiv:2503.13657](https://arxiv.org/pdf/2503.13657)
- [Agent PR 实证, arXiv:2606.18168](https://arxiv.org/abs/2606.18168)（80.2% 弱断言测试）
- [LLM judge 双向失效, arXiv:2604.20202](https://arxiv.org/html/2604.20202v1)（51 对 Android API 迁移、单 judge 的窄域实验，此处作方向性证据；铁律 4 另有多重独立支撑）
- [Reward hacking 泛化, arXiv:2406.10162](https://arxiv.org/abs/2406.10162)
- [Chroma context rot 报告](https://research.trychroma.com/context-rot)
- [Drew Breunig 上下文失效分类](https://www.dbreunig.com/2025/06/22/how-contexts-fail-and-how-to-fix-them.html)
- [LLM judge 偏差面板研究, Verdict-Bench/Jury](https://ijisae.org/index.php/IJISAE/article/view/8407)（单 judge 一致率 ~87.6% vs bias-aware 面板 98.1%；position/verbosity/self 三类偏差的量化与消解）
- [Position bias 系统研究, IJCNLP 2025](https://aclanthology.org/2025.ijcnlp-long.18/)（15 judges × 22 任务 × 150k 实例）
- [Self-preference DBG 度量, EMNLP 2025](https://aclanthology.org/2025.emnlp-main.86/)（自偏好与质量效应解耦）

---

## 附录 B：术语

- **loop**：由编排器驱动的、围绕单一角色职责的重复执行单元。
- **执行单元（unit）**：一个 task packet 的一次完整处理（spawn → 执行 → L1 → 评审）。
- **契约（contract/packet）**：loop 间交接的带版本号文件，系统唯一信任的任务描述。
- **oracle**：确定性的验收手段（测试、命令、脚本），与 LLM 评审相对。
- **熔断（circuit breaker）**：预算或空转检测触发的自动停机/升级机制，预算部分为可配置开关（off/soft/hard）。
- **ratchet（棘轮）**：只允许单向移动的闸门，如覆盖率只许升不许降。

---

## 附录 C：编辑格式 A/B 实测（2026-08-13，本地）

环境：pi 0.84.0，provider `opencode-go`（OpenCode Go 订阅 key），模型 `deepseek-v4-flash`，`pi -p --no-session` 非交互驱动。6 个编辑任务（t1 单行修复 / t2 三处分散修复 / t3 重复代码块消歧 / t4 五处重命名 / t5 函数中间插入 / t6 空白敏感格式），每种编辑模式独立全新会话跑一遍，验证方式为任务自带的 `node test.js`。

- **冒烟**：V4 Flash 经 OpenCode Go 在 pi 上链路可用（修复 bug → 跑测试 → PASS），证明 Execute Loop 的"pi CLI + Go 订阅"通道可行。
- **hashline 模式**（pi-hashline-edit 项目级安装，`--approve` 加载）：6/6 通过，0 次编辑失败，总耗时 201s（单任务 11–119s）。有效性已核实：JSON 事件流中 edit 工具参数为哈希锚点格式（`{"op":"replace","pos":"16#SW",…}`），确认扩展真实生效。
- **plain 模式**（pi 默认 edit）：6/6 通过，0 次编辑失败，总耗时 117s（单任务 12–47s）。

**结论**：两种模式在本基准下全部通过、均无编辑失败；plain 总耗时约为 hashline 的 58%（hashline 的读文件带哈希标注、单轮 token 更多）。**Execute Loop 默认使用 pi 原生编辑协议，不引入 hashline**。此结果与 can1357 基准中"DeepSeek 系是唯一 hashline 反而更差的模型"一致。

**置信度声明**：6 任务 × 单轮、小型机械修复，属方向性证据。若后续在大型重构类编辑上观察到默认 edit 失败率上升，应重新评估（hashline 作为备选已验证可用）。


---

## 附录 D：reasoning_effort 透传实测（2026-08-18，本地）

方法：OpenCode Go 直连端点（`https://opencode.ai/zen/go/v1/chat/completions`），模型 `deepseek-v4-flash`，同一难题 prompt（三合一：987654×32109 心算 / 7^2026 末三位推导 / 5 金币称重方案），`reasoning_effort` 分别传 low 与 max 各 3 次，对比推理量。另做 pi L0 冒烟（`pi -p --model opencode-go/deepseek-v4-flash --thinking max --no-session --mode json`）。

数据：

| 档位 | 推理文本字符数 | completion tokens | usage 细目 |
|---|---|---|---|
| low ×3 | 14680 / 15100 / 17234 | 10562 / 10874 / 11948 | `reasoning_tokens` 存在（9340–10877） |
| max ×3 | 36334 / 49843 / 51070 | 14962 / 18287 / 18996 | **`reasoning_tokens` 字段缺失** |

结论：

1. **Go 透传 `reasoning_effort`**：max 档推理文本量为 low 档的 2.4–3.4 倍，方向正确、量级显著（若被代理剥掉，两档应统计不可区分）。Execute/Review/Judge 保留 OpenCode Go 通道，显式传 max。
2. **坑：max 档下 usage 缺 `reasoning_tokens` 细目**（low 档有）。编排器计量不得假设该字段恒在，缺失时回退用 completion tokens + 响应推理文本量。
3. **pi L0 冒烟通过**：pi 0.84.0 `--thinking max` 经 Go 驱动 V4 Flash，事件流中 reasoning 内容真实到达；pi 侧 usage 暴露 `reasoning` / `cacheRead` / `cacheWrite` 分项，满足编排器指标采集需求。
4. 方法学备注：简单题（3,3,8,8 算 24 点）两档推理量无差异——答案已记忆化，透传验证必须用非记忆化难题；Go 代理返回的 usage 数字与文本量偶有出入，以文本量为辅证。
