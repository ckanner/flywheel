---
name: flywheel-garden
metadata:
  version: "1.3.1"
description: Analyze retro.md failure patterns and knowledge-asset decay (AGENTS.md/skills), producing garden-proposals.md for human review — proposals only, never auto-applied. Use when the user says "/skill:flywheel-garden" or wants to distill loop learnings into skill proposals.
whenToUse: 当用户想把 retro 里反复出现的失败模式提炼成 skill/验收规则提案、或清理 AGENTS.md/skills 腐烂内容时使用。
compatibility: "Requires the Flywheel CLI and configured Garden harness; default Kimi CLI with subscription login."
arguments: slug
---

# flywheel-garden — 知识资产维护提案（外挂模块）

Button only: validate + invoke CLI + translate. No loop logic
here. Garden **只产提案，绝不自动应用**——改的是 loop 自己的知识资产，
应用权在人。

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 存在。`retro.md` 为空时提醒用户：提案原料
   主要是 retro 积累，空 retro 也能跑（只做腐烂检查）但价值有限。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel garden "$slug"
   ```

   若 `flywheel` 不在 PATH，使用已配置的绝对二进制路径；未安装时按项目 README 安装所需版本。DSH 功能须使用包含该实现的构建；版本与安装步骤见项目 README，不能假定旧 npm 发布包已支持。

3. **翻译结果**，用人话讲：
   - retro 中反复出现（≥2 次）的失败模式有哪些、各几次；
   - `docs/<slug>/garden-proposals.md` 里每条提案：类型（失败模式/腐烂清理）、
     证据引用、建议动作；
   - 提案校验未过（缺证据引用）时明确指出：原始输出仍在文件里，但证据不足
     的提案不该应用；
   - 提醒：审完提案后**由人手动**改 AGENTS.md/skills/契约模板；改完可考虑
     把高频教训沉淀进 `/skill:flywheel-new` 的生成习惯。
