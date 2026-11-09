# Flywheel 流程控制台

本文描述当前源码中的控制台。源码、开发候选、本机安装与 npm 正式发行分别验收；不要仅凭本文判断 `@latest` 已包含这些功能。CLI、平台包和 DSH 插件必须同版。

## 从配置开始

在已进入对话视图的 DSH 会话中，Flywheel 排在“统计”之后；新会话的空白首页尚未显示这排会话视图。内部有“运行画布”“全局／项目配置”“运行与对比”。全局配置可在没有任务目录时编辑；项目配置写入所选 Git 项目根。任务配置仍具有最高优先级，取首个完整文件，不做字段深度合并。

先读取草稿，再选择 Provider／Harness、模型、推理等级、权限与历史策略。“载入本版本默认值”导入完整默认模板；保存前显示差异与目标，Go 核对内容 revision、备份旧文件并原子写入。正在运行的引擎保留原快照，新配置在后续启动时加载。

支持的资源入口包括 ChatGPT · DSH、ChatGPT · Codex Pro、Kimi Code、DeepSeek Account（默认 Flash）及可选 Claude Code Pro。模型初值来自本版本模板，真实能力以 `config inspect`／`doctor` 为准。Claude 的可选适配器必须匹配公开 machine API；缺失或权限不支持会拒绝调用，不自动换后端。账号登录仍由各 Provider 自己完成。

`permissions.bindings` 可以覆盖 `loops.<role>`、`routes.<route>` 和 `judge-panel`；未覆盖项使用全局策略。会话策略按角色覆盖：执行 routes 共用 execute 的策略，各 packet 的历史仍隔离。scoped 只续接相同任务、角色、目标和绑定下可恢复的历史；fresh 也保留调用安全账本。

## 横向画布与节点操作

主线从左到右展示 Plan／拆分、调度、任务分支、集成、终验、PR、Merge。点击任务展开 Execute、L1、Review、Git 交付。状态配合颜色和文字，运行与节点详情展示来源 DSH 会话、项目／worktree、分支、HEAD、run ID、调用绑定和失败证据。

画布支持拖动、横移、定位当前节点和分组浏览。大任务使用有界诊断摘要；“读取完整节点证据”按需加载选中节点，revision 与同一时刻的画布状态一起刷新。摘要不会改写原始账本。

| 操作 | 含义 |
|---|---|
| 运行此阶段 | 提交已观察的 revision；Go 在任务租约内重新检查前置条件 |
| 人工继续一次 | 填写原因，追加一次 Execute／L1 尝试；保留累计失败和预算 |
| 已完成单元重跑 | 归档旧证据和提交回执，使当前与后续单元的候选证据失效 |
| Git 交付 | 只在当前 L1 与独立 Review 通过后持久化当前单元 |
| 终验 | 已集成单元的 L2／L3 与独立 Judge；不重新调用 Plan 或启动未完成单元 |
| 继续整个流程 | 由 Go 继续已保存的 Full 或 ultraWork／Quick 策略 |
| 停止 | 请求 Go 收束在飞子进程；关闭页面不代表停止 |

有显式调度声明且依赖已交付的独立 packet 可单独提前执行，仍使用隔离 worktree。合并后的候选变化会使旧验收失效，必须重新验证和 Review。每个异步节点操作保存 running／passed／failed／cancelled 回执，失败原因不会仅留在后台日志。

Quick 在 UI 中显示为 ultraWork／Quick；它仍要求确定性验证和独立最终 Review。Plan／Judge 按持久化策略标为不适用，不伪造完成状态。

## 串行与并行

配置入口为 `scheduler`，字段和边界以 `flywheel template scheduler-policy.schema.json` 为准。例如：

```json
{"scheduler":{"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1}}}
```

`providers` 的键是实际账号渠道名。并行资格来自冻结 packet 的声明，而不是模型临时口头建议：

```yaml
schedule:
  after: [M01-T01]
  writes: [src/search/, tests/search/]
```

依赖必须指向较早编号的已知 packet；写入范围是规范相对路径／目录前缀，不支持通配符或越出项目的路径。声明要覆盖实现、普通测试和必要配置，并考虑共享测试服务等真实依赖。未声明独立性、写入范围冲突或需要 live 凭证的单元保留串行。

