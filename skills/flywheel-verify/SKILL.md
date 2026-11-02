---
name: flywheel-verify
metadata:
  version: "1.5.0"
description: Trigger only the verification gates (L1/L2/L3) for a Flywheel task — no model involved, pure deterministic commands. Use when the user says "/skill:flywheel-verify docs/<slug> [packet]" or wants to re-run acceptance checks.
whenToUse: 当用户只想重跑确定性验收（L1/L2/L3 命令，无模型参与）时使用。
compatibility: "Requires the Flywheel CLI. No model call; verification commands run under the engine watchdog."
arguments: slug packet

---

# flywheel-verify — 只触发验证闸门

Button only: validate + invoke CLI + translate. No loop logic
here — and no model either: verification is the orchestrator running commands.

> 参数经 kimi 命名参数注入正文：`$slug`、`$packet`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 只验某个 packet 的 L1
   flywheel run "$slug" --only verify --packet "$packet"
   # 或不指定：全部未完成 packet 的 L1 + 当前里程碑 L2/L3 闸门
   flywheel run "$slug" --only verify
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：每条命令的退出码与通过与否、git diff 边界判定
   （must-not-touch / protected-test-patterns / max-files）、里程碑闸门结论。任何红色都
   原样报告命令与输出摘要——命令是 ground truth，不替它找理由。

## 注意

- 不带 packet 的 verify 覆盖**所有里程碑**里未完成 packet 的 L1，不只当前
  里程碑；L2/L3 闸门仍只跑当前里程碑。
