# 从需求准备到 PR 合并

[English](en/workflow.md) · 简体中文

本文描述 **Flywheel 当前源码**的推荐工作流。默认 `hard` 路由使用
Codex Pro 插件 machine API；源码存在不代表 npm `latest` 已更新，安装状态见
[发行指南](releasing.md)。下列命令是操作示例，不会因阅读本文自动执行。

建议先建立任务集成分支及独立 worktree，再准备契约、开 Draft PR、运行引擎。
Flywheel 将通过验收的 packet 合入任务分支；整个需求通过终验和远端检查后，
再由人或已获授权的宿主 Agent 合并 PR 到 `main`。

## 0. 常规入口：描述目标并启动

包含自适应工作流的同版本 CLI／DSH 插件提供统一入口：

```text
/flywheel-task 修复已定位的问题，补齐回归测试并完成验证。mode=auto
```

宿主使用当前对话研究代码、复用或建立任务 worktree，准备 Spec、packet、验收与
工作流事实，保存契约后调用 `flywheel_launch`。Go 校验资格并启动，DSH 将这个
具体任务关联到当前会话的 Flywheel 视图；通常无需复制路径或反复确认已授权范围。
`flywheel control launch --help` 可确认新接口是否存在；旧发行中的基础命令不代表
已经具备此入口。

| 选择 | 引擎选择与闭环 |
|---|---|
| Auto（默认） | 根据已研究并记录的事实选择 Quick；资格不足或存在风险则 Full |
| Quick | 一个里程碑、一个 packet，执行、全部必需验证、独立最终 Review、Git 交付 |
| Full | Plan、逐 packet 执行与评审、集成 L2/L3、Judge |

Quick 不按文件数量判断风险。范围与实现模式必须明确、全部需求有验收映射、
独立最终评审可以完整判断，缺失 L2/L3 要有不适用理由。鉴权、支付、迁移、并发、
权限或未决设计等使用 Full；显式 Quick 也不能绕过资格。最终 Review 发现验收
缺口或新风险时，Go 保存升级并继续 Full，保留预算、工作区和历史。

正常首次成功的 Quick 只有 Execute 与最终 Review 两次阶段 Agent 调用，宿主
准备、模型内部轮次和重试另计。必需测试、覆盖率、适用 L2/L3 与缺陷注入仍保留，
而且都必须验证最终 Review 读取的同一候选。Plan／Judge 记录为“不适用”及原因，
不能显示假通过。

只想先看方案时用 `/flywheel-new <需求>`；已准备或要恢复任务时用
`/flywheel-run <真实任务目录>`。用户已有的工作区、实施与交付授权继续沿用，
明确提出“先给我看方案”时才停在审阅。下面逐节展开准备、启动与交付的检查点，
不要求自动入口逐项等待用户点击。

## 1. 选择准备阶段的模型

`flywheel-task` 与 `flywheel-new` 使用当前 DSH 对话的模型。`loops.plan` 只控制引擎启动后的规划
和修订，不会替准备阶段切换模型。建议按需求的不确定性选择：

| 准备任务 | DSH Provider / 模型 | 思考等级 |
|---|---|---|
| 跨模块设计、协议与生命周期、复杂边界和验收设计 | `chatgpt` / `gpt-6-astra` | `high` |
| 范围清楚、已有代码与模式可参考的普通功能 | `chatgpt` / `gpt-6.1-sol` | `high` |
| 很明确的小修复、文档或重复模式的补充 | `deepseek-account` / `deepseek-flash` | `high` |

