# Auto／Quick／Full 第一阶段实施与验收

日期：2026-11-07。对应 [PR #29](https://github.com/ckanner/flywheel/pull/29)，基线为
`f6272907a5e9fdc891bec0394a593636a5bb8214`。原方案的文档提交为
`a9e8f9763b9d9bfb81f19fb903f1b336a6944737`；本轮在同一 PR 追加实现。
最终提交、完整门禁结果和六包候选的源码快照标识记录在 PR 的验收表中。

## 已实施范围

统一入口为 `flywheel-task`：当前宿主研究代码，准备任务集成 worktree、Spec、
packets 与真实检查依据，再通过 `flywheel_launch` 或 `control launch` 启动。
CLI 消费已准备的契约，不另行调用分类模型，不接管自然语言需求研究。

Auto／Quick／Full 是流程选择，不是模型或权限预设。旧任务继续 Full；新任务的
Go 选择器检查结构、需求映射、资格事实、证据、风险，以及 L1/L2/L3 的适用性。
首版 Quick 限一个里程碑和一个可独立验收的 packet，没有任意文件数阈值。

Quick 保留 Execute、L1、适用缺陷注入、实际候选上的 L2/L3、一次独立最终
Review，以及同一候选的本地 Git 交付。正常首次成功有两次阶段 Agent 调用；
宿主准备、内部工具轮次、重试和升级不计入这两个阶段。Plan／Judge 不适用时
分别记录理由，没有伪造通过裁决。

策略版本、请求模式、生效策略、阶段适用性与升级事件由引擎持久化。Quick 发现
新风险、验证缺口或有界修复失败时只向 Full 升级，保留任务身份、预算、已有
工作区、尝试和历史。手动最终 Review 也立即保存必要的升级 checkpoint，但
不在这个单步内调用 Plan 或继续交付。未知策略版本和不一致状态在加载期拒绝。

DSH 使用公开 session projection 折叠原有 command/native tool/PTC 事件，恢复
确切的 taskDir/cwd。没有增加自定义 Session 事件，没有扫描或猜测任务目录，
也没有第二份引擎状态。重放、checkpoint、fork、会话切换和迟到响应保持隔离。
未确认的变更必须先查询原任务；没有已记录会话目录时，省略 cwd 的变更请求在
Go 派发前拒绝，显式绝对路径仍可使用。

新增的第 11 个 skill 及其契约参考文件进入二进制内嵌、快照安装与真实 npm
安装检查。旧的准备、运行、执行、评审、验证、状态按钮及中英文指南同步更新。

## 验收覆盖

| 层面 | 命令或测试入口 | 验证的事实 |
|---|---|---|
| Go 全部包 | `make quality-gate` | vet、gofmt、全部测试与逐函数 100% statement coverage；不降低旧门禁 |
| Quick 行为与恢复 | `internal/machine/workflow_*_test.go` | 两次调用正常完成、候选 L2/L3、独立最终裁决、退回修复、资格变化与单向升级、预算和工作区保留 |
| 完成与故障 | 同上及已有 Git／进程测试 | unit 已交付、milestone 已关闭、task 已完成三个 checkpoint 的恢复；外部提交改变候选不能复用旧绿灯；读失败、CAS、取消、Git hook／提交失败保留现场 |
| 契约和状态 | contract/state/diagnostic/cli 的 workflow 测试 | 合法选择、旧账本 Full、未知版本、阶段矛盾、只读预检、字面 argv、活动任务防重复启动和模式冲突 |
| DSH 插件 | `npm --prefix plugins/dsh run check` | 两个受支持版本的真实发布包，包括 Session、SessionProjectionRegistry、renderer、native/PTC dispatcher，及控制协议／视图行为 |
| Go／Node 边界 | `FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH\|CodexPro)NodeWorkerOfflineBridge$' -count=1 -v` | 两类 worker 的真实跨进程协议、权限、取消、清理回执与错误语义 |
| 并发行为 | CI 中相同的七个核心包 `go test -race ... -count=1` | machine、verify、watchdog、drivers、cli、modules、diagnostic |
| npm 协议 | `npm test` | 启动器、安装器、配置保护及发行契约 |
| 六包安装 | `npm run build:npm` 后 `FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install` | 隔离源码快照构建、真实本地 registry 安装、11 个 skills 与引用文件、实际 host 入口导入、配置保护、离线 worker 与取消 |

六包安装夹具提供真实锁定的 `dsh-subagent-codex-pro@0.8.0` 与 `zod@4.3.6`
发布 tarball，核对源码锁文件的 SHA-512、包名、版本与依赖图。不会用伪造 schema
依赖或只检查工作区源文件来代替安装后的入口。

## 审查中修复的问题

- Quick 的手动 Review 不能漏掉适用缺陷注入；缺失、不可注入或未完成结果不能交付。
- milestone 已关闭后恢复也重新核对最终候选，防止收尾前崩溃后错误认可另一份已提交代码。
- Quick 及其升级任务的缺陷注入配置进入证据和契约身份，原生 Full 的旧身份保持兼容。
- 手动评审发现必须 Full 时立即持久化；不能在重启后重新取得一次 Quick 裁决来抹掉风险。
- 初始 CAS 保存失败时不创建 Git 契约 checkpoint，也不启动模型。
- 控制诊断取消后不再为了填充响应 cwd 额外运行 Git，保留正常清理和退出行为。
- 修正新 skill 文案的 YAML 解析问题，避免安装时漏装；真正安装的新 skill 数量为 11。
- 同一任务再次启动后刷新实际状态，避免视图只保留旧状态；无法证明会话 cwd 时提前拒绝变更。
- 独立审查复现了宿主测试共用 100ms 超时导致正常 status 偶发失败；改为真实进程启动后显式取消挂起请求，正常恢复使用原有控制期限，生产超时与断言不变。
- 明确 `flywheel-run` 同时收到 `pause=review` 与 mode 时在同一前台命令中透传，防止遗漏用户显式 Full 选择。

## 验证边界

本轮使用 Linux、Go 1.27、Node 24.22.1 和 npm 12.2.1。真实发布的 DSH
0.2.1-rc.5／0.2.2-rc.1 包参与离线测试；模型结果由明确的测试驱动提供。
这不等同于用户 Mac 的原生桌面视觉验收、Seatbelt／钥匙串行为或真实账号推理。
没有声称所有实际任务都只需两次调用，也没有用阶段减少推导耗时或费用减半。

六包为开发候选，源码版本沿用 0.28.5，并明确标记不可发布；不覆盖已有 npm
发行或 tag，不提升 latest，不修改用户安装。正式发行须另选新版本并遵守现有
精确 tag、同一候选、完整性和安装门禁。模型、账号、完全权限选择、串行 packet
调度与独立 Execute／Review 会话保持现有约定。多任务团队、依赖图并行、附件协议
以及引擎内自动 GitHub 交付仍属于后续范围。
