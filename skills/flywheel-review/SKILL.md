---
name: flywheel-review
metadata:
  version: "1.5.0"
description: Trigger only the Review Loop for a Flywheel task, optionally for one unit (single state transition). Use when the user says "/skill:flywheel-review docs/<slug> [unit]" or wants to re-run just the review of a unit.
whenToUse: 当用户只想重跑某单元的 LLM 评审（L1 已绿之后）时使用。
compatibility: "Requires the Flywheel CLI and the configured read-only Review harness; default DSH with the ChatGPT provider plugin."
arguments: slug unit

---

# flywheel-review — 只触发 Review Loop

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`、`$unit`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全；目标单元的 **L1 已通过**
   （引擎会同时核对 `evidence.l1.pass` 和当前候选/契约/配置身份）——评审只在确定性
   验收全绿之后开口。若给了 unit id，确认 packet 存在。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 重跑某单元评审
   flywheel run "$slug" --only review --unit "$unit"
   # 或不指定：当前里程碑第一个未完成 packet（L1 未绿会报错——先 execute/verify）
   flywheel run "$slug" --only review
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：verdict（pass / return / verification-gap）、分数、理由；
   return 时转述具体修改要求、失败证据、涉及位置、完成条件；
   verification-gap 时说明该单步只返回裁决，不会自动跨环修订；由用户运行
   Plan 修订允许范围内的契约，再 verify，或恢复完整 run 由引擎处理闭环。
   目录外冻结 oracle 的修改仍需明确任务；不得为了通过直接修改它。
