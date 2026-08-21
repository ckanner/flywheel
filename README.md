# Flywheel（飞轮）

Loop Engineering 系统：外部编排器进程（不是 agent）驱动多个 agent loop（规划/执行/评审/终验），接收已冻结的需求，自动完成软件任务的规划、实现、评审与分层验证，全程可无人值守，异常时升级给人。

- 编排器 = 确定性 Node.js 代码（机械状态转移，设计铁律 1）
- 角色与知识 = Agent Skills（本仓库 `skills/`，只是"按钮"）
- 契约 = `docs/<slug>/` 下的静态文件（spec.md + task-packet + state.json，全部进 git）

权威设计文档：**[DESIGN.md](DESIGN.md)**（v0.15）。仓库工作宪法：[AGENTS.md](AGENTS.md)。会话交接上下文：[HANDOFF.md](HANDOFF.md)。

## 安装

```bash
# 依赖只有 yaml（其余全部 node: 内置模块），需要 Node.js >= 20
npm ci

# 方式一：全局 link 出 flywheel 命令
npm link

# 方式二：不 link，直接用 node 调用
node bin/flywheel.js --help
```

真实驱动依赖本机已装的 [pi](https://github.com/earendil-works/pi)（Execute/Review/Judge）与 kimi code CLI（Plan/Garden）；渠道凭证走环境变量（`OPENCODE_API_KEY` / `KIMI_API_KEY`）。测试与演练（mock 驱动）不需要任何凭证。

## 安装 skills 到 kimi code / pi

kimi code 与 pi 都实现 Agent Skills 标准，共享安装点 `~/.agents/skills/`：

```bash
# 默认：软链（仓库即唯一信源，改了即生效；幂等，重复运行报告"已安装，跳过"）
flywheel install-skills

# 替换已存在的条目
flywheel install-skills --force

# 复制快照（用于发布，而不是软链）
flywheel install-skills --copy

# 安装到某个项目根的 .agents/skills/（项目级）
flywheel install-skills --project /path/to/project
```

装完后在 kimi code 或 pi 里用 `/skill:<name>` 调用，例如 `/skill:flywheel-run`。安装时会在 skills 安装点写入锚文件 `flywheel-home.txt`，skill 用它定位本仓库根（软链/复制安装都有效）。

## skills 一览（都只是按钮，不含 loop 逻辑）

| 调用 | 作用 |
|---|---|
| `/skill:flywheel-new` | 对话式任务生成器：调研仓库 → 问意图 → 生成契约 → 停下等人审 |
| `/skill:flywheel-run` | 以独立进程拉起编排器（触发全部 loop） |
| `/skill:flywheel-plan` | 只触发 Plan Loop 单步 |
| `/skill:flywheel-exec` | 只触发 Execute Loop（可指定 packet） |
| `/skill:flywheel-review` | 只触发 Review Loop（可指定 unit） |
| `/skill:flywheel-verify` | 只触发确定性验证闸门 L1/L2/L3（无模型） |
| `/skill:flywheel-status` | 只读进度（state.json/指标/缓存命中率/inbox） |
| `/skill:flywheel-stop` | 停止编排器进程，状态定格 |
| `/skill:flywheel-intake` | 外挂：扫任务源 → triage-inbox → LLM 分诊建议（不冻结需求） |
| `/skill:flywheel-garden` | 外挂：retro 失败模式 + 知识资产腐烂 → 提案（需人审） |

## 使用全流程（最小示例，详见 DESIGN.md §12）

```bash
# 1. 在 harness（kimi code 或 pi）里生成任务契约
/skill:flywheel-new 把"给网关加 /health 端点"做成 flywheel 任务
#    → 生成 docs/<slug>/{spec.md, task-packet-*.md, state.json, flywheel.config.json}
#    → skill 会把契约 git add + commit，然后停下

# 2. 人审契约（关键 checkpoint，不可省）：过一眼需求编号、验收命令、边界

# 3. 启动（在 harness 里扣扳机；编排器是独立进程，关窗不影响运行）
/skill:flywheel-run docs/<slug>

# 4. 观察与干预（在哪都行，全是文件操作）
flywheel status docs/<slug>
flywheel stop docs/<slug>

# 5. 交付：分支/PR 留给人最终确认合并（外层循环归人）
```

单 loop 手动触发（旁路常开）：`flywheel run docs/<slug> --only execute --packet M02-T03`、`--pause-before review` 逐步监督。

## 目录结构

```
bin/flywheel.js          # CLI 薄入口（run/status/stop/intake/garden/install-skills）
orchestrator/
  core/                  # 编排器引擎：machine（状态机）、state（CAS 账本）、contracts、
                         # verify（L1/L2/L3 + git diff 判定）、watchdog（双计时看门狗）、
                         # errors（错误分类/退避）、exit-contract、done-report、
                         # circuit-breaker（熔断/空转）、defect-injection、metrics、
                         # prompts、config、fallback（降级链）
  modules/               # 外挂：intake（§4.5）、garden（§4.6）、module（共享调用上下文）、
                         # install-skills
  drivers/               # 可替换驱动：mock（测试）/ pi（真实 Execute/Review/Judge）/
                         # kimi（真实 Plan/Garden，长驻会话）
templates/               # 契约模板与 schema（task-packet / spec / state / config 示例）
skills/                  # 10 个 SKILL.md（Agent Skills 标准，frontmatter: name/description/compatibility/whenToUse/arguments）
tests/                   # node:test 全套（单元 + mock e2e + 真实冒烟 + 演练）
drills/                  # 实证演练记录（403 降级、缺陷注入）
```

## 任务目录（docs/<slug>/，目标仓库内，全部进 git）

生成时（/skill:flywheel-new）：`spec.md`（编号需求）+ `task-packet-*.md`（契约）+ `state.json`（初始状态）+ `flywheel.config.json`（法定配置表）。

运行后逐渐多出：

```
reports/<packet-id>.done.json   # 每个单元的 done-report（executor 自报，三级证据之一）
metrics.jsonl                   # 每次 spawn 一条：模型/harness/token/成本/缓存命中/耗时/错误码
retro.md                        # 单元完成行 + 里程碑汇总（§5.3 字段）
inbox.md                        # 升级叫人的入口（正常应为空）
triage-inbox.md                 # intake 候选（跑了 flywheel intake 才有）
garden-proposals.md             # garden 提案（跑了 flywheel garden 才有）
approvals/<packet-id>.json      # live 凭证任务的人工批准文件（需要时）
orchestrator.pid / orchestrator.log   # 运行中的进程文件与 nohup 日志
```

## 测试

```bash
npm test          # 全部：单元 + mock 端到端 + 真实 pi/kimi 冒烟（无凭证时自动跳过）
```

当前 206 个测试。真实冒烟需要本机 pi/kimi 与对应环境变量；mock 驱动覆盖全部编排逻辑，可在无凭证环境完整运行。另有 `npm run skill-format-check` 校验 skills frontmatter（进 CI）。

## 文档入口

- [DESIGN.md](DESIGN.md) — 权威设计文档（§2 设计铁律是最高约束）
- [AGENTS.md](AGENTS.md) — 仓库工作宪法（构建/闸门/硬契约）
- [HANDOFF.md](HANDOFF.md) — 新会话交接上下文
- [drills/](drills/README.md) — 实证演练记录与环境事实基线
