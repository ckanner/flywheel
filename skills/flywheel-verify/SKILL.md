---
name: flywheel-verify
metadata:
  version: "1.0.0"
description: Trigger only the verification gates (L1/L2/L3) for a Flywheel task — no model involved, pure deterministic commands. Use when the user says "/skill:flywheel-verify docs/<slug> [packet]" or wants to re-run acceptance checks.
whenToUse: 当用户只想重跑确定性验收（L1/L2/L3 命令，无模型参与）时使用。
compatibility: "Requires the flywheel CLI (single static binary; make build + make install in the flywheel repo, or install via npm). No harness CLI needed — verification is model-free."
arguments: slug packet

---

# flywheel-verify — 只触发验证闸门

Button only: validate + invoke CLI + translate. No loop logic
here — and no model either: verification is the orchestrator running commands.

> 参数经 kimi 命名参数注入正文：`$slug`、`$packet`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 只验某个 packet 的 L1
   flywheel run "$slug" --only verify --packet "$packet"
   # 或不指定：全部未完成 packet 的 L1 + 当前里程碑 L2/L3 闸门
   flywheel run "$slug" --only verify
   ```

   若 flywheel 不在 PATH：`flywheel run docs/<slug> --only verify [--packet M02-T03]`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不适用就直接问用户要仓库路径）。
   （Go 版为单二进制：`make build && make install` 或经 npm 安装后 `flywheel` 即在 PATH。）

3. **翻译结果**：每条命令的退出码与通过与否、git diff 边界判定
   （must-not-touch / 测试文件 / max-files）、里程碑闸门结论。任何红色都
   原样报告命令与输出摘要——命令是 ground truth，不替它找理由。

## 注意

- 不带 packet 的 verify 覆盖**所有里程碑**里未完成 packet 的 L1，不只当前
  里程碑；L2/L3 闸门仍只跑当前里程碑。
