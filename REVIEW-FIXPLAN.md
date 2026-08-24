# Flywheel 全量评审结论与修复规划（2026-08-24）

> 状态：待人工对齐。本文档是 2026-08-24 全量评审的产出：外部来源核实、SKILL.md 规范符合性审查、
> 十铁律×代码逐条裁决、全部问题清单（统一编号）与分阶段修复方案。
> 评审方法：DESIGN.md v0.17 全文 + 全部 internal/ 源码精读 + 双子代理独立审计 +
> `make quality-gate` 实跑（20 包 100% 覆盖率全绿）+ 外部链接逐条核实（web_search + 原文读取）。
> 结论先行：**十铁律七条满分三条带注记成立，零条被违反；AGENTS.md 硬契约全部实证成立；
> 无 P1 缺陷；系统架构足以支撑目标，剩余风险集中在三处未证伪的外部假设。**

---

## 1. 外部来源核实结论（附录 A 相关）

### 1.1 已核实为真的来源

| 来源 | 判定 | 要点 |
|---|---|---|
| Osmani《Loop Engineering》 | ✅ | 五构件 = Automations/Worktrees/Skills/Plugins/Sub-agents + 第六项外部 State 层；与 Flywheel 是类比映射 |
| Anthropic《Effective Harnesses》 | ✅ | one-shotting / 过早完成 / 浅层验证三大失败模式逐字属实 |
| Anthropic《Multi-agent research》 | ✅ | 委托契约要素与 4×/15× token 数据属实 |
| Cursor《Scaling agents》 | ✅ | planner/worker/judge、扁平协调证伪、按角色选模型均有原文 |
| Ralph（ghuntley） | ✅ | bash while + 新上下文成立；"文件系统状态"是归纳用语 |
| **pi**（earendil-works/pi） | ✅ | `-p/--mode json/--no-session/--thinking max` 逐项核对 CLI Reference；JSONL 事件流 + usage 分项确认。仓库已从 badlogic/pi-mono 迁移 |
| **kimi code** | ✅ | stream-json / resume / goal 退出码 0/3/6 / #1911（open）均属实 |

### 1.2 本次新增应补入附录 A 的来源

1. **Warp Agent Skills**：<https://docs.warp.dev/agents/capabilities/skills/> ——第三个实现 Agent Skills
   标准的 harness（此前漏调研）。关键事实：
   - 必填 frontmatter 只有 `name` + `description`；
   - 参数不用 `arguments:` 字段，用正文 `$ARGUMENTS`/`$N` 占位符；
   - 扫描目录推荐 `.agents/skills/`（项目级）与 `~/.agents/skills/`（全局）——**直接佐证 Flywheel 共享安装点的选型**。
2. **agentskills.io 官方规范**：<https://agentskills.io/specification> ——字段权威定论：
   `name`(必填,≤64,须与目录同名) / `description`(必填,≤1024) /
   `license` / `compatibility`(≤500) / `metadata`(string→string map) / `allowed-tools`（可选）。
   提供 `skills-ref validate` 校验器。
3. **kimi code Skills 官方文档**：<https://moonshotai.github.io/kimi-code/en/customization/skills>
4. **pi Skills 官方文档**：<https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md>

### 1.3 由官方文档定案的事实（修正此前的判断）

- **`whenToUse`/`arguments` 归属**：两者不在 agentskills.io 规范中，但 **kimi code 官方明确定义**
  （whenToUse 还接受 when-to-use/when_to_use 变体；模型依据 `description` + `whenToUse`
  决定自动调用）。AGENTS.md 称其为"kimi code 扩展、自动调用依赖"——**对 kimi 而言正确**，
  但需补一句边界："非 agentskills.io 标准字段；其他 harness（Warp/Claude Code）不消费"。
- **`${KIMI_SKILL_DIR}`**：kimi 官方变量（当前 skill 所在目录），SKILL.md 中的用法正确。
- **共享安装点**：kimi（User 级扫 `~/.agents/skills/`，官方注"shared across tools"）、
  pi（全局扫 `~/.agents/skills/`）、Warp（推荐目录即它）三方一致。install-skills 选型正确。
- **调用语法**：`/skill:<name>` 是 **kimi code 的调用形态**（官方示例 `/skill:review-pr #1234`），
  pi 也注册 `/skill:name` 命令；但 Warp 是 `/<name>`，Claude Code 经 Skill 工具加载。
  README"调用形式为 /skill:<name>（Agent Skills 标准）"应改为"kimi code / pi 的调用形态"。
