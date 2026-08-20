---
name: flywheel-intake
description: Scan task sources (GitHub issues / custom commands) into the triage inbox and get LLM triage advice — candidates never become tasks without human approval. Use when the user says "/skill:flywheel-intake" or wants to discover new flywheel task candidates.
compatibility: Requires the flywheel CLI (Node.js >=20; run "npm link" in the flywheel repo, or fall back to node <flywheel-repo>/bin/flywheel.js) and pi >=0.84 (triage runs V4 Flash); github-issues sources also need the gh CLI.
whenToUse: 当用户想扫描任务源（issue/CI 失败/依赖升级）发现候选任务、或刷新 triage 分诊建议时使用。
arguments:
  - name: slug
    description: 任务目录路径，如 docs/<slug>
    required: true
  - name: source
    description: 可选，只扫 intake.sources 中名为此的源
    required: false
---

# flywheel-intake — 扫任务源进 triage（外挂模块，§4.5）

Button only (DESIGN.md §13): validate + invoke CLI + translate. No loop logic
here — and **no freezing**: Intake 只产候选与分诊建议，冻结需求永远走
`/skill:flywheel-new` + 人审（§12）。

## 步骤

1. **校验**：`docs/<slug>/` 存在且 `flywheel.config.json` 里有
   `intake.sources` 配置。如果用户给了源名，确认它在 sources 里。
2. **调用**（在目标仓库根执行，前台一把即返回）：

   ```bash
   # 扫全部源
   flywheel intake docs/<slug>
   # 只扫一个源
   flywheel intake docs/<slug> --source ci-failures
   ```

   未 `npm link` 时：`node <flywheel-repo>/bin/flywheel.js intake docs/<slug> [--source ci-failures]`（<flywheel-repo> = 本 skill 所在仓库根，可由 ${KIMI_SKILL_DIR} 向上两级定位；注意 ${KIMI_SKILL_DIR} 可能指向软链路径本身，建议优先 npm link）。

3. **翻译结果**，用人话讲：
   - 扫到多少候选、多少是新的、多少因指纹重复被跳过；
   - 每条新候选的分诊建议：建议接 / 建议不接 / 需人判断 + 一句话理由；
   - 明确告诉用户：**候选不是任务**。要接的候选，到
     `docs/<slug>/triage-inbox.md` 把该条的 `- approval:` 改成 `approved`，
     然后用 `/skill:flywheel-new` 从 approved 候选生成契约（仍需人审冻结）。
