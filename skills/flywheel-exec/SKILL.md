---
name: flywheel-exec
metadata:
  version: "1.6.0"
description: Run the bounded Execute Loop for a Flywheel task, optionally for one packet, until L1 passes or a failure limit is reached. Use when the user says "/skill:flywheel-exec docs/<slug> [packet]" or wants to run just one execution unit.
whenToUse: 当用户只想手动跑某个执行单元（调试单个 packet、重跑失败单元）时使用。
compatibility: "Requires Flywheel >=0.28.0 and configured route harnesses: DSH Flash for default, Kimi CLI for frontend, the bundled Codex Pro machine worker for hard tasks."
arguments: slug packet

---

# flywheel-exec — 只触发 Execute Loop

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`、`$packet`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

`slug` 表示实际任务目录路径，例如 `docs/my-task` 或绝对路径；不是需要自动拼接 `docs/` 的短名称。先将用户输入解析为已经存在的目录，后续引用同一路径。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。若给了 packet id（如 M02-T03），
   确认对应 `task-packet-*.md` 存在。
   首次运行或升级后用 `flywheel config inspect --cwd <仓库绝对路径> --task-dir <任务绝对路径> --json`
   查看有效路由。0.28.0 的 hard 默认是 `codex-pro`，直接使用已安装插件的 machine API；
   旧配置不自动迁移。不要在 skill 中改用 `codex exec` 或启动父模型再次委派，
   也不要把 `subagent_codex_pro` 写成 DSH provider。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 指定 packet
   flywheel run "$slug" --only execute --packet "$packet"
   # 或不指定：取当前里程碑下一个未完成 packet
   flywheel run "$slug" --only execute
   ```

   若 `flywheel` 不在 PATH，使用已安装的 `~/.config/flywheel/npm-launcher` 或用户显式配置的绝对路径。首次安装使用 `npx -y @ckanner/flywheel@latest install --json`，核对结果 `ok` 与每一步；`ready:false` 表示仍需处理模型依赖或配置。安装不证明账号已授权，预检不调用付费模型。

3. **翻译结果**：该子环内的执行尝试、done-report、L1 验收与失败反馈；
   成功会报告 `L1 green (execute loop closed)`，不是只生成代码就返回。
   失败区分预算／凭证停止、尝试次数用尽、相同失败空转和实际错误。
   token／成本缺失要说明未知，不能记成免费。提醒：还需要独立 Review
   和引擎持久化交付；该命令不会自动运行 Review 或把单元标为 completed。
   达到失败出口后由用户选择修订 Plan 或重试，不在 skill 中另建重试循环。