- **kimi 目录型 SKILL.md 必填字段只有 name + description**（缺任一解析失败）；
  type 默认 prompt；arguments 支持**字符串数组或空白分隔字符串**两种形态。

---

## 2. SKILL.md 全量规范符合性审查（10 个文件 × 3 套规范）

### 2.1 字段矩阵结论

| 字段 | agentskills.io | kimi code | pi | Warp | Flywheel 现状 | 判定 |
|---|---|---|---|---|---|---|
| name | 必填,≤64,=目录名 | 目录型必填 | 必填(放宽目录名匹配) | 必填 | 10 个全部合规（kebab-case、=目录名） | ✅ |
| description | 必填,≤1024 | 目录型必填 | 必填,≤1024 | 必填 | 全部非空、含触发关键词（英文 Use when…） | ✅ |
| metadata.version | 合法用法(map 自由键) | 未定义(忽略) | metadata 透传 | 未定义 | 全部 "1.0.0"，内部门禁强制 | ✅ |
| compatibility | 可选,≤500 | 未定义 | 可选 | 未定义 | 全部 ≤500 字符（v0.17 已加引号修 YAML） | ✅ |
| whenToUse | **不在规范** | **官方字段**,自动调用依据 | 未定义(忽略) | 未定义 | 全部非空中文 | ✅(kimi)/无害(其他) |
| arguments | **不在规范** | 官方字段,但**仅支持字符串数组或空白串** | 未定义 | 不用(正文占位符) | **对象形态 `{name,description,required}`——没有任何 harness 文档支持** | ⚠️ T3 |
| `${KIMI_SKILL_DIR}` | — | 官方变量 | — | — | 9 个按钮 skill 使用正确；锚文件回退链完整 | ✅ |

### 2.2 逐文件内容审查发现

全部 10 个文件的"只是按钮"纪律执行到位：无 loop 逻辑、参数校验 + CLI 调用 + 结果翻译三段式一致；
flywheel-new 的问答纪律（意图问人/事实自查）、冻结点声明、git add+commit 硬规则齐全；
intake/garden 的"候选不是任务""只产提案"人机分界清晰。发现的问题：

- **T3｜`arguments` 对象形态无规范支撑**（10 个文件全中）。
  kimi 只认字符串数组/空白串（`arguments: target mode`）；对象 `{name,…,required}`
  大概率被静默忽略——而正文也确实没用 `$slug`/`$0` 占位符，等于声明了参数却从未接线
  （kimi 行为兜底：无占位符时实参以 `\n\nARGUMENTS: <text>` 追加正文，功能不断但不优雅）。
  **修复**：arguments 改字符串数组；exec/review/verify/intake 四个带可选参数的 skill
  在正文命令示例中接入 `$1`/`$ARGUMENTS`。
- **T4｜调用语法归属**（README/DESIGN §13）：见 §1.3 最后一条。
- **T6｜flywheel-new 缺"验收命令试跑"硬规则**：生成器没被要求实际执行生成的 L1 命令，
  契约质量完全依赖生成会话的水平（系统上限 = 契约质量的上游瓶颈）。
  **修复**：SKILL.md 流程第 3 步加硬规则——每条 acceptance 命令提交前必须在目标仓库
  实际执行一次，允许失败但必须把命令+退出码+输出摘要写进 spec 附表作证据；
  无法执行的命令（环境缺失）必须显式标记 `unverified` 并提醒人审关注。
- **可选 T7**：CI 增加 `skills-ref validate ./skills/*`（agentskills.io 官方校验器），
  与现有 skillsembed 门禁互补（标准符合性 vs 内部约定）。

---

## 3. 十铁律 × 代码逐条裁决（摘要）

