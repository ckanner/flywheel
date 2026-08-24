---
name: flywheel-status
metadata:
  version: "1.1.0"
description: Read-only progress report for a Flywheel task — reads state.json/retro.md/metrics and explains progress in plain language. Use when the user says "/skill:flywheel-status docs/<slug>" or asks how a flywheel task is doing.
whenToUse: 当用户询问任务进度、成本、缓存命中率、是否停滞或有 inbox 待办时使用（只读，不改状态）。
compatibility: "Requires the flywheel CLI (single static binary; make build + make install in the flywheel repo, or install via npm)."
arguments: slug
---

# flywheel-status — 看进度（只读）

Button only: read state + translate. Writes nothing.

> 参数经 kimi 命名参数注入正文：`$slug`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 步骤

1. **校验**：`docs/<slug>/state.json` 存在（不存在 = 任务未生成或未启动）。
2. **调用**（在目标仓库根执行）：

   ```bash
   flywheel status "$slug"
   ```

   若 flywheel 不在 PATH：`flywheel status docs/<slug>`（<flywheel-repo> 定位：install-skills 在 skills 安装点写了锚文件，`cat ${KIMI_SKILL_DIR}/../flywheel-home.txt` 即仓库根；锚文件不存在说明 skill 直接来自仓库内，${KIMI_SKILL_DIR} 向上两级即仓库根；都不适用就直接问用户要仓库路径）。
   （Go 版为单二进制：`make build && make install` 或经 npm 安装后 `flywheel` 即在 PATH。）

3. **翻译结果**，用人话讲：
   - 任务状态、编排器是否存活（pid）、当前里程碑、上次推进时间——
     推进时间久未更新 = 可能停滞/断链，提醒用户。
   - 每个 packet：状态、尝试/失败次数、三级证据（executor 自报 < L1 确定性
     通过 < Review  verdict——证据等级不同，讲进度时区分）。
   - token/成本累计、缓存命中率（突降 = 前缀被破坏，提醒）、预算档与告警。
   - inbox 有待处理条目时**必须显著提醒**：这需要人。
   - 用户要更多细节时，可读 `docs/<slug>/retro.md`、`metrics.jsonl`、
     `orchestrator.log` 再讲。