这是工程起点，不是本项目完成的模型 benchmark。准备错误可能影响多个 packet，
所以复杂需求值得在冻结契约前多检查一次；不必为每个小单元重复进行完整架构规划。
先使用 `high`，遇到仍未解决的关键设计问题，再对那次分析选择模型支持的更高档位。
[OpenAI reasoning 指南](https://developers.openai.com/api/docs/guides/reasoning)
说明 effort 的速度、用量与推理投入取舍；它不能证明本项目的最优档位。

开始前在 DSH 当前账号的模型目录核对 provider、精确模型 ID 和可选 effort。
目录与实际请求支持也可能有差异，不能把配置示例当成账号可用性证明。
这里的 DeepSeek 选择复用已登录、已充值的 DSH 账号；`deepseek-official` API
是另行显式配置的可选渠道，默认预设没有通往它的后备链。用户自定义后备策略仍需显式配置。`high` 是尚未经本项目 A/B
验证的工程基线；不要自动把所有阶段提高到 `xhigh` 或 `max`。
`Codex Pro` 是插件名，不会自动选择 Pro 模型或最高思考档。
默认的运行期 Plan 仍为 Kimi CLI `kimi-code/k3.1-pro / high`，详见
[默认配置](../templates/flywheel.config.example.json)；Kimi 别名也须在本机可用。

## 2. 先建立任务集成 worktree

假设当前位于业务仓库根目录，远端为 `origin`、目标分支为 `main`。
将示例任务名与目录名替换为本次需求；不同任务使用不同名称。

```sh
git status --short
git fetch origin
git worktree add -b task/my-feature ../repo-my-feature origin/main
cd ../repo-my-feature
git branch --show-current
```

最后应显示 `task/my-feature`。这组命令创建新分支和新目录，不切换或重置原
checkout 的分支。若分支或目录已存在，先检查其用途，不用强制参数覆盖。
在 DSH 中打开新目录 `repo-my-feature`，之后的契约、预检、启动和 PR 操作都以
这个任务 worktree 为工作目录。运行中不要切换该目录的分支或混入别的任务。

引擎中的“主 checkout”指传入的 `--cwd`，不固定指 `main`。单元通过后执行
`git merge --ff-only` 合回这个 checkout 当前所在分支。因此从任务分支启动，
内部 packet 就合入 `task/my-feature`；从 `main` 启动则会推进本地 `main`。
实现依据见 [worktree 操作](../internal/verify/verify.go) 与
[单元交付](../internal/machine/units.go)。

worktree 隔离工作目录和 index，不是安全沙箱。当前引擎默认尝试创建 packet
worktree，但创建失败时会记录原因并回退 `in-place`；它没有“必须隔离”的开关。
出现 `worktree unavailable` 时检查日志和工作区状态；需要严格隔离的任务应停止，
人工安排并核实隔离恢复。已经保存的 `in-place` 状态不会因修复目录或重新启动
就自动变成独立 worktree；不要手改 `state.json`，也不能把默认开启理解成任何
情况下都有独立工作区。

## 3. 在任务 worktree 中准备并审阅契约

确认当前目录仍是 `repo-my-feature`，读取准备 skill：

```sh
flywheel skills read flywheel-new
```

在 DSH 对话输入框发送（也可从 `/` 菜单选择技能）：

```text
/flywheel-new 为本需求建立 docs/my-feature。先查真实代码，写清需求、packet、路由、修改边界和可执行验收；试跑 L1 并记录结果，完成后供我审阅。
```

**无需先填写 Flywheel 视图。** 助手按 skill 创建或复用任务 worktree，生成
`docs/my-feature/spec.md` 和 task packets，并报告项目根 `repo_dir` 与任务资料
目录 `task_dir` 的真实绝对路径。前者是代码与工作区，后者保存本任务的契约、
状态和报告；一个项目可以包含多个任务资料目录，不能把项目根直接当成任务目录。

DSH 的正式技能语法是 `/flywheel-new`，不是 `/skill:flywheel-new`；后者是
Kimi 的语法。依据见 [DSH 技能入口](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-skill/README.md)
和 [Kimi slash commands](https://moonshotai.github.io/kimi-code/en/reference/slash-commands.html)。

审阅至少覆盖：需求与 packet 的对应、L1/L2/L3 命令、失败出口、冻结验收路径，
以及模型和权限配置。100% 覆盖率如为任务要求，应有实际可执行的门禁命令。
不要填未经运行的验收结果；环境缺失应标记 unverified。不要手工创建或修改
`state.json`，它由引擎管理。完整要求见 [flywheel-new](../skills/flywheel-new/SKILL.md)。

自动准备启动使用 `flywheel-task`，其契约在 Spec 的 `workflow` 中记录 mode、
版本、代码依据、风险与资格判断，字段见[工作流配置](configuration.md#工作流策略与恢复)。
只有模型写了“low risk”不能取得 Quick；Go 校验结构与声明，执行和最终评审再
检查实际候选。没有 workflow 字段的旧契约保留 Full。

按已有 Git 授权保存审阅后的契约，仅选择本任务文件：

```sh
git diff -- docs/my-feature
git add docs/my-feature
git diff --cached
git commit -m "docs(my-feature): add approved task contracts"
```

若冻结验收文件位于任务目录之外，审阅后显式补选那些路径。提交前确认暂存区
没有无关文件或凭证，不使用全仓库的 `git add .` 代替范围检查。契约必须实质
进入提交，packet worktree 才能从 Git 基线取得它；空提交不能替代这一步。

## 4. 尽早建立任务级 Draft PR

契约确认并提交后，就可以推送任务分支、建立 Draft PR，不必等全部实现结束。
以下使用已登录且有仓库权限的 GitHub CLI；也可在 GitHub 页面完成同样操作：

```sh
git push -u origin task/my-feature
gh pr create --draft --base main --head task/my-feature
```

按提示填写需求、范围、验收计划与当前状态。一个可独立交付的需求或里程碑通常
对应一个 PR；packet 是引擎内部工作单元，不必逐个开 PR。很大的需求应先拆成
可独立验证、回滚的里程碑。[Draft PR 无法合并](https://docs.github.com/en/pull-requests/reference/pull-requests)，
适合保留进行中的差异、讨论和检查结果。CI 是否在 Draft 阶段运行取决于仓库配置。

## 5. 从同一个任务 worktree 启动

契约审阅并保存后，可继续在 DSH 对话逐条发送以下技能调用，将示例路径替换为
准备阶段报告的真实 `task_dir`。启动后再查询，需要停止时调用最后一项：

```text
/flywheel-run /absolute/path/to/repo-my-feature/docs/my-feature
/flywheel-status /absolute/path/to/repo-my-feature/docs/my-feature
/flywheel-stop /absolute/path/to/repo-my-feature/docs/my-feature
```

这些技能从实际任务目录解析项目根并调用 CLI，无需先打开或填写视图。
如果直接使用终端，当前目录必须仍是 `repo-my-feature`，任务目录也在这个 worktree 中：

```sh
flywheel config inspect --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --json
flywheel doctor --cwd "$PWD" --task-dir "$PWD/docs/my-feature" --probe --json
flywheel control start --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
flywheel control status --cwd "$PWD" --task-dir "$PWD/docs/my-feature"
```

新统一入口可将 start 换成 `control launch ... --mode auto`；它消费已经准备的
契约，默认请求 Auto。`start`／前台 `run` 可带 `--mode auto|quick|full`，省略时
保留契约或持久化策略。旧状态不会因新增 --mode 而静默跳过原来的 Full 阶段，
恢复也不会重新分类或从 Full 降为 Quick。

预检不发送模型推理，不能证明账号额度或真实模型调用已经通过。启动会实际调用
所选模型。DSH `flywheel_launch` 与 `flywheel_control` start 将明确的 `taskDir`
和 `cwd` 关联到发起会话的 Flywheel 视图；单纯 shell 调 CLI 不会创建桌面会话关联。
终端启动或另开已有任务时，仍可手动填“任务资料目录”和“项目目录”。后者仅在
会话当前目录已经是同一 `repo_dir` 时可留空。插件通过公开
`workingDirectory.get(session)` 读取目录；不扫描或猜测。仅在 shell 中 `cd`
不会改变 DSH 的会话目录，不要混用原 checkout 的账本和新 worktree。

Full 的每个 packet 经实现、L1 与独立 Review 通过后，引擎提交并快进合入任务分支，
全部 packet 完成后仍需集成 L2/L3 与 Judge。Quick 在实际 packet 候选中先执行
所有适用 L2/L3，再作覆盖全部需求的一次最终 Review，然后持久化同一候选。
两种流程都必须核对 `state.status` 与交付证据；单元合并不等于整个需求完成。
合并冲突或验证失败时保留产物并处理记录，不强制清理。停止与恢复见
[入门教程](getting-started.md#7-启动观察停止与恢复)。

若报告 `cleanup-unconfirmed` 或 `process cleanup is unconfirmed`，引擎会将本次
运行升级为 `escalated` 并停止自动执行，保留工作区和审计记录。先核对本次调用
拥有的 worker、guardian 和 app-server 是否已退出，再按恢复流程继续；不能仅
因外层 worker 已退出或已等待几秒，就假定内部进程不会再修改工作区。

### 5.1 Skills 入口与各 Loop 的工作目录

内置 skills 将任务 worktree 约定作为准备、启动、恢复和单环入口的检查步骤。
`flywheel-task`／`flywheel-new` 在生成契约前创建或复用任务 worktree；`flywheel-run` 以及单环
skills 从实际任务目录解析同一 worktree 根，再调用 CLI。默认不从 `main`、
`master` 或 detached HEAD 启动开发；已有明确工作区选择与授权不重复询问。
恢复复用原任务路径及引擎保存的 packet 状态，不为恢复另建一份任务或复制 state。

| 阶段／入口 | 实际工作目录 | 谁负责选择或创建 |
|---|---|---|
| `flywheel-task`／`flywheel-new` 准备契约 | 任务集成 worktree | 宿主 Agent 按 skill 创建或复用 |
| Full Plan／升级后修订 | 任务集成 worktree，修订任务契约 | 引擎使用启动时的根目录 |
| Execute、L1、Review | 当前 packet 候选工作区；正常模式为 packet worktree | 引擎创建、记录并复用 packet worktree |
| Quick 适用 L2/L3 | 同一 packet 候选工作区，先于最终 Review | 引擎不能使用尚未集成修改的旧 checkout |
| Full L2/L3、Judge | 任务集成 worktree | 引擎验证已集成的需求与里程碑 |
| Intake、Garden | 所选任务所属 worktree | 入口解析目录，Go 写建议结果 |
| Status、Stop | 对应原任务路径与根目录 | 直接查询或停止，不要求为此新建 worktree、切分支或提交 |

模型调用的权限由 `permissions.mode` 决定：`role-based`（已有文件缺省）下，
Plan／Execute 请求 `workspace-write`，Review／Judge 以及 DSH／Codex（含 Codex Pro）
的 Intake／Garden 请求 `read-only`；`full-access` 下全部阶段通过所选后端正式接口
请求完全权限。Kimi print 保持原生 auto，不提供可宣称的 OS 级只读或工作区沙箱。
权限不改变阶段职责：评审只评价候选，辅助模块只产建议；评审前后的候选身份检查
仍然执行。完整配置和迁移说明见[配置指南](configuration.md)。

各 Loop 不会各自新建一份 worktree。任务 worktree 贯穿整个需求，
packet worktree 贯穿该单元的执行、验收与评审；完成交付才由引擎处理本地合并
与清理。恢复入口仍是 `flywheel-run`，没有另一个独立的 resume skill。

Go 调度 packet，首版仍是串行执行，不依赖一个持续存活的父模型会话监督。
每次 Execute／Review 使用新会话；重试保留磁盘产物，不复用执行者上下文。
Plan 仅在配置允许、后端支持且身份匹配时恢复历史。团队通信、依赖图并行调度
不在本期能力范围。
`flywheel-verify` 只运行确定性验收：Full 保留指定 packet 的 L1／当前里程碑
L2/L3 语义；Quick 还会在实际候选上收齐适用 L2/L3。它不调用 Review／Judge，
不创建 worktree，也不交付任务。Quick 的 `--only review` 是最终 Review；若发现
验收缺口或资格失效，Go 当即保存单向 Full 升级，下一次完整 run 才进入 Full。
`--only plan` 则要求 Full，确需规划时显式 `--mode full` 由引擎升级。
`--only execute` 始终只是 Execute／L1 子环，不等于完整 Quick。

Skill 的启动检查由宿主 Agent 遵守，CLI／GUI 直接调用不会自动执行这些 skill
正文，也不会自动创建外层任务分支。直接使用控制接口时仍须显式选择正确的
`taskDir` 与正确的 `cwd`（或经核实的当前会话目录默认值）；引擎现有的
`in-place` 回退边界见第 2 节。只读查询和停止
不受开发分支检查阻碍，便于处理异常任务。

## 6. 完整终验后再合并 PR

确认 `state.status` 为 `completed`，需求覆盖、验收报告、独立评审及本地提交
都齐全，再核对工作区并推送任务分支。`control` 返回 `ok:true` 只代表控制操作成功。

```sh
git status --short
git log -5 --oneline
git push origin task/my-feature
gh pr ready
gh pr checks
```

由人或获授权的宿主检查 PR 最终差异、文档、剩余验证边界和当前 head 的远端 CI。
独立审阅应基于契约、差异和测试证据；执行模型自报完成不是审阅通过。
代码或 base 更新后，重新取得对应候选的检查与审阅，不能沿用旧提交的绿灯。
所有条件满足后再人工合并；合并后检查 main CI。发布仍是另一个发行步骤。

## 7. 自动合并与职责边界

当前 Flywheel 引擎负责本地状态、验收、提交和 packet 合并，**不自动 push、
创建 GitHub PR、等待远端 CI 或合并远端 PR**。本文的 GitHub 步骤由使用者或
获授权的宿主 Agent 执行，不能据此宣称引擎已实现远端交付自动化。

默认保留人工最终合并。若以后允许低风险任务自动合并，应先明确适用范围、
required checks、审阅要求和授权，再按仓库策略启用；Agent 的 pass 不能绕过门禁。
[GitHub auto-merge](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/automatically-merging-a-pull-request)
用于等待既定要求满足后合并，不是取消要求。分支更新及过期审阅规则见
[受保护分支](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)。

更多依据：[Git worktree](https://git-scm.com/docs/git-worktree)、
[Astra 模型与 effort](https://developers.openai.com/api/docs/models/gpt-6-astra)、
[Kimi 模型配置](https://www.kimi.com/code/docs/en/kimi-code/models)、
[Flywheel 设计](design.md)。