| # | 铁律 | 裁决 | 关键证据 |
|---|---|---|---|
| 1 | 编排器不是 agent | ✅ | 全部决策点机械可查（run.go:585/circuitbreaker.go:136/errs.go:79）；判断外置 Plan/Review/Judge/inbox |
| 2 | 状态落盘，交接走文件/git | ✅ | state.json(CAS)+retro+reports+metrics.jsonl；Plan 会话句柄入盘、冷启动协议闭环（steps.go:882-915） |
| 3 | 单元=全新上下文 | ✅ | Execute/Review/Judge 每次 `pi --no-session` 新会话；Plan 长驻是 §4.1 明文例外且有机械冷启动协议 |
| 4 | oracle 先行，LLM 在后 | ✅ | processPacket 阶段机 + `--only review` 双入口强制 l1PassTrue（run.go:1253-1256） |
| 5 | 做判分离 | ⚠️ | 评审输入无 transcript/done-report 正文；异厂商组合成立。降级路径（非 git）自报清单带标签泄漏；diff 文本是 executor 可写面（注入残余风险，业界共识为 best-effort，见 §6） |
| 6 | 契约显式字段化+版本 | ✅ | schema additionalProperties:false + LOAD 时 fail-fast + 跨字段规则；版本号三处齐备 |
| 7 | 会话级模型隔离 | ✅ | 单 spawn 单模型；降级换模型发生在会话之间；Plan 会话钉死 model+channel 才 resume（#15） |
| 8 | 分级+升级路径 | ✅ | routes 表 + hard 升级 K3 + 三级 escalation + spin 止损；最坏 bounded 到 inbox |
| 9 | 成本可观测+熔断可配 | ⚠️ | 记录恒开✓ 三档✓；但 C1（CacheWrite 漏计价）实违本条的准确性；kimi Plan usage null 是明文豁免 |
| 10 | 缓存一等指标 | ⚠️ | DeepSeek 侧稳定前置/易变尾部✓ 逐 provider 命中率✓；review prompt 中段时间戳违反字面纪律（影响≈0）；kimi 侧不可观（同豁免）；KIMI_NOW resume 刷新打折跨 resume 收益（A5③） |

**AGENTS.md 硬契约实证**（全部成立）：state.json 写权收敛于 internal/state.Save
（machine/cli-stop/modules 三入口，modules 带 pid 守卫防 mid-run 抬 revision）；
九错误码与 §6 逐字一致；覆盖率门禁实跑全绿；ledger 单一信源成立（defectinjection
应用层有一个洞 → C3）；skills 真 YAML 门禁存在（skillsembed_test.go:16-93）；
输出契约 root.go 实现一致；SIGTERM kill switch 双入口注册。

---

## 4. 问题总清单（统一编号）

> 编号规则：A=设计级（改设计/流程），C=代码缺陷级，T=文档/skill 级。
> 每条给出：证据 → 影响 → 修复方案 → 验收标准。

### 设计级

**A1｜planStep 产出零校验；planner 可弱化验收且无留痕**（最高优先）
- 证据：steps.go:955（成功判定仅 exit==0）；escalatePacket 仅 `PlanRevisions < 1` 限次；
  DC2 只约束 verification-gap 必须"加强"。
- 影响：executor 连败 → return-to-planner → planner 改弱 acceptance → L1 绿 → 单元通过，
  全程无机械痕迹。注：这不是铁律 1 违反（计划质量归 Plan 是法律授权），缺的是**机械留痕**
  把该环节拉回可监督范围。
- 方案：plan 成功路径（常规 run.go:1011 reloadPackets 与 escalatePacket 两处）在 reload 前
  对旧 PacketList 快照每 packet 的 acceptance 指纹（sha256(L1 commands+expect) 序列化），
  reload 后 diff；acceptance 变化 → retro 行 + `state.budget.warnings` 告警 + Log
  （不阻止——范围裁决权归 Plan，铁律 1；留痕供人审 status 时看见）。
  纯函数 `diffAcceptanceSignatures(old, fresh)` 放 machine 包。
- 验收：单测两分支（修订改弱/未动）各断言 retro 行与 warning；quality-gate 绿。
  DESIGN §4.1 增补"已知风险"段落记载该风险与本缓解。

**A2｜DESIGN §4.1 与实现的规划职责不符**
- 证据：packets 全由 flywheel-new 一次性生成；planStep 遇无 packet milestone 直接报错（steps.go:897）。
- 方案：§4.1 改写为两段式事实："初始规划 = flywheel-new 生成 + 人审冻结（§12）；
  运行中 Plan Loop = 修订器/加强器（escalation 与 verification-gap 触发）"。
- 验收：DESIGN 通读无"Plan Loop 签发 packet 序列"的运行期表述残留。

