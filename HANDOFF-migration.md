# Flywheel Go 迁移 — 交接上下文（2026-08-21）

> 给接续的 agent：正在把 Flywheel 从 Node.js 全量移植到 Go（用户裁决，2026-08-21）。
> 本文件是迁移中途交接，**权威设计仍是 `DESIGN.md`（v0.16 讨论稿）**；现有 `HANDOFF.md` 描述的是 Node 实现的运行环境事实，迁移完成后将整体更新。

## 1. 用户要求（不可违背）

1. 按计划**一步一步完整实现** Go 迁移，要准确；**每完成一部分，阶段性自审（review → 修复 → 再 review 直到没问题）**。
2. 依赖与 Go **用尽可能最新的版本**，不必跟 lark-cli 的版本一致。
3. **单元测试覆盖率 100% 反向驱动可测性**：松耦合、高内聚、开闭原则；输入输出准确。**禁止魔改单测绕过覆盖率**。
4. 全部完成、确认与方案一致、都修好后，**更新完整文档**（DESIGN.md 升版本 + README + HANDOFF.md + AGENTS.md）。
5. Skills 规范要对照官方 spec（agentskills.io / openagentskills.dev / Warp docs: https://docs.warp.dev/agents/capabilities/skills/）+ lark-cli 的写法，检查现有 skills 是否合规并统一。
6. **不要直接提交/推送 main**：全部弄好后**开一个 PR** 供人完整 review。

## 2. 当前进度（已完成的代码全部在本机工作区，未提交 git）

