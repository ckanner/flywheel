---
name: flywheel-intake
metadata:
  version: "1.5.0"
description: Scan task sources (GitHub issues / custom commands) into the triage inbox and get LLM triage advice — candidates never become tasks without human approval. Use when the user says "/skill:flywheel-intake" or wants to discover new flywheel task candidates.
whenToUse: 当用户想扫描任务源（issue/CI 失败/依赖升级）发现候选任务、或刷新 triage 分诊建议时使用。
compatibility: "Requires the Flywheel CLI and configured Intake harness; default DSH DeepSeek Flash. GitHub issue sources also need gh."
arguments: slug source

---

# flywheel-intake — 扫任务源进 triage（外挂模块）

Button only: validate + invoke CLI + translate. No loop logic
here — and **no freezing**: Intake 只产候选与分诊建议，冻结需求永远走
`/skill:flywheel-new` + 人审。

> 参数经 kimi 命名参数注入正文：`$slug`、`$source`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 存在且 `flywheel.config.json` 里有
   `intake.sources` 配置。如果用户给了源名，确认它在 sources 里。
2. **调用**（在目标仓库根执行，前台一把即返回）：

   ```bash
   # 扫全部源
   flywheel intake "$slug"
   # 只扫一个源
   flywheel intake "$slug" --source "$source"
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**，用人话讲：
   - 扫到多少候选、多少是新的、多少因指纹重复被跳过；
   - 每条新候选的分诊建议：建议接 / 建议不接 / 需人判断 + 一句话理由；
   - 明确告诉用户：**候选不是任务**。要接的候选，到
     `docs/<slug>/triage-inbox.md` 把该条的 `- approval:` 改成 `approved`，
     然后用 `/skill:flywheel-new` 从 approved 候选生成契约（仍需人审冻结）。