**A3｜单 judge 与所引证据相抵触**
- 证据：里程碑 Judge pass/fail 直接决定关闭（run.go:1055-1086）；2025 年研究：单 judge
  受 position bias/self-preference/verbosity bias 系统性影响，单 judge 与 ground truth
  一致率约 87.6%，bias-aware panel 达 98.1%（Verdict-Bench/Jury,
  ijisae.org/index.php/IJISAE/article/view/8407；position bias 系统研究
  aclanthology.org/2025.ijcnlp-long.18/；self-preference DBG
  aclanthology.org/2025.emnlp-main.86/）。
- 方案（分层落地）：
  - **立即（低成本）**：强化裁决出口契约——ParseReviewVerdict 增加 score 非空与
    reasons ≥1 条校验，缺失按 unparseable 走重试；BuildJudgePrompt/BuildReviewPrompt
    要求 verdict.reasons 每条必须引用可复核证据（命令名+退出码 / 文件:行号）。
  - **首跑后评估**：里程碑 Judge 双模型面板（K3 主判 + V4 Pro 异厂商复核，
    分歧→inbox），config 开关 `judge.panel` 默认关；引 Jury 论文的 diversity+校准聚合。
- 验收：prompts 单测覆盖新校验分支；DESIGN §4.3 已知风险段更新引用。

**A4｜凭证卫生表述强于实际保证**
- 证据：childenv.go:9-12 白名单含 HOME/XDG_*——gate 命令跑 executor 写的代码，
  可达 ~/.pi/agent/auth.json、~/.kimi-code。
- 定性：与 v0.10 信任模型一致，非 bug；但 §9 表述易被读成防恶意。
- 方案：§9 加一段边界声明："凭证卫生 = 防事故性继承（白名单不含任何 API key）；
  不防恶意进程读 HOME 下 auth store——那是容器化重评（§11.7）的触发条件之一"。
- 验收：DESIGN §9 更新；无代码改动。

**A5｜小项**
- ① LastProgressAt 只写不读（§6"用于发现断链"半缺失）。
  方案：status 子命令增加停滞判定（last-progress 距今 > watchdog.wall-timeout → 显式
  "疑似停滞/断链"提示）；§6 措辞同步（cron 短进程模式下断链发现主通道是 pid 陈旧 + 进程不存在）。
- ② kimi Plan token 不可观测：维持 v0.15 豁免，DESIGN 已记录，不动。
- ③ KIMI_NOW resume 刷新（kimi #446/#498）打折长驻会话跨 resume 缓存：
  §8-4 加一句事实注记即可。
- ④ L2/L3 闸门失败直奔 inbox 无回 Plan 路径：实现替设计补的空白，回写 §4.4
  （保守处理是有意选择：L2/L3 失败多为环境/契约级问题，重试烧钱无益）。

### 代码级

