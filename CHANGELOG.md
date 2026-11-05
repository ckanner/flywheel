# Changelog

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

Codex Pro 默认接入。安装和迁移见 [配置指南](docs/configuration.md#codex-pro-默认与旧配置迁移)；源码、测试、CI 和 registry 发布证据分别记录，不用离线通过代替用户 Mac 实测。

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

DSH 桌面集成、可靠交付闭环与 npm 一条命令安装。此节记录对应版本的源码变更，不单独证明 npm 已发布或用户 Mac 已验收；用 `npm view @ckanner/flywheel version` 核对实际发行。旧 0.22.1 不包含本节新增安装和 DSH 接口。安装与迁移见 [入门教程](docs/getting-started.md)。

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

更早历史见 [历史设计](docs/design-history-v0.24.md) 的变更记录。
