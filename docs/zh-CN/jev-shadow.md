# 可选 JEV 影子评估

[文档目录](../README.zh-CN.md) · [English](../en/jev-shadow.md) · 简体中文

JEV 是可选实验：读取有界任务摘要，记录工作流建议，不决定生产任务的实际模式。
日常使用先阅读[工作流指南](workflow.md)；需要为配置的 endpoint 收集比较证据时，
再启用影子评估。

## 开关与行为

JEV **默认关闭**。旧配置不含 `jev-shadow` 也等同关闭：不读 JEV 密钥、不发请求、不写影子记录，仍按原来的 Go 规则选 Quick／Full。它不是编排器必需的依赖。

桌面进入 **Flywheel → 配置**，选择全局或项目范围，再打开 **JEV 影子建议**。
配置会自动读取；核对 endpoint 后保存。也可将下面的片段合并到实际生效的完整
`flywheel.config.json` 中：

```json
{
  "jev-shadow": {
    "enabled": false,
    "endpoint": "https://merouter.play.one2x.ai/v1/systemone",
    "model": "jev",
    "expected-model": "jev-1.14.0",
    "key-env": "MEROUTER_API_KEY",
    "timeout-ms": 5000
  }
}
```

公用模板不预置某个组织的 endpoint。密钥只放在环境变量中，配置填变量**名称**。桌面启动的引擎必须能继承这个变量；终端里存在变量不代表桌面能读取。不要把密钥填进 endpoint 或模型字段。

配置仍按任务 > 项目 > 全局 > 内置选择一个完整文件，不跨文件合并。`config inspect --json` 和桌面配置编辑器可检查所选文件。保存影响下次启动，不追改运行中的进程。可以随时关闭，已有实验记录保留。

开启的含义是允许向所填 endpoint 发送新任务的目标、需求／验收描述和规划依据摘要，私有目录另存该摘要。不会读取源码、diff、运行日志或 Provider 对话。会去除 URL 和常见用户目录前缀，并拒绝常见密钥模式；这不能保证任意业务文本无敏感信息，请按自己的数据要求决定是否开启。

| 情况 | 行为 |
|---|---|
| 开关关闭／旧配置缺省 | 原流程，零 JEV 请求 |
| 新任务首次普通 `run`／桌面启动 | 原模式先保存；最多观察一次，至多等待 5 秒，无自动重试 |
| 恢复、重试、单阶段操作 | 不补发观察请求 |
| 缺少密钥、endpoint 无效、超时、401、服务故障、记录故障 | 提示跳过／弃权；主流程继续，实际模式不变 |
| 显式 Full 或结构条件不允许 Quick | 保留确定性阻断，不调用模型 |
| 硬预算任务 | 跳过自动观察，避免在硬预算外产生实验用量 |
| 用户停止任务 | 取消影子请求并沿原规则停止任务，不能吞掉停止信号 |

即使 JEV 建议 Quick，实际 Full 也不会降级。影子评估不改模型路由、权限、Session、验收、PR 或 Merge 门禁。每次请求的用量与耗时独立记录，不混入主任务预算，不把缺失用量当零，不承诺账单节省。

## 实验与报告

自动观察保存到 `~/.config/flywheel/shadow/<目标摘要>/<输入摘要>/`，目录 0700，文件 0600。日志会给出具体目录。`cases.json` 为有界摘要；`target.json` 固定 endpoint、模型、问题和策略；调用记录有原始概率、建议、实际基线、响应版本及用量。只有派发意图而没有终态时保持 unknown，同目录不会重发；更换目录意味着显式新实验。

使用提供 `shadow prepare|run|report` 命令的 Flywheel 版本：独立实验是显式操作，即使自动开关关闭仍可使用；它不会启动或改动任务。

```bash
# 为尚未运行或已停止的任务准备摘要，先审阅文件内容。
flywheel shadow prepare --task-dir docs/my-task > /tmp/jev-cases.json

# 显式发送已审阅摘要；环境变量 MEROUTER_API_KEY 由你自己的环境提供。
flywheel shadow run --input /tmp/jev-cases.json \
  --directory /tmp/my-jev-experiment \
  --endpoint https://merouter.play.one2x.ai/v1/systemone \
  --max-calls 20 --timeout 5s

# 离线报告；不需要密钥或网络。
flywheel shadow report --input /tmp/jev-cases.json \
  --directory /tmp/my-jev-experiment
```

自动捕获的报告把 `--input` 指向对应目录的 `cases.json`。要补标签，请复制摘要到另一个文件再编辑 `label`：`mode: quick|full`、`status: provisional|confirmed`、`author`、`reason`。标签不发送模型，也不改变预测缓存。别修改自动目录里的原始摘要。

数据集最多 100 项、1 MiB；单请求最多 64 KiB；显式命令的 `--max-calls` 为 1–100，单次 `--timeout` 最多 30 秒。新请求超过上限时整批拒绝。HTTP 不重定向、不重试，不记录错误响应正文。输入错误退出 1；含失败、unknown 或未运行项退出 3；用户取消退出 143；成功报告退出 0。`prepare|run|report` 的 stdout 均为单份 JSON，错误写 stderr。

## 如何解释结果

第一版只使用 8 个有具体判断标准的 Noul 问题。正向概率都 ≥0.8、风险概率都 ≤0.2 才给 Quick 候选；任一风险 ≥0.8 给 Full；其余 abstain。问题、阈值和版本写入 manifest，不把 Noul 概率包装为“整体正确率”。响应版本必须符合 `expected-model`；MeRouter alias 升级时会弃权，需显式审查后改固定版本。

这些阈值来自初步实验，**尚未生产校准**。报告将决策时快照、准备快照、旧任务回放、历史摘要和合成样本分别统计；旧任务当前模式不一定是最初的 Auto 决策。人工标签缺失或 provisional 时，不报告已校准误放率。现有 Auto 已使用准备阶段的契约事实，影子调用是新增费用，不能宣称省掉了一次原本不存在的分类调用。

第一批[六个历史工作项](../evaluation/jev-historical-provisional.json)引用已完成提交，实际模式均标 unknown，人工标签为 provisional。它适合验证接口和找反例，不能证明生产收益。升级为真正自动路由前，需要独立确认的决策时样本、误放率与弃权率评估；当前没有 JEV 生产路由开关。
