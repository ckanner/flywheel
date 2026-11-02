---
name: flywheel-status
metadata:
  version: "1.5.0"
description: Read-only progress report for a Flywheel task — reads state.json/retro.md/metrics and explains progress in plain language. Use when the user says "/skill:flywheel-status docs/<slug>" or asks how a flywheel task is doing.
whenToUse: 当用户询问任务进度、成本、缓存命中率、是否停滞或有 inbox 待办时使用（只读，不改状态）。
compatibility: "Requires the Flywheel CLI; read-only status and measured usage reporting."
arguments: slug
---

# flywheel-status — 看进度（只读）

Button only: read state + translate. Writes nothing.

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：实际任务目录 `$slug` 下的 `state.json` 存在；不存在时说明引擎尚未初始化状态，不能仅据此断言契约未生成。
   可用 `flywheel control status --task-dir <绝对任务路径> --cwd <绝对仓库路径>`
   获取只读快照；`state: null` 也不代表需要手工创建 state.json。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel status "$slug"
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**，用人话讲：
   - 任务状态、编排器是否存活（pid）、当前里程碑、上次推进时间——
     推进时间久未更新 = 可能停滞/断链，提醒用户。
     `running:false` 只表示引擎未运行；分别说明 completed、paused、stopped、
     escalated 或 budget-stopped，不把所有终态都叫“已停止”。
   - 每个 packet：状态、尝试/失败次数、三级证据（executor 自报 < L1 确定性
     通过 < Review  verdict——证据等级不同，讲进度时区分）。
   - token/成本累计、未知用量／未知成本调用数、缓存命中率、预算档与告警。
     已知总数只是下限，未知不等于 0。缓存率变动只能提示排查，不能无证据
     断言前缀损坏；不同 provider 未必提供可比的 cache usage。
   - inbox 有待处理条目时**必须显著提醒**：这需要人。
   - 用户要更多细节时，可读该任务目录中的 `retro.md`、`metrics.jsonl`、
     `orchestrator.log` 再讲。

用户询问实际模型或配置来源时，调用
`flywheel config inspect --cwd <绝对仓库路径> --task-dir <绝对任务路径> --json`。
区分模型 profile、harness、DSH provider 和账号 channel；别名不是授权证明。
需要排查安装时调用 `flywheel doctor`，SDK 探测显式加 `--probe`，不要自动试调用付费模型。
