# 调查目标的准备与交付

适用于“只排查”“先定位”“排查后在原范围内修复”。本文件只规定准备、调用与
结果翻译；动作循环、预算、选择、取消与恢复由 Go 负责。

## 准备一次

核实当前任务 checkout 与实际 `docs/<slug>` 目录；复用已有目录，不复制准备账本或
重置预算。先读取当前安装版本的模板：

```bash
flywheel template preparation.schema.json
flywheel template preparation.example.json
```

用当前宿主模型形成少量可证伪假设和具体取证动作。记录目标、预期、环境、时间窗口、
已知证据、精确允许读取的相对路径以及动作、总时限、文件体量和输出上限。不要照抄
示例路径。首版支持 `read_file`、`search_text`、`list_files`，均由 Go 实现；目录
授权只允许读取名字，不递归授权读取内容。没有 shell、联网、生产控制或临时写入动作。
不把 full-access、角色名或 worktree 当成只读隔离证明。

只排查设 `intent: investigate`，`implementation-scope: []`。已要求“排查并修好”设
`investigate-and-fix` 并记录已授权的修复路径范围；不能用该字段自行扩大原需求。
JEV 默认省略，旧 `jev-shadow` 不授权新数据发送或采用。需要实验时沿用用户明确
批准的版本化 decision 配置、具体模型、渠道、数据字段和预算，不自动开启付费服务。

在 DSH 调 `flywheel_control`：`action: investigate`，传实际绝对 `taskDir`、`cwd`，
并传 `investigation: {action: start, contract: <已校验的完整契约>}`。Go 保存
`preparation.json`；宿主只生成输入，永不编辑该状态文件。其他宿主等价调用：

```bash
flywheel control investigate --task-dir "$task_dir" --cwd "$repo_dir" < start-request.json
```

`start-request.json` 是 `{ "action": "start", "contract": { ... } }`。这里的
`start` 只保存准备记录，未执行调查动作。用户明确要求先审阅方案时，到此交付并停止。

## 启动、查看或恢复

原需求授权调查运行时，使用刚读到的 revision 调一次：

```json
{"action":"investigate","taskDir":"/absolute/repo/docs/task","cwd":"/absolute/repo","investigation":{"action":"run","revision":1}}
```

示例 `1` 必须替换为实际回执的 revision。Go 在后台运行整个有界动作集；不要在
skill 中逐动作调用或重写选择循环。通过同一目录的 `flywheel_control status`、
`flywheel-status` 或视图查看；停止用 `flywheel-stop` / `control stop`。
后台启动的 `ok:true` 仅表示启动请求确认，状态和报告以随后快照为准。

结果未知时查原目录。未确认动作不会自动再派发；`run` 只尝试读取原回执，仍缺失则
保持 blocked。可选 JEV 未知请求不会重发，但可结算未知用量后采用合法确定性基线。
不要新建任务、换模型、切模式或手工改 JSON 绕过它。

## 形成报告并决定终点

`awaiting-report` 表示 Go 已交付观察，当前宿主模型据此形成一次报告输入，不代表
已经定位因果。引用观察的 `invocation.id`，逐项写假设为 supported / contradicted /
unknown，说明原因、反例检查、局限、推荐修复、回归检查和回退条件。
报告顶层状态为 supported / inconclusive / access-blocked；supported 必须有真实
观察引用，Go 校验引用和状态一致性，不代替分析者证明语义结论。

通过 `flywheel_control` 传 `investigation: {action: report, revision: <实际版本>,
report: <完整报告>}`，或相同 `control investigate` JSON stdin 提交。输出过大、
权限不足或未定位时如实交付局限；不伪造根因或代码完成。

只排查到报告结束。已授权修复且 supported 的同一任务，在同一工作区生成合法 Spec、
packets 与验收，再走常规 launch；Go 绑定调查来源，将决策已知／未知用量一次计入
任务总账，并约束实际实现 diff 在原修复范围内。不重复询问原授权，不因 Full 扩大
范围。尚无受支持结论时交付未定论和缺口，不用增加新任务来重置原调查预算。
