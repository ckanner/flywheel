# Flywheel — 新 Session 交接上下文

> 给接续工作的 agent：你在 `~/Documents/projects/flywheel/`（git 主线 main）。
> 仓库为 **Go 版全量实现**（2026-08-22 迁移完成）：cmd / internal / templates / skills / scripts。
> **第一步先通读 `DESIGN.md`（v0.17，约 500 行）**，它是唯一权威设计文档。§2 的十条设计铁律是最高约束，任何实现决策不得与之冲突。

## 项目一句话

Flywheel（飞轮）是一套 loop engineering 系统：外部编排器进程（不是 agent）驱动多个 agent loop（规划/执行/评审），接收已冻结的需求，自动完成软件任务的规划、实现、评审与分层验证，全程无人值守，异常升级给人。

## 构建与验收（唯一闸门）

```bash
go build ./... && make quality-gate   # vet + fmt-check + 覆盖率 100% 门禁
```

- Go >= 1.27；依赖 cobra/yaml.v3/jsonschema v6/testify（`go.mod` 为准）。
- **覆盖率 100% 是硬门禁**：反向驱动可测性（sleep/now/env/log 注入缝）；禁魔改单测绕过；不可达防御分支删代码+注释。
- skills frontmatter 规范由 internal/skillsembed 测试强制（真 YAML 解析 + name/version/description/whenToUse/arguments），CI 阻断。

## 已定型的关键决策（细节全在 DESIGN.md）

- **编排器是确定性 Go CLI**（cobra 命令树），不是 skill、不是 agent；skill 只是触发按钮。
- **三个主 loop**：Plan = kimi code + K3（长驻会话）；Execute = pi + V4 Flash（每单元全新会话）；Review = pi + V4 Pro（全新会话）。Judge 主判 = kimi code + K3（fresh `-p`），双面板复核 = pi + V4 Pro。
- **全部模型 reasoning_effort=max**；会话级模型隔离；跨角色只传文本契约。
- **单元级 worktree 隔离**（v0.16）：worktree 在仓库旁 `.flywheel-worktrees/`，ff-merge 回主 checkout，账本提交限定 docs/<slug>/；不可用回退原地执行。
- **state.json 只由编排器写入**：CAS revision、tmp+rename 原子写；agent 只写 done-report。
- **无容器沙箱**：本机直跑；git diff 事后判定边界；executor 环境白名单注入低价值 key。
- **渠道（v0.19 收敛）**：无 fallback provider——Kimi 系全走 kimi code 订阅 OAuth，DeepSeek 系只走 OpenCode Go；RATE_LIMIT 同渠道退避重试、QUOTA 直接升级 inbox。Judge 主判 = kimi code + K3 fresh `-p`；双面板复核 = pi + V4 Pro。

## 审计修复已全部落地（v0.17，勿回退）

P1 kill switch（RunTask 注册 SIGTERM → 杀子进程组 → quiescence → exit 143）；skills frontmatter
YAML 合法化 + 真 YAML 门禁；P2 in-place base-ref 重锚、空转检测用 L1 权威 diff、先 commit 再
completed、resume 补缺陷注入；P3 日志走 stderr、install-skills actionText 含 stale 等。

## v0.18（2026-08-24，PR #3 待合）

全量评审修复：里程碑 Judge 双 judge 面板（config `judge-panel`，分歧→inbox）；plan 修订
acceptance 指纹留痕；P2×3 与 C 级小修批量；SKILL.md 规范订正（arguments 字符串形态 +
正文占位符，版本 bump 1.1.0；frontmatter 归属按 kimi/pi/Warp/agentskills.io 官方文档精确化）；
Makefile 覆盖率门禁收紧为逐函数 100%。评审台账见 REVIEW-FIXPLAN.md。**尚未首跑**——mimir
S4-0 仍是下一步。

## 本机环境事实

- macOS arm64；Go 工具链经 mise 管理（默认 go@1.27.0）。⚠️ shell PATH 硬编码了旧版 go 1.26 路径——新会话先 `go version` 核对，不对则 `eval "$(mise env)"`。
- pi 0.84.0 已装；`OPENCODE_API_KEY`、`KIMI_API_KEY` 在环境变量；`~/.pi/agent/models.json` 有 maxTokens=131072 覆盖（删掉或换机需重建）。
- kimi stream-json 无 usage 暴露：Plan 的 token 不可观测（接受现状，看门狗兜底）。
- git remote origin → github.com/ckanner/flywheel（ssh）；gh 已登录 ckanner。

## 下一步（迁移完成后）

1. 用真实任务（mimir 头部 S4-0/S0-00/M1-1）做首次实战演练，按 §10 实证门槛逐项验证。
2. goreleaser 发布流程首跑（tag → release → npm 包 @ckanner/flywheel 安装冒烟）。
3. retro.md 积累后跑 `flywheel garden`，检验知识资产维护循环。

## 决策原则

**质量/效率/速度优先，成本不计入**（2026-08-20 用户确立）。模型/通道选型、架构取舍不再权衡价格因素。