| ID | 级别 | 问题 | 证据 | 修复方案 | 验收 |
|---|---|---|---|---|---|
| C1 | P2 | 兜底成本估算漏 CacheWrite（铁律 9 准确性） | machine/context.go:141 EstimateCostUSD 少传 CacheWrite（metrics 层本就支持按 input 价计费） | 补传 `CacheWrite: usage.CacheWrite`；同步更新对应单测期望值 | 单测：无 cost 流 + CacheWrite>0 → 估算含缓存写入计价；coverage 100% 保持 |
| C2 | P2 | 评审者看不到 untracked 新文件内容（违反 §4.3"真实 git diff"承诺；greenfield 首批任务正中靶心） | verify.go:295 GetDiffText 仅 `git diff base -- .` | 对 changed-but-untracked 且未被 exclude 的文件逐个追加 new-file 内容段（仿 `diff --git`头 + 全文），受 DiffTextCap 总量约束；二进制文件以 `[binary]` 占位 | verify 单测：worktree 新建文件 → GetDiffText 含内容；exclude 生效；超 cap 截断标记 |
| C3 | P2 | 缺陷注入账本守卫精确相等可绕过（`reports/x` ≠ `reports/`） | defectinjection.go:177-186 | 改为复用 ledger.IsLedgerTaskPath 语义（传入 taskDir 相对路径判断），消灭两套匹配逻辑 | defectinjection 单测：patch 触 reports/x.json → uninjectable；coverage 保持 |
| C4 | P3 | execute 失败路径空转签名退化为自报/空集 | run.go:608 | execute 失败时若 worktree 存在，编排器亲自 GetChangedFiles(uCwd, baseRef) 作权威集；不可得再落自报兜底 | machine 单测：连续两次同错误码失败 + 权威 diff 相似 → spin |
| C5 | P3 | kimi resume_hint 格式漂移静默退化为每轮冷启动 | drivers/kimi.go:64 | planStep 冷启动分支已有日志；补一处：sessionID=="" 且 stored 存在时 Log"未能从上次流中捕获 session id" | drivers 单测断言日志回调收到提示 |
| C6 | P3 | pi 错误事件只保留最后一条 | drivers/pi.go:92 | ErrorMessage 改为追加（`; ` join，去重），或保留首条+计数 | drivers 单测：双错误事件 → 两条都在 |
| C7 | P3 | jitter 显式配 0 不可达 | errs.go:174 | **决定：不修**（Node destructuring 语义忠实移植；确定性测试经 Random 注入已可达）。补注释说明即可 | 注释合并进既有测试 |
| C8 | P3 | done-report 多余键放行 | donereport.Parse | 校验键集合恰好等于 done-report-format 声明的 4 键（多余键列入 Errors） | donereport 单测：多一键 → !OK |
| C9 | P3 | Save CAS 读-写 TOCTOU 微窗口 | state.go:221 | **决定：接受不修**。pid 单实例下唯一并发写者是人手编辑；窗口毫秒级、后果轻。注释已说明 CAS 目的 | — |
| C10 | P3 | ForeignDirtyFiles 不处理 porcelain rename（`R old -> new`）与引号路径 | verify.go:173-188 | 解析 porcelain XY 格式：rename 条目拆两路径；引号路径去引号+反转义（或保守：整行跳过 rename 态只报目标路径） | verify 单测构造 rename/status 输出 |
| C11 | P4 | 手工把 milestone.gate 写成非对象时 judge 结论落入孤立 map | steps.go:808-812（coverage_steps_test.go:532 自首） | judgeStep 内 gmap 类型断言失败时记 warning 并跳过 judge 结论写入（或拒绝字符串 gate） | machine 单测一例 |

### 文档 / skill 级

