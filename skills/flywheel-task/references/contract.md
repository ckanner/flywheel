# 紧凑契约准备

用于已有 legacy 任务、明确手工准备的兼容路径及调查修复交接。新实现目标
默认使用薄 brief，由 Go 的方案/子计划调用形成这些契约，不要求宿主提前写全。字段以当前二进制的
`flywheel template spec.schema.json` 和 `flywheel template task-packet.schema.json`
为准；不新增只有提示词理解的运行字段。

## 工作流事实

把经过调研的工作流判断写入 Spec frontmatter。以下是**字段示例**，代码依据与
理由必须替换成当前任务的真实事实；不要复制这些值作为低风险证明：

```yaml
workflow:
  version: 1
  mode: auto
  reason: "缺陷已定位，沿用现有实现模式与回归测试入口"
  scope-clear: true
  implementation-known: true
  verification-complete: true
  review-sufficient: true
  evidence:
    - "src/example.go 中已定位失败分支；tests/example_test.go 可覆盖复现与回归"
  risks: []
  not-applicable:
    L2: "纯本地函数，未改变模块间协议；L1 包含调用边界回归"
    L3: "没有 UI、外部服务或独立用户路径；行为由函数契约直接验证"
```

Go 决定最终策略。Quick 需要一个里程碑、一个 packet、四项事实标志均为 true、
非空理由与证据、无风险信号、全部 Spec 需求在 packet 中映射且有非空 verification，
以及至少一条 L1。缺少 L2 或 L3 时须解释对应的不适用理由；存在该层检查就必须执行，
不能为满足 Quick 删除检查。不能用“一共改几个文件”替代范围、风险与验收判断。

发现鉴权、权限、支付、迁移、并发、事务、跨模块重构或未决架构／产品取舍时，
记录具体风险，不把相关标志写成 true。证据不足时允许 Full；用户明确 Full 就保留
Full，不通过修改契约抢回 Quick。模型建议只是声明，执行与最终评审仍须检查新风险。

旧任务缺少 workflow 字段继续使用 Full；恢复不重分类，也不手动改 `state.json`。
不得更改已运行任务的流程字段来逃过原来的完成条件。

## 可执行验收

- 每条需求写明可检查的成功标准与 verification，不以“符合最佳实践”替代断言。
- packet 引用对应需求、执行 route、真实 context-files、修改边界、L1/L2/L3 和
  有界失败出口。覆盖率是项目门禁时写成真实命令；普通单元测试与回归可由 executor
  补充，冻结验收写入 `boundaries.must-not-touch` 或配置 `protected-test-patterns`。
- 新任务在 packet 的 `done-report-format` 声明 `workflow-risks: []` 字符串数组。
  执行者发现新风险时写入具体事实；Go 在最终 Review 前处理升级，不让执行者
  自己改模式或 `state.json`。无新风险时保持空数组，不写虚构的“无风险”证明。
- 在任务 worktree 逐条试跑所写验收命令，记录命令原文、退出码与最多十行输出摘要。
  目标尚未实现而正常失败可以成为基线；命令无法运行、缺少环境或凭证时写明
  `unverified: <原因>`，不能把 verification-complete 声称为 true。
- Quick 的正式 L2/L3 由引擎在实际 packet 候选上执行。宿主预跑集成工作区的结果
  只是准备证据，不能替代最终候选的验证或最终 Review。
- 已知短复现或局部检查时，可在 packet 中声明至多八条 `feedback-checks`，使用与
  L1 相同的 `cmd`／`script` 和 `expect` 结构，并用最多 160 字符的 `purpose` 说明
  它能区分什么原因。用途会发给 JEV，不得混入凭据、命令或原始日志；缺少用途的
  检查仅供执行者参考。JEV 开启后由 Go 从有用途的已批准检查中选择并执行。
  不要把完整验收移入这里，也不要
  因某条短检查通过就宣称任务完成；最终 L1/L2/L3 和 Review 仍须独立完成。
- 将试跑事实、需求授权、交付范围和非目标写入 Spec 正文；不把这些说明伪装成
  schema 未声明的机器字段。凭证只记录变量名与所需能力，不保存值。

## 独立任务与调度声明

需要并行时，先读取当前二进制的 packet schema 与 `config inspect`；只使用已实现的
`schedule` 字段。以真实代码依赖排序 packet 编号，`after` 指向较早的已知单元，
`writes` 是本任务允许修改的规范相对路径或目录前缀，不能用通配符、上级路径或
模糊的全仓范围假装独立。把实现、普通测试和必要配置一起纳入写入范围。

不能确认独立性时保留串行；存在共享文件就如实声明，交给 Go 排队。宿主只准备
契约，不自行并发调用执行器、不分配额外 packet worktree、不写调度或 session 状态。
规划修订仍需整体加载校验；用户在控制台切换调度策略只影响后续启动。

## 文件与配置

创建 Spec、task packets 和需要的冻结验收。任务级 `.gitignore` 逐行忽略
`orchestrator.log`、`orchestrator.pid`、`metrics.jsonl`、`.state.lock`、`.run.lock`、
`.stop-request`、`.control/`；不要忽略 `state.json`、`retro.md`、`inbox.md` 或 reports。
这些运行状态文件由引擎生成与管理，不由 skill 预填。

配置发现按任务 → 项目 → 全局 → 内嵌默认，取首个完整文件，不做深度合并。
默认复用已有 ChatGPT、Kimi Code K3.1 Pro 与 DeepSeek Flash 资源；实际模型和
权限以 `config inspect` 为准，不因流程模式覆盖用户绑定或恢复智谱等旧后备渠道。
可选 `claude-code-pro` 是独立 harness；用户决定用途后才绑定 profile，不把
`subagent_claude_code` 视为 DSH provider，也不增加父模型委派。启动前由 doctor
核对配套 0.5.1 machine/lifecycle/recovery API 与 full-access，角色续接、只读恢复和
质量重建由 Go 管理；保留原调用证据，不另写恢复状态或因配置变化重置预算。
需要 live 凭证时如实声明 `needs-live-credentials`，沿用实际凭证授权机制，不能
凭 full-access 或 Quick 名称跳过它。
