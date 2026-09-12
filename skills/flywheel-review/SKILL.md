---
name: flywheel-review
metadata:
  version: "1.2.0"
description: Trigger only the Review Loop for a Flywheel task, optionally for one unit (single state transition). Use when the user says "/skill:flywheel-review docs/<slug> [unit]" or wants to re-run just the review of a unit.
whenToUse: 当用户只想重跑某单元的 LLM 评审（L1 已绿之后）时使用。
compatibility: "Requires the flywheel CLI (single static binary from the GitHub release, or make build + make install in the flywheel repo) and the harness CLIs the loops use: pi >=0.84 and/or kimi CLI."
arguments: slug unit

---

# flywheel-review — 只触发 Review Loop

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`、`$unit`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全；目标单元的 **L1 已通过**
   （`state.json` 中该 packet 的 `evidence.l1.pass === true`）——评审只在确定性
   验收全绿之后开口。若给了 unit id，确认 packet 存在。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 重跑某单元评审
   flywheel run "$slug" --only review --unit "$unit"
   # 或不指定：当前里程碑第一个未完成 packet（L1 未绿会报错——先 execute/verify）
   flywheel run "$slug" --only review
   ```

   若 flywheel 不在 PATH：`flywheel run docs/<slug> --only review [--unit M02-T03]`（flywheel 应已在 PATH 上——npm 全局安装或开发机 go install；不在则先安装，不要绕道源码仓库）。
   （Go 版为单二进制：`make build && make install` 或经 npm 安装后 `flywheel` 即在 PATH。）

3. **翻译结果**：verdict（pass / return / verification-gap）、分数、理由；
   return 时转述具体修改要求、失败证据、涉及位置、完成条件；
   verification-gap 时说明编排器会加强验收后复验而不是退回执行。