| ID | 级别 | 问题 | 修复方案 |
|---|---|---|---|
| T1 | P3 | drills/*.md 可重复入口指向已删除的 Node 测试（node --test tests/drill-*.test.js、core/*.js 路径） | 入口改为 Go 测试真名（`go test ./internal/drivers -run …`、`go test ./internal/defectinjection …`）；core/*.js 引用改 internal 包路径 |
| T2 | P3 | frontmatter 归属表述不精确（AGENTS.md、skillsembed_test.go:16-18,60 注释） | 按 §1.3 定案改写：非 agentskills.io 标准字段 + kimi code 官方定义（附文档 URL）+ 自动调用依据 + 其他 harness 不消费 |
| T3 | P3 | SKILL.md arguments 对象形态无规范支撑且正文无占位符 | 见 §2.2 |
| T4 | P3 | `/skill:<name>` 被表述为"Agent Skills 标准" | README/DESIGN §13 改注："kimi code 与 pi 的调用形态；Warp 为 /<name>；Claude Code 经 Skill 工具" |
| T5 | P3 | 附录 A 缺 Warp / agentskills.io / kimi·pi skills 官方文档四个来源 | 按 §1.2 增补（Warp 归 Harness 调研组，其余归方法与规范组） |
| T6 | P2 | flywheel-new 缺"验收命令提交前试跑"硬规则（契约质量上游瓶颈） | 见 §2.2 |
| T7 | 可选 | CI 增加 `skills-ref validate`（agentskills.io 官方校验器） | make quality-gate 追加一步或 CI workflow 追加 step |

### 明确不修清单（含理由）

- ARG_MAX/prompt 作为 argv 上限风险：DiffTextCap 100KB + CacheScopeCap 50KB + packet 体，
  总 prompt ~200KB 量级 << macOS ~1MB argv 限制。不加管道化传输。
- C7 jitter、C9 TOCTOU：见上表。
- kimi Plan usage 不可观：v0.15 用户裁决豁免。

---

## 5. 分阶段修复计划

> 每阶段以 `make quality-gate` 全绿收口；不 commit（等用户批准）。

### Phase 0｜文档与规范订正（无代码风险，~半天）
T1、T2、T4、T5 + A2、A4、A5③④ 的 DESIGN 改写。
验收：文档通读一致性；无代码行为变化（go build/test 不动仍绿）。

### Phase 1｜P2 代码三连修（~半天-1 天）
C1 → C2 → C3，各自带回归测试。
验收：新增单测全绿；quality-gate 全绿；drill 手工冒烟（临时 repo 建 untracked 文件跑
`--only review` 看 prompt 含新文件内容）。

### Phase 2｜设计补强与小修（1-2 天）
A1 机械留痕（含 DESIGN §4.1 已知风险段）→ A3 立即版（verdict 出口契约强化）→
A5① status 停滞提示 → T3/T6 skill 修订 → C4/C5/C6/C8/C10/C11 小修批量。
验收：各项对应单测；skillsembed 门禁对新 frontmatter 仍绿；
`flywheel skills list/read` 冒烟。

### Phase 3｜首跑准备（与 Phase 2 可并行推进）
1. OpenCode Go ToS 决策记录（无人值守条款灰区签字画押，封号预案=Zen 按量价重算经济模型）。
2. 真实 403 报文实测预案：拿到超额账号当天按 drills/403-fallback.md 方法打一轮，
   模式表集中在 errs.go patterns 一处可即调。
3. mimir 任务生成时执行 T6 新规（每条 L1 命令试跑留证）。
4. 缺陷注入 patch/test-cmd 配置模板随首批 packet 一起产出。
5. 全部就绪后 `/skill:flywheel-run docs/mimir-s4-0` 首跑，按 HANDOFF §下一步观察。

---

## 6. 业界最佳实践对照（本次修复方案的依据）

| 议题 | 业界参照 | Flywheel 现状与采纳 |
|---|---|---|
| LLM judge 可靠性 | Verdict-Bench/Jury（单 judge 一致率 87.6% → bias-aware panel 98.1%）；position bias 系统研究（IJCNLP 2025）；self-preference DBG（EMNLP 2025） | 已做对：异厂商 judge + rubric 显式打分 + L1 先行；采纳：A3 出口契约强化 + 双模型面板（v0.18 完整版） |
| 评审者面对不可信 diff | OWASP LLM01 / Prompt Injection Prevention Cheat Sheet（防御纵深、指令数据分离、最小权限）；Anthropic security-review 官方自述 "not hardened against prompt injection, trusted PRs only"；GitInject, arXiv:2606.09935（AI CI 流水线注入实证） | 铁律 4（oracle 先行）正是 OWASP 分层+最小权限的正确实例；残余风险按业界口径定性为 best-effort 写进 DESIGN §4.3；不引入重型注入分类器（成本不成比例） |
| Planner 监督 | Cursor planner/judge 分相；Anthropic multi-agent 委托契约 | A1 留痕 = 把"委托契约变更"纳入可审计面，方向与 multi-agent observability 结论一致 |
| Skill 生态兼容性 | agentskills.io 规范 + kimi code / pi / Warp 三家官方文档交叉验证 | 共享安装点选型获三家共同佐证；T3/T4 修正后完全兼容 |
| 变异测试演进路径 | DESIGN §4.3 已预留 Stryker 方向 | 维持：缺陷注入抽检是变异测试的确定性子集，先实战再演进 |

---

## 7. 裁决结果（2026-08-24，用户已裁决）

| # | 待裁决项 | 结果 |
|---|---|---|
| 1 | Phase 顺序 | ✅ 按计划（文档 → P2 → 补强 → 首跑准备） |
| 2 | A3 双 judge 面板 | ✅ **现在就做完整版**（config `judge-panel`：K3 主判 + V4 Pro 复核；一致按共识、分歧 → inbox），出口契约强化一并落地 |
| 3 | T7 skills-ref validate | 以等价 Go 实现落入门禁（skillsembed 扩展 agentskills.io 标准约束），不引入外部工具依赖 |
| 4 | C7/C9"明确不修" | ❌ 推翻——**两者都修**：C7 jitter `*float64` 指针化（nil=默认 0.1，显式 0=无 jitter）；C9 CAS 加 flock 文件锁闭 TOCTOU |
| 5 | 文档入库 | ✅ 本文档随修复分支提交 |

交付方式：全部修完后出 PR 供人审，**不首跑**。