### 已完成并测试绿（覆盖率 100%，`go test ./...` 全绿）
| 包 | 对应 Node 源 | 状态 |
|---|---|---|
| `cmd/flywheel/main.go` + `internal/cli/root.go`（仅 version 子命令） | bin/flywheel.js | 骨架可运行；全部子命令接线未做 |
| `internal/build` / `templates/schemas.go`（go:embed 单一信源） | — | 完成 |
| `internal/errs` / `config` / `ledger` / `childenv` / `fallback` | errors/config/ledger/child-env/fallback.js | ✅ 各 100% |
| `internal/contract` | contracts.js | ✅ 100%（含 P2 schema 断言、Slug 行为差异注释） |
| `internal/state` | state.js | ✅ 100%（CAS/tmp+rename/initState/interrupted/usage 记账） |
| `internal/donereport` | done-report.js | ✅ 100% |
| `internal/exitcontract` | exit-contract.js | ✅ 100% |
| `internal/metrics` | metrics.js | ✅ 100% |
| `internal/circuitbreaker` | circuit-breaker.js | ✅ 100%（预算熔断 + DC4 空转检测） |
| `internal/prompts` | prompts.js | ✅ 100%（缓存纪律：稳定前置/易变尾部） |
| `internal/verify` | verify.js | ✅ 100%（真实 git 测试；gate/worktree/提交/边界） |
| `internal/watchdog` | watchdog.js | ✅ 100%（双计时 + 进程组 kill + quiescence，真实子进程测试） |
| `internal/drivers` | drivers/*.js | ✅ 100%（fake pi/kimi 二进制 + PATH 前置真 exec） |
| 分发骨架：package.json、scripts/run.js、install.js、release-preflight.js、.goreleaser.yml、Makefile（coverage 门禁）、.gitignore | — | 完成 |

### 进行中（两个并行子代理负责，勿重复移植）
- **MachineTask** → `internal/machine/` + `internal/defectinjection/`（machine.js 1506 行 + defect-injection.js；含 P1 SIGTERM 注册与 P2 四项行为修复）
- **ModulesTask** → `internal/modules/` + `internal/skillsembed/`（intake/garden/install-skills/module.js + go:embed skills 数据层）

### 未开始
- cli 全部子命令接线（run/status/stop/intake/garden/install-skills/skills/lint）+ 输出契约（JSON→stdout、人话 status/stop、错误→stderr、退出码 0/1/3）
- format-check 的 Go 形态（建议内建 skillsembed 测试断言 frontmatter 规范）
- 文档更新（DESIGN v0.17 / README / HANDOFF / AGENTS）+ Node 遗留删除 + feat/go-port PR

### 已完成的其他项
- 4 个 SKILL.md YAML 非法修复（compatibility 加引号，2026-08-22 全部 10 个验证通过）——P1-2 关闭
- `.github/workflows/ci.yml` 已重写为 Go（setup-go 1.27 + make quality-gate）

## 3. 技术决策（已定，勿轻易推翻）

- **Go 1.27.0**（2026-08-19 发布；2026-08-22 用户拍板按"用最新"要求升级，go.mod 指令同步升；原 1.26.0 作废）。module `github.com/ckanner/flywheel`（对齐 remote `git@github.com:ckanner/flywheel.git`）。
- 依赖（最新）：`spf13/cobra v1.10.2`、`gopkg.in/yaml.v3 v3.0.1`、`santhosh-tekuri/jsonschema/v6 v6.0.3`、`stretchr/testify v1.12.1`（测试）。
- **契约校验改用 JSON Schema 库**（消灭 schema↔代码漂移 + 修复审计 P2 深度校验缺口）。v6 API 要点：`compiler.DefaultDraft(jsonschema.Draft2020)`（方法非字段）；错误用 `ve.BasicOutput()` 展平（`OutputUnit.InstanceLocation` + `OutputUnit.Error.String()`，无 `Message` 字段）。
- **Node 正则 → RE2 等价改写**（RE2 无 lookbehind/lookahead/惰性量词）：
  - statusCode：`(^|[^0-9A-Za-z.,])429(?![0-9A-Za-z.,])` → `(^|[^0-9A-Za-z.,])429([^0-9A-Za-z.,]|$)`（布尔检测等价）。
  - TIMEOUT 前置否定 `(?<!\bnot\s)(?<!n't\s)`：Go 里 `FindAllStringIndex` + `timeoutNegated()` 后处理。
  - frontmatter 惰性正则 `^---...[\s\S]*?---`：手写逐行扫描 `splitFrontmatter`。
- **覆盖率门禁**：`make coverage` = `go test $(go list ./... | grep -v /cmd/flywheel)` 断言 100.0%。`cmd/flywheel` 的 main() 是单行 `os.Exit(cli.Execute())` 桩，无逻辑，排除在门禁外（逻辑全在 `internal/cli`，会被测）。
- **可测性注入**：machine 的 sleep/now/env/log 全部可注入；drivers 通过临时目录放 fake `pi`/`kimi` 可执行文件 + PATH 前置实现真实 exec 测试（不引入 mock 注入缝）；watchdog 用真实子进程组 + 小超时。
- **npm 分发**：lark-cli 模式 — goreleaser 出 tar.gz + checksums.txt，npm 包 `bin→scripts/run.js`、`postinstall→scripts/install.js`（GitHub release 下载 + SHA-256 校验 + npmmirror 兜底），run.js 带运行时自动下载兜底（npx/monorepo 场景）。包名暂定 `@ckanner/flywheel`。
- **worktree**：保留 shell 出 `git` CLI 的语义（不引 go-git）。

## 4. 三份审计结论（已完成的只读审计，P1/P2 清单，移植时必须落实）

### P1（必须修）
1. **kill switch 失效**：Node `runTask` 定义了 `onSigterm` 却从未 `process.once('SIGTERM')` 注册（只有 `runSingleStep` 注册了，machine.js:1278-1281/1371 vs :1418）。`flywheel stop` 杀编排器后子进程组孤儿化继续写 worktree。→ Go 移植：machine 的 RunTask 必须注册 SIGTERM → kill 子进程组 → quiescence → exit 143。
2. **4/10 SKILL.md YAML frontmatter 非法**：`compatibility:` 裸标量含 `: `（"…the loops use: pi >=0.84…"），yaml 库直接拒载；`skill-format-check.js` 是纯正则所以 CI 假绿。涉事：flywheel-run/plan/review/exec 的 SKILL.md:7。→ 加引号 + format-check 改真 YAML 解析。

### P2（契约校验层 + 失败路径）
- `done-report-format:{}` 静默架空完成报告校验（Node contracts.js:103-105 + done-report.js:22-26）→ JSON Schema 已修（schema 要求恰好 4 键）。
- `boundaries.must-not-touch` 条目类型未校验 → 非字符串运行期崩（verify.js:313-314）→ schema 已修（string 数组）。
- `boundaries:{}` 静默通过 → **已在 schema 加 `minProperties:1`**（task-packet.schema.json，注意 example 需有字段）。
- spec `title` schema 要求而 validateSpec 不查 → JSON Schema 已修。
- **fallback 原地执行模式下 Planner 修订后 base-ref 不重锚**（machine.js:951-955 只在 worktree 分支执行）→ Go 移植必须对 in-place 模式也重锚（或提交契约后重取 base）。
- **空转检测读 executor 自报 changed-files**（machine.js:893-896）而非权威 git diff → Go 移植用 L1 boundary 的权威 changedFiles。
- **单元先标 completed 再 commit**（machine.js:986-989 vs 998-1028）→ Go 移植先 commit 成功再标 completed 并保存。
- flywheel-new 违反"skills are buttons"（内含生成逻辑、无 CLI 子命令）+ 复刻字段表 + 指示 agent 直写初始 state.json（与"agents/skills 永不写 state.json"矛盾）→ 文档口径统一（AGENTS.md 加 carve-out）；字段表改引 schema 单一信源。
- skill-format-check 不查 `whenToUse`/`arguments` 却声称 CI 强制 → 重写时补上。

### P3（挑选，尽量在移植中顺带修）
stdout 日志与 JSON 摘要交错（→ Go 版日志走 stderr 或独立 log）；install-skills actionText 缺 'stale'；triage-inbox.md/garden-proposals.md 不在 ledger 分类；verdict 角色 exit 检查不查 signal/stderr；crash 后 resume 跳过缺陷注入；review diff 不可用时回退 executor 自报；coverage 闸门纯约定不强制。

## 5. 已读透的源码（新 session 不必重读；引用行号有效）

- `DESIGN.md` 全文（v0.15/v0.16 变更记录、§1-§13、附录 A-D）
- `orchestrator/core/machine.js` 全文（1506 行）
- `orchestrator/core/`：state.js / contracts.js / verify.js / watchdog.js / errors.js / circuit-breaker.js / metrics.js / done-report.js / exit-contract.js / ledger.js / fallback.js / config.js / defect-injection.js / child-env.js / prompts.js
- `orchestrator/drivers/`：index.js / pi.js / kimi.js / mock.js
- `orchestrator/modules/`：intake.js / garden.js / install-skills.js / module.js
- `bin/flywheel.js` 全文、`templates/`（task-packet.schema.json 全文、spec.schema.json、flywheel.config.example.json 主体）、`tests/helpers.js`、`tests/e2e-mock.test.js`（头部）
- lark-cli 仓库：README/package.json/.goreleaser.yml/build.sh/main.go/go.mod/AGENTS.md/scripts/install.js+run.js/cmd/skill/skill.go/content_embed.go/skill-template/SKILL.md 模板/一个 SKILL.md 实例
- 官方 Skills spec（agentskills.io + openagentskills.dev）+ Warp skills 文档

**没读全的（需要时再读）**：`tests/` 其余 20+ 个测试文件细节（Go 测试将按行为重写，不必逐行移植）；`templates/state.schema.json` 全文；`templates/task-packet.example.md`/`spec.example.md`；`templates/flywheel.config.example.json` 尾部（intake 段）；其余 9 个 SKILL.md 全文（flywheel-run 已读）。

## 6. 移植顺序（每阶段完成后自审 + 跑该包测试与覆盖率）

1. **contract 测试**（下一步；含 schema 生效验证、P2 修复断言、state.schema 对 initState 形状的自检测试）
2. **state / donereport / exitcontract / metrics / circuitbreaker**（纯逻辑，Node 已读，直接移植 + 100% 测试）
3. **prompts**（提示词构建器 + 评审裁决解析；注意缓存纪律：稳定内容前置、易变内容尾部）
4. **verify**（runGateCommands / getBaseRef / getChangedFiles / getDiffText / checkBoundaries / 单元提交 / worktree 管理；用真实 git + t.TempDir() 测试）
5. **watchdog**（双计时 + 进程组 kill；真实子进程测试，小超时；注意 quiescence 语义）
6. **drivers**（Driver 接口 + mock/pi/kimi；fake 可执行文件 + PATH 前置）
7. **machine**（状态机最大头；注入 sleep/now/env/log；mock driver + 真实 git temp repo 做端到端；修 P1 SIGTERM、base-ref、completed 顺序、spin 权威 diff、resume 缺陷注入）
8. **modules + skillsembed**（intake/garden/install-skills + `flywheel skills list/read` go:embed 版）
9. **cli 接线**（全部子命令 + 输出契约：run/intake/garden JSON→stdout、status/stop 人话、错误→stderr、退出码 0/1/3）
10. **覆盖率 100% 门禁 + 修复循环**（`make coverage`）
11. **skills 规范修复**（4 个 YAML 加引号；format-check 改 YAML 解析 + 强制 whenToUse/arguments；与 Warp/官方/lark-cli 规范对照结论写进文档）
12. **文档**：DESIGN.md 升 v0.17（记录 Go 迁移 + 各 P2 修复 + 变更记录）；README 重写（安装/测试/目录结构）；HANDOFF.md 重写；AGENTS.md 重写（Go 构建/测试/硬契约）；删掉 `HANDOFF-migration.md`
13. **CI + 发布**：`.github/workflows/ci.yml` 改 Go（setup-go + make quality-gate）；goreleaser 发布 workflow（可选）
14. **Node 遗留删除**：bin/、orchestrator/、scripts/skill-format-check.js、package-lock.json、tests/*.js（干净切换，无 shim）
15. **PR**：分支（如 `feat/go-port`）→ 提交（Conventional Commits）→ push → `gh pr create`；**不直接动 main**

## 7. 环境事实

- 本机：macOS arm64，Go 1.27.0 已装（2026-08-22 mise 安装并设为默认，go.mod 指令已同步 1.27.0）。⚠️ 用户 shell PATH 硬编码了 `~/.local/share/mise/installs/go/1.26.0/bin`——新会话先 `go version` 核对，不对则 `eval "$(mise env)"`。git remote 已配（origin → ckanner/flywheel，ssh），`gh` 已登录（ckanner），当前在 main 分支、工作区干净（除本次新增未提交文件）。
- 真实运行环境事实（Node 版 HANDOFF.md 记录，迁移后需按 Go 版复核）：pi 0.84.0、`~/.pi/agent/models.json` maxTokens 覆盖（131072）、KIMI_API_KEY/OPENCODE_API_KEY 在环境变量。
- 用 `make coverage` 验证覆盖率；`make quality-gate` = vet + fmt-check + coverage。

## 8. 踩过的坑（勿重蹈）

- jsonschema/v6 没有 `Compiler.Draft` 字段 / `ValidationError.Message` —— 用 `DefaultDraft()` 方法与 `BasicOutput()`。
- RE2 不支持 lookbehind/lookahead/惰性量词，正则移植必须改写并**用测试锁定语义**（errs 包已有先例）。
- 覆盖率只差一两行时用 `go test -coverprofile` + `awk -F: '$3+0==0'` 定位未覆盖行，再补测试，不要放宽门禁。
- `math/rand` → 用 `math/rand/v2`（项目规则）。
- 测试断言"我以为的语义"可能 ≠ Node 真实行为（如 "context deadline exceeded" 归 CONTEXT_WINDOW_EXCEEDED 而非 TIMEOUT）—— 以 Node 源码行为为准，忠实移植，注释说明。

## 9. 开工前核查记录（2026-08-22，本 session 实测）

- **依赖版本全为最新**：cobra v1.10.2、testify v1.12.1、jsonschema/v6 v6.0.3、yaml.v3 v3.0.1（v3 已冻结，维护 fork = go.yaml.in/yaml/v3，go.mod 已正确引用为 indirect）。go.mod 指令需从 `go 1.25.0` 升到 `go 1.27.0`（与新 toolchain 对齐）。
- **外部参考出处核对**：
  - Warp skills 文档：`https://docs.warp.dev/agents/capabilities/skills/`（有完整 URL，步骤 11 精读）。
  - agentskills.io / openagentskills.dev：仅域名，无子页 URL，步骤 11 时现场查。
  - **lark-cli 源码出处断链**：§5 的"已读透"摘要保留，但无仓库 URL；本机仅有 npm 全局包（`~/.local/share/mise/installs/node/25.2.1/bin/lark-cli`），GitHub 上 ckanner 名下无 lark-cli 仓库。步骤 11 需复核时：向用户索取仓库地址，或从 npm 包内提取 install.js/run.js/SKILL.md 模板实例。
- **下一步动作**：① `go.mod` 升 `go 1.27.0`；② `internal/contract` 覆盖率补到 100%（缺口：`Acceptance.Level`、`Spec.Slug` 0%，`compileSchemas`/`ParseFrontmatter`/`ParsePacket`/`ParseSpec`/`LoadPackets`/`StateSchema`/`ValidateStateValue` 错误分支）；③ 跑 `make quality-gate` 确认该包 100% 后进入步骤 2。
