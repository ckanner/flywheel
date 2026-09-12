# Flywheel（飞轮）

Loop Engineering 系统：外部编排器进程（不是 agent）驱动多个 agent loop（规划/执行/评审/终验），接收已冻结的需求，自动完成软件任务的规划、实现、评审与分层验证，全程可无人值守，异常时升级给人。

- 编排器 = 确定性 Go 代码（机械状态转移，设计铁律 1）
- 角色与知识 = Agent Skills（本仓库 `skills/`，只是"按钮"）
- 契约 = `docs/<slug>/` 下的静态文件（spec.md + task-packet + state.json，全部进 git）

权威设计文档：**[docs/design.md](docs/design.md)**（v0.22，变更记录在文件头）。仓库工作宪法：[AGENTS.md](AGENTS.md)。

## 安装（npm，平台二进制包模式）

```bash
npm i -g @ckanner/flywheel     # 平台二进制经 optionalDependencies 自动就位，无 postinstall 下载
flywheel install-skills        # skills 物化 + 全局配置 ~/.config/flywheel/flywheel.config.json 一次就位
```

- 安装完全自包含：契约 schema、skills 快照、§13 法定配置表全部内嵌二进制，**无需 clone 本仓库**；维护者与分用户用法完全一致。
- 支持平台：darwin/linux × amd64/arm64（`os`/`cpu` 字段自动选取，`--no-optional` 会导致平台包缺失）。
- 非 npm 渠道：[GitHub Releases](https://github.com/ckanner/flywheel/releases) 下载平台 tar.gz（`shasum -a 256 -c checksums.txt` 校验）。
- 开发机源码运行：`go install ./cmd/flywheel`；`flywheel install-skills --link` 可选软链源码树（仅维护者便利，非安装路径）。

## 本机环境事实（维护机，v0.22）

- macOS arm64；Go 工具链经 mise 管理（go@1.27.0）——shell PATH 可能残留旧版路径，新会话先 `go version` 核对。
- **zcode**：driver 默认 `node /Applications/ZCode.app/Contents/Resources/glm/zcode.cjs app-server`（app 内置 runtime，随桌面自动更新；`FLY_ZCODE_SERVER_BIN` 覆盖 launcher，`FLY_ZCODE_BIN` 只管桌面注册二进制——v0.22 语义拆分）。Z.AI Coding Plan 凭证在 `~/.zcode/cli/config.json`（provider zai）；app-server 协议 live 冒烟 2026-09-10 通过（8.2s 全闭环）；协议无公开文档——桌面大版本更新后跑协议探针（design.md §10-5）。
- **codex**：CLI 已升 0.154.0（gpt-6-astra 硬门槛 ≥0.154 npm / ≥0.153 app 内置）；chatgpt 订阅 OAuth 已登录。注意：代理经 HKG 节点偶发 403，重试即过。
- **kimi code**：订阅 OAuth 登录在；stream-json 无 usage 暴露——Plan 的 token 不可观测（接受现状，看门狗兜底）。
- **omp（B 通道）**：已装、`zhipu-coding-plan` provider 配好（智谱国内计划）——zcode 协议漂移时的应急通道。

## 安装 skills 到 kimi code / pi

kimi code、pi、Warp 均实现 Agent Skills 标准并扫描共享安装点 `~/.agents/skills/`：

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

> 调用形态按 harness 而异：kimi code 与 pi 为 `/skill:<name>`；Warp 为 `/<name>`；
> Claude Code 经 Skill 工具加载。`whenToUse`/`arguments` 是 kimi code 定义的字段
> （自动调用依据），不在 agentskills.io 标准内，其他 harness 会无害忽略。

装完后在 kimi code 或 pi 里用 `/skill:<name>` 调用，例如 `/skill:flywheel-run`。安装时会在 skills 安装点写入锚文件 `flywheel-home.txt`，skill 用它定位本仓库根。

## skills 一览（都只是按钮，不含 loop 逻辑）

| 调用 | 作用 |
|---|---|
| `/skill:flywheel-new` | 对话式任务生成器：调研仓库 → 问意图 → 生成契约 → 逐条试跑验收命令留证 → 停下等人审 |
| `/skill:flywheel-run` | 以独立进程拉起编排器（触发全部 loop）；可选 `pause=review` 监督冻结点 |
| `/skill:flywheel-plan` | 只触发 Plan Loop 单步 |
| `/skill:flywheel-exec` | 只触发 Execute Loop（可指定 packet） |
| `/skill:flywheel-review` | 只触发 Review Loop（可指定 unit） |
| `/skill:flywheel-verify` | 只触发确定性验证闸门 L1/L2/L3（无模型） |
| `/skill:flywheel-status` | 只读进度（state.json/指标/缓存命中率/inbox；推进超时→停滞提示） |
| `/skill:flywheel-stop` | 停止编排器进程，状态定格 |
| `/skill:flywheel-intake` | 外挂：扫任务源 → triage-inbox → LLM 分诊建议（不冻结需求） |
| `/skill:flywheel-garden` | 外挂：retro 失败模式 + 知识资产腐烂 → 提案（需人审） |

内置 skill 目录也可用 CLI 直接查看：`flywheel skills list` / `flywheel skills read <name>`（go:embed 内嵌，无需源码树）。

## 使用全流程（最小示例，详见 [docs/design.md](docs/design.md) §12）

```bash
# 1. 在 harness（kimi code 或 pi）里生成任务契约
/skill:flywheel-new 把"给网关加 /health 端点"做成 flywheel 任务
#    → 生成 docs/<slug>/{spec.md, task-packet-*.md, state.json, flywheel.config.json}
#    → skill 会把契约 git add + commit，然后停下

# 2. 人审契约（关键 checkpoint，不可省）：需求编号、验收命令、边界、试跑证据
#    （生成器已逐条试跑 L1 命令并把退出码/输出记入 spec 附表——审它别跳过）

# 3. 启动（在 harness 里扣扳机；编排器是独立进程，关窗不影响运行）
/skill:flywheel-run docs/<slug>            # 全自动
/skill:flywheel-run docs/<slug> review     # 监督模式：Review 前冻结，等你看完再续

#    单步玩法：--only execute / verify / review（与 pause-before 互斥）

# 4. 观察与干预（在哪都行，全是文件操作）
flywheel status docs/<slug>
flywheel stop docs/<slug>

# 5. 交付：分支/PR 留给人最终确认合并（外层循环归人）
```

单 loop 手动触发（旁路常开）：`flywheel run docs/<slug> --only execute --packet M02-T03`、`--pause-before review` 逐步监督。

## 目录结构

```
cmd/flywheel/            # main() 单行桩（os.Exit(cli.Execute())）
internal/
  cli/                   # cobra 命令树：run/status/stop/intake/garden/install-skills/skills
  machine/               # 状态机（DESIGN §6 刻意做薄）：runTask/runSingleStep/steps
  contract/              # 契约解析 + JSON Schema 校验单一信源（templates/*.schema.json）
  state/                 # state.json CAS 账本（tmp+rename 原子写）
  donereport/            # done-report 解析与格式校验
  exitcontract/          # 出口契约三正交量判定
  verify/                # L1/L2/L3 闸门 + git diff 边界 + worktree 隔离 + 单元提交
  watchdog/              # 双计时看门狗（流 idle + wall-clock；进程组 kill + quiescence）
  errs/                  # 错误分类/退避/降级集（RE2 等价改写）
  fallback/              # 渠道降级链（绑定表改写）
  circuitbreaker/        # 预算熔断（off|soft|hard）+ DC4 空转检测
  defectinjection/       # 缺陷注入抽检（DC3）
  metrics/               # JSONL 指标（模型/token/成本/缓存命中/prompt 哈希）
  prompts/               # 提示词构建（§8 缓存纪律：稳定前置/易变尾部）+ 裁决解析
  drivers/               # mock（测试）/ pi / omp（B 通道）/ zcode（app-server）/ codex / kimi（长驻会话）
  config/                # 法定配置表加载与校验
  childenv/              # 子进程环境白名单（凭证卫生 §9）
  ledger/                # 账本区路径分类（边界判定的单一信源）
  modules/               # 外挂：intake/garden/install-skills/module 共享上下文
  skillsembed/           # go:embed skills 数据层（skills list/read + 规范门禁测试）
templates/               # 契约模板与 schema（task-packet / spec / state 示例）
skills/                  # 10 个 SKILL.md（Agent Skills 标准）+ embed.go
scripts/                 # npm 分发安装器（lark-cli 模式）
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
orchestrator.pid / orchestrator.log   # 运行中的进程文件与日志
```

## 测试

```bash
make test            # go test ./... 全部包
make coverage        # 覆盖率门禁（cmd 桩除外，逐函数 100%）
make quality-gate    # vet + fmt-check + coverage（CI 同款）
```

单元测试覆盖率 100% 是硬门禁（用户确立的设计纪律：反向驱动松耦合与可注入性）；
v0.18 起按**逐函数**检查——任何函数 <100% 直接失败，不再只看总数。mock 驱动覆盖全部编排逻辑，
可在无凭证环境完整运行；verify/watchdog 用真实 git 与真实子进程组测试。

## 文档入口

- [docs/design.md](docs/design.md) — 权威设计文档（§2 设计铁律是最高约束）
- [AGENTS.md](AGENTS.md) — 仓库工作宪法（构建/闸门/硬契约）
- [drills/](drills/README.md) — 实证演练记录与环境事实基线
