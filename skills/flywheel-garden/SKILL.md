---
name: flywheel-garden
version: 1.0.0
description: Analyze retro.md failure patterns and knowledge-asset decay (AGENTS.md/skills), producing garden-proposals.md for human review — proposals only, never auto-applied. Use when the user says "/skill:flywheel-garden" or wants to distill loop learnings into skill proposals.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js) and kimi CLI (garden runs K3).
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
---

# flywheel-garden — 知识资产维护提案（外挂模块）

Button only: validate + invoke CLI + translate. No loop logic
here. Garden **只产提案，绝不自动应用**——改的是 loop 自己的知识资产，
应用权在人。

## 步骤

1. **校验**：`docs/<slug>/` 存在。`retro.md` 为空时提醒用户：提案原料
   主要是 retro 积累，空 retro 也能跑（只做腐烂检查）但价值有限。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel garden docs/<slug>
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js garden docs/<slug>`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不可用就问用户。建议优先 npm link）。

3. **翻译结果**，用人话讲：
   - retro 中反复出现（≥2 次）的失败模式有哪些、各几次；
   - `docs/<slug>/garden-proposals.md` 里每条提案：类型（失败模式/腐烂清理）、
     证据引用、建议动作；
   - 提案校验未过（缺证据引用）时明确指出：原始输出仍在文件里，但证据不足
     的提案不该应用；
   - 提醒：审完提案后**由人手动**改 AGENTS.md/skills/契约模板；改完可考虑
     把高频教训沉淀进 `/skill:flywheel-new` 的生成习惯。
