# Flywheel — 新 Session 交接上下文

> 给接续工作的 agent：你在 `~/Documents/projects/flywheel/`（git 主线 main）。
> 仓库为 **Go 版全量实现**（2026-08-22 迁移完成）：cmd / internal / templates / skills / scripts。
> **第一步先通读 `DESIGN.md`（v0.21，约 500 行）**，它是唯一权威设计文档。§2 的十条设计铁律是最高约束，任何实现决策不得与之冲突。

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
- **三个主 loop**：Plan = kimi code + K3（长驻会话）；Execute = zcode + GLM-5.3（每单元全新会话，app-server 协议驱动）；Review = Codex + GPT-6-Astra（全新会话）。Judge 主判 = kimi code + K3（fresh `-p`），双面板复核 = Codex + GPT-6-Astra。
- **全部模型 reasoning/thinking=max**；会话级模型隔离；跨角色只传文本契约。
- **单元级 worktree 隔离**（v0.16）：worktree 在仓库旁 `.flywheel-worktrees/`，ff-merge 回主 checkout，账本提交限定 docs/<slug>/；不可用回退原地执行。
- **state.json 只由编排器写入**：CAS revision、tmp+rename 原子写；agent 只写 done-report。
- **无容器沙箱**：本机直跑；git diff 事后判定边界；executor 环境白名单注入低价值 key。
- **渠道（v0.22 三厂商定稿）**：无 fallback provider——zcode 通道（Execute default/multimodal/Intake）走 Z.AI Coding Plan（凭证 `~/.zcode/cli/config.json` 用户侧，零 key 注入）；Codex 通道（Review/面板）走 chatgpt 订阅 OAuth（`codex login`）；Kimi 系（Plan/Garden/Judge 主判/frontend/hard）走 kimi 订阅 OAuth。omp+智谱国内计划退居 B 通道（协议漂移应急）。RATE_LIMIT 同渠道退避重试、QUOTA 直接升级 inbox。

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
- **zcode**（v0.22 主执行 harness）：driver 默认 `node /Applications/ZCode.app/Contents/Resources/glm/zcode.cjs app-server`（app 内置官方 runtime，随桌面自动更新；env `FLY_ZCODE_BIN` 可覆盖为任意 launcher）。Z.AI Coding Plan 已在 `~/.zcode/cli/config.json` 配好（provider zai）；app-server 协议 **live 冒烟 2026-09-10 通过**（create/send/事件流/session/read 全闭环，8.2s）。协议无公开文档——桌面大版本更新后跑协议探针（§10-5）。
- **codex**（v0.22 Review/面板 harness）：npm CLI 已升 **0.154.0**（2026-09-10；gpt-6-astra 硬门槛 ≥0.154/内置≥0.153，live 验证通过）；chatgpt 订阅 OAuth 已登录（auth.json）。注意代理经 HKG 节点偶发 403，重试即过。
- **kimi code**：订阅 OAuth 登录在；stream-json 无 usage 暴露：Plan 的 token 不可观测（接受现状，看门狗兜底）。
- **omp（B 通道）**：已装、`zhipu-coding-plan` provider 配好（智谱国内计划）——zcode 协议漂移时的应急通道。
- git remote origin → github.com/ckanner/flywheel（ssh）；gh 已登录 ckanner。

## 下一步（v0.22 落地后）

1. 用真实任务（mimir 头部 S4-0/S0-00/M1-1）做首次实战演练，按 §10 实证门槛逐项验证（v0.22 新增第 5/6 条：zcode 协议稳定性、Codex 首跑清单）。
2. 验证 Codex exec 线程在 ChatGPT 桌面侧边栏的呈现（注册已通，即时呈现待 app 重启复核）。
3. goreleaser 发布流程首跑（tag → release → npm 包 @ckanner/flywheel 安装冒烟）。
4. retro.md 积累后跑 `flywheel garden`，检验知识资产维护循环。

## 决策原则

**质量/效率/速度优先，成本不计入**（2026-08-20 用户确立）。模型/通道选型、架构取舍不再权衡价格因素。