只有 Execute／L1 并发，工作区各自独立；一个 Go 协调器串行 CAS 保存状态和用量。hard 预算只允许一个模型调用在飞，并在下一调用前确认前一笔计费。其他模式按最大并发与渠道限制接纳。停止会等待整批已启动工作收尾。

Review、集成与里程碑 gate 保持串行。后续分支吸收已完成成果后，在合成候选上重新验收。执行重试耗尽仍走原有升级策略；集成冲突或门禁失败保留 worktree 和失败证据。`writes` 是调度／验收契约，完全权限本身不提供 OS 沙箱。

## 在同一 Provider Session 中接管

任务节点显示准确的 Provider session。读取最近回复只读取摘要校验过的原回执，不宣称它是后端的完整历史。

已确认收尾、已消费结果、绑定匹配的 packet session 可“暂停并接管”。接管标记持久化后，普通 run／单阶段操作不能绕过它。桌面输入的人工 Prompt 和 `flywheel session` 都通过原适配器恢复那个准确 thread，并继续预算与计费；恢复失败或需要重建时不冷启动替代。

```sh
cd -- '/absolute/task-worktree'
flywheel session '/absolute/task-worktree/docs/task-slug' --key 'execute/M01-T01'
```

终端每行是一条 Prompt；`/history` 查看最近回复，`/reconcile` 核对原调用，`/return` 验证交回，`/release` 退出接管并保持暂停，`/quit` 保留接管退出。多行 Prompt 可用桌面文本框。

“验证并交回”重新运行 L1、适用的 Quick gate 和独立 Review；人工回复本身不是通过证据。Go 保存绑定当前候选的 `human-takeover` 来源，后续正常交付仍由引擎完成。“退出接管”不声称验收成功；结果或清理未知时也不能借此解除阻断。里程碑会话目前提供只读回复，交互接管以 packet 为单位。

会话账本存储为 v2，新引擎兼容旧 v1；旧引擎会拒绝 v2，避免忽略接管标记。配置的 `sessions.policy-version` 仍是 1，worker／lifecycle 协议不因此改变。

## 发现运行与比较

CLI 和桌面启动自动登记 run ID；会话投影保留多个已确认任务。登记位于全局配置旁的 `runs/`，可用绝对 `FLYWHEEL_RUNS_DIR` 覆盖。列表只读取明确登记，显示最近 100 项，不扫描整个磁盘；当前存活状态始终重新查询 Go。

对比页面显示操作种类、分支、HEAD、契约／配置条件、运行时长、新增尝试、失败尝试和交付。可选准确 run ID，也可查看本任务全部调用（包括早期未登记记录）。指标先按 invocation／receipt 对账再汇总，迟到回执归属原调用，不重复计费。

用量、费用和时延只表示已知量，缺失／部分数据单列，费用也可能来自配置单价估算。调用失败率不等于代码质量；条件不同的运行只并列展示，不伪称配对收益。完整的质量结论需要任务验收和独立 oracle。

## 远端 PR 与 Merge

在 PR 节点先查询项目与远端状态。远端操作需要 GitHub CLI 的正常登录，以及当前本地任务完成、候选仍有有效验收、明确的仓库／HEAD／目标分支授权。已记录的项目远端发生变化会拒绝交付；fetch 与 push URL 必须指向同一仓库。

推送使用已批准的 commit 和明确任务分支，不强推，不隐式跟随 tags 或推送 submodules。可选择独立的远端任务分支，不必切换本地 checkout。推送后重新查询已有 PR，避免把预先创建的 Draft PR 误当成需要重复创建。

合并再次核对 PR、源 HEAD、base 和检查状态，并使用 GitHub CLI 的 `--match-head-commit`。不使用 admin 绕过，也不擅自开启 auto-merge。仓库要求 merge queue 时，“请求已接收”和“已合并”分别记录，以远端回执为准。接口语义见 [GitHub CLI 文档](https://cli.github.com/manual/gh_pr_merge)。

已确认的创建／合并操作重跑时先核对已有结果。命令失败或回执丢失保留 unknown，不猜测远端成功。PR／Merge 状态与引擎的本地完成分开；发布应绑定实际 merge commit 和发布产物重新验收，不因点击 Merge 自动发布 npm 或部署。
