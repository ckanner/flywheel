---
name: flywheel-status
metadata:
  version: "2.0.0"
description: Read-only progress report for a Flywheel task — reads state.json/retro.md/metrics and explains progress in plain language. Use for "/flywheel-status docs/<slug>" in DSH, "/skill:flywheel-status docs/<slug>" in Kimi, or a question about how a flywheel task is doing.
whenToUse: 当用户询问任务进度、成本、缓存命中率、是否停滞或有 inbox 待办时使用（只读，不改状态）。
compatibility: "Requires the Flywheel CLI; read-only status and measured usage reporting."
arguments: slug
---

# flywheel-status — 看进度（只读）

Button only: read state + translate. Writes nothing.

DSH 在对话中使用 `/flywheel-status <任务资料目录>`；Kimi 使用 `/skill:flywheel-status <任务资料目录>`。
可直接在对话中查询，无需先填写 Flywheel 视图；查询也不会自动改变视图所选任务。

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **定位原任务**：先执行 `task_dir="$(cd "$slug" && pwd -P)"`，失败则报告路径问题。
   查询不要求创建 worktree、检查分支或切换 checkout；即使当前终端在 `main`、
   detached HEAD 或仓库外，也直接查询这个原任务路径，不搬移账本。
   `state.json` 不存在时先看控制回执是否有 `preparation`：这是尚未进入实现的调查，报告其预算、证据与独立终态；两者都没有才说明尚未初始化。不能仅据此断言契约未生成。
   可用 `flywheel control status --task-dir "$task_dir" --cwd "$task_dir"`
   获取只读快照；`state: null` 也不代表需要手工创建 state.json。
2. **调用**（使用绝对任务路径，不依赖当前工作目录）：规划任务优先读取结构化状态，
   DSH 用 `flywheel_control({action:"status",taskDir,cwd})`；其他宿主调用：

   ```bash
   flywheel control status --task-dir "$task_dir" --cwd "$task_dir"
   ```

   它包含精确方案、独立审查、批准和子计划记录。需要人读摘要或查询旧任务时可用：

   ```bash
   flywheel status "$task_dir"
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**，用人话讲：
   - 有 `state.planning` 时，先报告 phase、当前/已批准根方案、各 scope 子计划的
     revision/digest、独立审查与 findings、累计/本轮次数、已应用反馈、启动授权。
     queued 的 planning-control 回执只是排队，不能算应用或通过。没有实现 workflow
     时说明尚未激活，不能按旧 Full 报告。根批准与子计划 Review 分别报告。
     等待人工批准时给出同一 Flywheel 视图的精确版本按钮；查询不会 approve 或启动。
     只批准后未授权启动时说明“开始已批准方案”，普通 Run 不会扩大授权。
   - 请求模式、生效 Quick／Full、选择理由与升级历史。需要机器完整记录时用
     `control status` 读取 `state.workflow`；无 planning 的旧状态无字段才按 Full 解释。
     Plan／Judge 不适用时说明原因，不把它们显示为通过。状态查询不重分类或切换模式。
   - 任务状态、编排器是否存活（pid）、当前里程碑、上次推进时间——
     推进时间久未更新 = 可能停滞/断链，提醒用户。
     `running:false` 只表示引擎未运行；分别说明 completed、paused、stopped、
     escalated 或 budget-stopped，不把所有终态都叫“已停止”。
   - 每个 packet：状态、尝试/失败次数、三级证据（executor 自报 < L1 确定性
     通过 < Review  verdict——证据等级不同，讲进度时区分）。
   - token/成本累计、未知用量／未知成本调用数、缓存命中率、预算档与告警。
     已知总数只是下限，未知不等于 0。缓存率变动只能提示排查，不能无证据
     断言前缀损坏；不同 provider 未必提供可比的 cache usage。
   - 有会话记录时说明角色、作用域、代次、动作／原因及最新调用的状态、派发、
     结果、清理和用量事实。`resume-candidate` 只是恢复尝试资格，不证明历史已保存
     或恢复成功；`cleanup=true` 也不等于结果已知。旧任务无记录不编造会话状态。
     查询不读取历史正文、不核销回执，也不改 state。P3 pending 不证明进程存活；
     找到回执不等于原候选证据齐全。P4 重建原因只表示所记录的可观察阈值，不能
     推断上下文容量或质量收益。读取 metrics 时将同调用 correction 作为用量对账，
     不当成额外一次模型调用。
   - inbox 有待处理条目时**必须显著提醒**：这需要人。
   - 有 packet worktree 记录时说明实际路径／模式；`in-place` 不是独立工作区。
     清理、派发或结果未知时明确阻断；不能只凭引擎或子进程已停就建议立即重试，
     也不能建议改 fresh、另建任务或删除记录。规划角色的 blocked unknown 可提示
     用户在视图或 `flywheel session ... --abandon` 退役；不要建议模型代输确认。
   - 用户要更多细节时，可读该任务目录中的 `retro.md`、`metrics.jsonl`、
     `orchestrator.log` 再讲。

用户询问实际模型或配置来源时，再从 `task_dir` 用
`git -C "$task_dir" rev-parse --show-toplevel` 解析并核实原任务的 `repo_dir`，调用
`flywheel config inspect --cwd "$repo_dir" --task-dir "$task_dir" --json`。
配置查询失败不撤销已经取得的状态结果。恢复使用 `flywheel-run` 的原任务路径与
原任务分支，分支／工作区检查属于启动步骤，不妨碍本次只读查询。
区分模型 profile、harness、DSH provider 和账号 channel；别名不是授权证明。
需要排查安装时调用 `flywheel doctor`，SDK 探测显式加 `--probe`，不要自动试调用付费模型。
