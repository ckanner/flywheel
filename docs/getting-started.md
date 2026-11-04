# Mac 安装与首个 Flywheel 任务

[English](en/getting-started.md) · 简体中文

本文适用于 **Flywheel 0.28.0 及以后的完整 npm 发行版**，覆盖 npm 一条命令安装 CLI、worker、DSH 插件与 skills，到配置、启动、观察、停止和恢复一个小任务。DSH 接口目标是已核对的 **Mac 桌面 0.2.1-rc.5**。真实桌面、Seatbelt 和账号调用仍需在自己的 Mac 验收，CI 不能代替这些检查。

下文的顶部 **Flywheel 会话视图**从 `0.28.1` 开始提供。`0.28.0` 仍使用底部弹层；需要把 CLI 和桌面插件升级到同一新版本，源码合并或重新载入旧版本不会带来新布局。CLI 和任务目录接口保持一致。

安装前用 `npm view @ckanner/flywheel version` 确认 registry 的 latest 已更新到所需版本；源码合并或 GitHub Release 不代表 npm 已发行。旧 0.22.1 没有本教程使用的安装与 DSH 接口，当前发行状态见[发行记录](releasing.md#3-失败恢复)。

## 目录

1. [准备运行环境](#1-准备运行环境)
2. [一条命令安装](#2-一条命令安装)
3. [桌面入口和 SDK 配置](#3-桌面入口和-sdk-配置)
4. [配置模型与 skills](#4-配置模型与-skills)
5. [运行无推理检查](#5-运行无推理检查)
6. [准备首个小任务](#6-准备首个小任务)
7. [启动、观察、停止与恢复](#7-启动观察停止与恢复)
8. [升级、排错与卸载](#8-升级排错与卸载)
9. [开发与备用构建](#9-开发与备用构建)

已有业务仓库另见[从需求准备到 PR 合并](workflow.md)：模型选择、任务 worktree、Draft PR 与合并时机。

## 1. 准备运行环境

| 依赖 | 用途与检查 |
|---|---|
| Git，已配置提交身份 | Flywheel 使用 worktree 和本地提交交付；执行 `git --version` |
| Node.js 22.18.0 或更高，npm | 安装 Flywheel 并运行 CLI 启动器／DSH worker；执行 `node --version`、`npm --version` |
| DSH Mac 桌面及其安装的 `dsh` 命令 | 在 About 核对具体版本；执行 `command -v dsh`、`dsh --version` |
| 已登录的 Kimi CLI | 默认 Plan、frontend、Garden 使用它；执行 `kimi --version`、`kimi --help` |
| 已登录的 Codex 程序 | Codex Pro 插件在 `hard` 路由中启动其 app-server；执行 `codex --version` |
| SDK profile 的 DeepSeek 和 ChatGPT provider | 默认实现／评审使用；配置与授权见下一节 |
| Go 1.27 或更高 | 仅本文的 Go 示例和源码构建需要；npm 安装 Flywheel 不需要 Go 工具链 |

安装器的前提是已有 Node／npm 和桌面 DSH；模型渠道可在安装后补齐。CLI 的登录和安装使用各自正式流程。Kimi 订阅、ChatGPT 订阅和 DeepSeek API 是三条独立渠道；安装 Flywheel 不会替这些工具登录、复制凭证或替换桌面程序。只运行默认小任务时不会使用 `hard` 路由，但所选 Plan、Execute、Review、Judge 均需要可用。

## 2. 一条命令安装

在能找到现有桌面 `dsh` 的终端运行：

```sh
npx -y @ckanner/flywheel@latest install
```

给 Agent 使用时，可以直接采用输出单个 JSON 文档的形式：

```sh
npx -y @ckanner/flywheel@latest install --json
```

两条命令选择一种即可。安装器按顺序完成：

1. 把本次安装器的精确版本装到 npm 全局目录，解析适合本机的 Go 平台包、同版本 DSH／Codex Pro workers 及精确插件依赖 `dsh-subagent-codex-pro@0.8.0`，核对 CLI 版本。
2. 从 Go 二进制安装全部 10 个 skills；全局配置不存在时生成默认配置，已有配置保留。
3. 写入 GUI 启动器和安装记录，使用绝对 Node／CLI 路径。
4. 通过现有桌面 `dsh` 注册精确版本的 Flywheel 插件，并通过 `plugin list` 验证安装结果。
5. 执行不带 `--probe` 的本地 `doctor`，汇总工具、配置及目录检查结果。

结果的 `ok` 与 `ready` 含义不同：**`ok:true` 表示软件安装完成，`ready:true` 表示本次本地 doctor 没有阻断项**。缺 Kimi／Codex 时允许 `ok:true, ready:false`，退出码仍为 0，具体缺项保留在 `diagnostic.checks`。这不代表账号已登录或真实推理通过；`authentication` 保持 `unknown`。安装步骤失败、取消、超时或诊断协议损坏则是安装失败，保留已经完成的步骤，修正原因后重跑。

### 安装参数

| 参数 | 作用 |
|---|---|
| `--prefix PATH` | npm 全局安装 prefix；默认使用 `npm prefix --global` |
| `--project PATH` | 把 skills 装到已存在项目的 `.agents/skills`，并在该项目做预检；CLI 仍按全局安装处理 |
| `--dsh-bin PATH` | 指定现有桌面 `dsh` 命令；默认依次使用 `FLYWHEEL_DSH_BIN` 与 PATH |
| `--profile NAME` | DSH 插件目标 profile；默认 `desktop`，与 worker 的 SDK profile 是两回事 |
| `--no-dsh` | 明确只安装 CLI、worker、共享 skills 和启动记录，跳过桌面插件注册 |
| `--no-global` | 配置当前已持久安装的包，不再执行 npm 全局安装；不应用来把临时 npx 缓存当作永久入口 |
| `--timeout-ms N` | 每个外部安装步骤的时限，默认 600000 毫秒，允许 100–1800000；取消／终止另有有界清理窗口 |
| `--json` | 只向 stdout 输出一个结构化安装结果，适合 Agent 自动处理 |

如果系统全局目录不可写，可以使用用户目录，并按实际位置指定桌面命令：

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local" --dsh-bin /absolute/path/to/dsh
export PATH="$HOME/.local/bin:$PATH"
flywheel version
```

将实际 prefix 的 `bin` 加入自己的 shell 配置后，后续终端也能直接使用 `flywheel`。GUI 通过自动生成的绝对路径入口启动，不依赖这条临时 `export`。不要为修复 prefix 权限问题盲目以 root 运行整个安装器；它需要写入当前使用者的 skills 与桌面 profile。

### 已经使用普通 npm 全局安装

普通 `npm install --global @ckanner/flywheel` 只放置包文件，不运行修改用户目录的 postinstall。随后显式完成配置：

```sh
flywheel setup --no-global
```

`install` 与 `setup` 支持相同参数；不加 `--no-global` 时两者都会确保本次精确版本的全局包已安装。选择 `--ignore-scripts` 不会破坏 Go／worker 的包内分发；显式 `setup` 仍需已有 DSH 及其插件安装所需的 registry 访问。安装器不会在后台下载或更新其他 harness。

## 3. 桌面入口和 SDK 配置

安装器已经调用正式接口 `dsh plugin --profile desktop add --save-exact --ignore-scripts @ckanner/dsh-plugin-flywheel@<本次精确版本>`，随后通过 `list` 核对版本。无需再分别安装 worker 或手动选择压缩包。该命令必须来自桌面安装；独立 npm DSH CLI 不能管理保留的 `desktop` profile。按照宿主提示重新载入，在顶部“对话／轨迹”所在的原生视图栏选择 **Flywheel**；未直接显示时检查该栏的更多入口。

这个视图用于控制和观察已经准备好的任务：看进度、人工处理项、实际模型和环境，显式启动、刷新或停止。它保留 DSH 的原生对话、轨迹、输入区、模型选择器和右侧栏；需求讨论与契约准备仍在原来的对话和 skills 中进行。视图不会自动发现别处启动的任务，需要选择相同的任务目录。

### 原生 SDK profile

worker 默认使用独立的 `sdk` profile。桌面已安装 provider，不代表 SDK profile 也已加载或授权。DeepSeek 使用 `deepseek-official` provider；GPT 原生路径需在 SDK profile 中配置 ChatGPT adapter，例如本轮核对的版本：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

随后按该 provider 的文档在对应 profile 完成授权，并保持一致的 `DSH_HOME`。不需要把凭证复制到任务契约或 Git。此处不把 `subagent_codex_pro` 当成原生 LLM provider。困难任务通过 `codex-pro` worker 直接使用已安装插件的公开 machine API，不需要 DSH 父模型再发一次工具调用。底层 Codex app-server 仍需本机 Codex 与登录；包依赖安装不代替它们。

SDK worker 的临时权限与工具策略只用于其子进程，不改写桌面 profile 或移除桌面已有能力。它限制自主委派、辅助模型压缩等工作，当前还禁用整个 web 工具；移到原生会话视图不会让 worker 自动继承桌面插件或轨迹。能力范围及后续检索接入边界见[插件说明](../plugins/dsh/README.md#权限上下文与报告归档)。

### Mac GUI 的自动入口

安装器在 `~/.config/flywheel/` 下生成 `npm-launcher` 和 `installation.json`。launcher 使用绝对 Node／CLI 路径，记录安装时已核对的 DSH 入口，并保留工具的绝对 PATH 目录；不会把临时 npx 缓存路径当成正式工具目录。npm CLI 启动器再解析与自身版本一致的 worker。

桌面插件在没有显式 `flywheelBinary` 配置时自动读取这份记录，核对 launcher 的普通文件类型、可执行位和 SHA-256 后启动。**不需要自己写启动脚本或改 `cordis.patch.yml`。** 已有显式配置优先，安装器不会覆盖；若要采用自动入口，先审阅旧配置再自行移除该覆盖项。记录不存在时沿用 PATH 查找；已有记录损坏或 launcher 被修改时报告错误，不静默切换另一份引擎。

更换 Node 版本、移动全局 prefix、安装新的 Kimi／Codex 或补齐验收工具后，在这些工具均可见的终端运行 `flywheel setup --no-global`，必要时加原来的 `--prefix`／`--dsh-bin`，再按宿主提示重新载入。launcher 的用户修改会被保护，处理方法见升级章节。工具 PATH 应包含 Node、所选 CLI 和项目验收所需的 Go／其他工具。不要猜测 `.app/Contents/...` 的内部路径。权限及 worker 环境变量见[插件文档](../plugins/dsh/README.md)。

## 4. 配置模型与 skills

配置优先级是：**任务目录 → 项目根 → 全局 → 二进制内嵌模板**，选择第一个完整文件，不逐字段合并。默认全局路径是 `~/.config/flywheel/flywheel.config.json`；`FLYWHEEL_CONFIG` 只替换全局路径，不能越过已有任务／项目文件。

查看当前二进制的配置：

```sh
flywheel template flywheel.config.example.json
```

为一个项目采用新默认表时，在确认没有需保留的同名配置后，从目标仓库根导出：

```sh
flywheel template flywheel.config.example.json > flywheel.config.json
```

`loops` 配置 Plan／Review／Judge 等阶段，`routes` 配置 Execute 的 default／frontend／hard，`channels` 描述账号渠道；`drivers` 只选择 `real` 或 `mock`。默认模型表见 [项目 README](../README.md#3-当前默认路由)。DSH 的 `provider` 必须是 SDK 实际注册的 ID；`channel` 可使用自己的渠道名称。旧配置不写 provider 时继续使用 channel 作为 provider ID。Flash 的 effort 是 `off/low/high/max`，没有 `medium`。

需要集中修改模型时，查看另一份完整示例：

```sh
flywheel template flywheel.profiles.example.json
```

它把模型定义放进 `profiles`，如 `loops.review: {"profile":"gpt-review"}`；阶段可显式覆盖 model／thinking／channel 等字段。两个配置示例选一个作为起点，不要让任务文件只写部分 profiles、再期待与项目文件自动合并。profile 不是 DSH profile；渠道别名也不会自动切换账号。完整字段、覆盖规则和迁移示例见[配置指南](configuration.md)。

编辑后先查看实际解析结果，确认没有被更高优先级文件遮住：

```sh
flywheel config inspect --cwd /absolute/path/to/target-repo --json
```

验收命令使用受限环境，而不是继承模型进程的全部变量。若项目需要自定义 `GOCACHE`、`GOPATH` 等工具配置，在 `verify.env-allowlist` 中列出所需变量名，并确保它们在引擎启动环境中有值；只记录变量名，不把密钥内容写入配置。

### 查看或补装 skills

一条命令安装已经包含全部 10 个 skills，默认在 `~/.agents/skills`；使用它们仍是可选的，桌面面板和 CLI 可以独立工作。需要另外安装到目标项目时运行：

```sh
flywheel install-skills --project /absolute/path/to/target-repo --json
flywheel skills list
flywheel skills read flywheel-run
```

安装输出会给出 `.agents/skills` 的准确位置。按宿主支持的发现规则启用目录，不能从文件已复制推断 DSH／Kimi 已自动发现。也可直接使用 Go CLI。

快照的 `.flywheel-snapshot` 清单记录文件内容 hash、类型与模式。重跑会跳过相同内容，更新未被修改的自有快照；用户修改、新增／删除文件、链接、未标记目录或损坏清单会保留并报告冲突。每个 skill 独立采用暂存、rename 和失败回滚，不会覆盖手工修改。旧纯文本标记只有在整个目录与当前内嵌内容相同时才能安全迁移。`--force` 只用于 `--link` 开发模式，不能绕过默认保护。安装会在全局配置缺失时创建它，但不会改写已有配置。

## 5. 运行无推理检查

在实际目标仓库中执行：

```sh
flywheel doctor --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --probe --json
```

需要核对桌面的自动入口时，可把命令开头的 `flywheel` 换成 `"$HOME/.config/flywheel/npm-launcher"`；最后仍应在桌面面板内复验，因为 GUI 的其余环境可能不同。

第一条只查有效配置、本地命令版本、目录及已声明环境变量；不会检查凭证文件、发送模型 prompt 或运行 packet 验收。第二条对选中的 DSH 绑定做 SDK 初始化、cwd／沙箱握手及正常关闭；provider 初始化可能联网查询目录，但**没有推理请求**。两者都不创建 Flywheel 状态、运行锁、worktree 或全局配置。

结果按 pass／fail／warn／unknown／skipped 展示；有 fail 时退出码 1，JSON 仍完整可读。即使 `ok:true`，unknown 的账号授权、订阅额度与实际推理仍未验证。Kimi/Codex 在这里只检查命令；Codex Pro 额外实际导入精确版本的公开 API，不会自动登录或试调用。Codex Pro 的完整沙箱策略只能事后审查，所以对应 sandbox 项会显示 warn，模型／配置／认证保持 unknown；`--probe` 也不会为它发送推理。DSH 同一账号、模型、effort、目录和权限只探测一次，总预算 40 秒另加清理窗口；超时后明确失败。

未指定任务时检查全部配置绑定，可能包括首个任务用不到的 Codex hard 路由。准备好任务后可把范围缩小为所需的 Plan／Execute route／Review／Judge（以及显式启用的 judge-panel）：

```sh
flywheel doctor --cwd "$PWD" --task-dir docs/add --probe --json
```

尚无 packet 时按规划前状态预检默认执行路由，规划后应再检查。坏契约会报告阻断项，不能用探测通过代替 schema 校验。若 cwd、权限、provider 或沙箱检查失败，先修正相应配置，不把权限改成 full access 绕过。桌面面板也提供“查看配置／检查环境／SDK 无推理探测”，使用同一个 CLI 后端；GUI 与终端环境不同的问题应在面板内复验。

这允许在准备契约时提前排查安装，但正式启动仍需至少一个已审阅的合法 packet。完成下一节的小任务准备后，再运行带 `--task-dir` 的检查。

## 6. 准备首个小任务

已有业务仓库应先建立任务集成分支和独立 worktree，再生成契约；启动时的 `cwd` 指向该 worktree。完整步骤见[从需求准备到 PR 合并](workflow.md)，下面的小仓库示例用于首次运行验收。

首次建议使用单独的小仓库，便于分清安装问题和业务代码问题。npm 包附带的准备脚本会创建一个 Go 加法函数任务：初始实现故意错误，执行者需修复实现并补普通单测；独立 acceptance 测试不可修改。本示例需要 Go 1.27 或更高。

```sh
FLYWHEEL_PACKAGE="$(npm root --global)/@ckanner/flywheel"
bash "$FLYWHEEL_PACKAGE/docs/examples/create-first-task.sh" "$HOME/flywheel-demo"
cd "$HOME/flywheel-demo"
git init
```

如果安装时使用了自定义 prefix，用 `npm root --global --prefix "$HOME/.local"` 等实际 prefix 定位包目录；不需要克隆源码。

脚本要求目标目录不存在，只写示例文件并读取本机 `flywheel template`；它不调用模型，也不自动提交。若 Flywheel 不在 PATH，可在调用脚本时设置 `FLYWHEEL_BIN=/absolute/path/to/flywheel`。

生成的关键文件：

| 文件 | 用途 |
|---|---|
| `sum.go` | 待实现的函数 |
| `acceptance/sum_test.go` | 冻结验收，禁止 executor 改动 |
| `docs/add/spec.md` | 两条需求与一个里程碑 |
| `docs/add/task-packet-M01-T01.md` | default 路由、写入边界、L1／L2 和评审要求 |
| `flywheel.config.json` | 从实际二进制导出的项目完整配置 |
| `.gitignore`、任务目录 `.gitignore` | 排除 coverage 和易变运行文件，保留状态／报告 |

先逐条运行 packet 中的 L1：

```sh
go test ./...
go test -coverprofile=coverage.out . &&
  go tool cover -func=coverage.out |
  awk '$NF != "100.0%" && $NF != "100%" { bad=1 } END { exit bad }'
```

初始失败是预期结果。把实际退出码和输出摘要写入 `spec.md` 的 dry-run evidence；不要填未经运行的结果。审阅需求、命令、配置和冻结边界后，将准备好的初始任务提交：

```sh
git add go.mod sum.go acceptance .gitignore flywheel.config.json docs/add
git commit -m "docs: prepare first flywheel task"
```

Git 提交身份必须已经按自己的设置配置。此时不应创建 `state.json`，它由引擎初始化。coverage 输出已被忽略，所以验收运行不会把生成物误判为候选代码漂移。

对于已有业务仓库，`flywheel-new` 使用当前 DSH 会话的模型生成契约；复杂需求建议选已授权的 `gpt-6-astra`、`high`，简单明确任务可沿用 Flash。它不会自动读取 `loops.plan` 来换模型，后者控制引擎运行后的 Kimi 规划／修订。可以通过 `flywheel-new` 生成并审阅契约，或者用 `flywheel template --list` 查看模板后手工准备。示例 auth 模板不是通用可运行任务，其中路径和验收命令必须改为真实项目的命令。不要照抄后直接启动。

## 7. 启动、观察、停止与恢复

以下从目标仓库根运行，且会使用真实配置调用模型：

```sh
flywheel control start --task-dir "$PWD/docs/add" --cwd "$PWD"
flywheel control status --task-dir "$PWD/docs/add" --cwd "$PWD"
```

`ok: true` 只表示控制操作成功；`running` 表示引擎运行状态，最终任务结果看 `state.status`。也可以在顶部 **Flywheel** 视图填入同样两个绝对路径，按需展开“模型配置与环境检查”，再点击“启动／刷新状态／停止”。视图区分未初始化、运行、已完成、暂停、停止和升级，标明最近刷新时间；它不会自动轮询。

宿主保留会话绑定期间，路径草稿与快照会按会话保留，切换“对话／轨迹／Flywheel”不丢失；重新载入插件或桌面后可能需要重新选择路径。修改路径清除旧结果并忽略迟到响应，不会停止旧任务。每个会话只等待一个控制请求；启动或停止等待期间锁定目录输入。

启动或停止超时、断连后，后台任务的结果可能尚未确认。保留原任务目录，先“刷新状态”和查看日志；界面在成功核对该任务前禁止继续启动／停止，也不会自动重试。已有快照只是上次结果，不能凭页面关闭、切换会话或停止按钮失去等待状态推断引擎已经结束。原生轨迹保留父会话自己的记录，worker 完整历史不自动合并到其中。

如果宿主明确报告请求在启动控制进程前已被拒绝，例如目录不存在，则修正目录或环境后可以重新提交；这与响应丢失后的结果未确认不同。界面只接受宿主的结构化确认，不通过错误文字推断任务没有执行。

需要人工在 Review 前检查结果时，使用前台监督模式；它与后台 start 二选一：

```sh
flywheel run docs/add --pause-before review
```

`control start` 当前不接受 `--pause-before`。`--only execute` 会运行 Execute 子环及 L1，直至通过或遇到失败出口；它不是一次模型调用。`--only review` 只返回评审裁决，不会自动跨到 Plan 修订。单环命令详见各内嵌 skill。

观察人话摘要和日志：

```sh
flywheel status docs/add
tail -n 80 docs/add/orchestrator.log
git log -5 --oneline
```

停止与恢复：

```sh
flywheel control stop --task-dir "$PWD/docs/add" --cwd "$PWD"
flywheel control start --task-dir "$PWD/docs/add" --cwd "$PWD"
```

仅在确认上次停止／暂停、检查并处理实际错误后恢复。恢复会重新核对契约和候选证据；不要手改 `state.json`，不要因为想重试就删报告、工作区或运行锁。`stopped` 表示引擎生命周期结束，已记录的清理错误仍需处理，不能据此断言任意逃逸进程都已清理。

首次实机验收应确认：任务 completed、需求都被覆盖、冻结验收没改、报告与独立评审存在、代码已在本地 Git 保存／合并、停止恢复符合预期；同时记录真实模型、延迟和用量。模型自报“完成”和 `control` 的 `ok` 都不能代替这些结果。

## 8. 升级、排错与卸载

### 升级或修复安装

先显式停止活动任务并确认停止结果，保存配置和定制 skills，再运行与首装相同的命令：

```sh
npx -y @ckanner/flywheel@latest install
```

自定义 prefix、项目 skills 或 DSH 入口时保留原参数。想固定某个已发布版本，可把 `@latest` 换成该精确版本，例如在 registry 确认 0.28.0 已发布后使用 `@0.28.0`；安装器始终为 CLI、平台包、worker 和桌面插件选择本次安装器的同一版本。已安装包的配置修复可用 `flywheel setup --no-global`，它不会将 CLI 升级到别的版本。

从 0.27.x 升级到本版后，要使用新的默认插件路径，应在有效配置的 `routes.hard`（或其 profile）设置 `"harness": "codex-pro"`，保留所选 model／thinking／channel。先用 `config inspect` 找到实际配置来源；不要把 external subagent 的工具名写成 DSH provider。

安装后对照新模板审阅需要迁移的配置，按宿主提示重新载入，运行 doctor，再用小任务验证。已有配置不会自动覆盖；特别检查任务级文件是否遮住了刚修改的项目或全局设置。安装不是跨 npm、DSH 与全部 skills 的单个事务：失败时报告已完成步骤，不擅自回退包管理器或删除用户文件。修正原因后重跑会重新核对版本与内容。

### skills 或 launcher 冲突

遇到内容冲突时，不要删整个 `.agents`、配置目录或修改 hash 清单来绕过校验。根据失败报告定位具体文件，先备份，再对照当前模板审阅差异。需要保留的定制可整理到另外命名的个人／项目 skill；确认后将**具体冲突目录**移到备份位置，重跑 `setup --no-global` 或对应范围的 `install-skills --json` 生成官方快照。旧版本纯文本标记不能证明目录未被修改，因此内容与新快照不一致时也需要这样处理。

launcher 被手工修改时，先备份修改。希望采用自动入口，可以将该 launcher 移到备份位置后重跑 setup；希望继续自定义，则在桌面配置中显式指定自己的 `flywheelBinary`。不要手改 `installation.json` 的 SHA-256 把未知内容标成安装器生成。若提示安装锁存在，先确认另一个安装进程是否仍在运行；只有确认进程已结束后才处理报告中的残留锁。

### 常见问题

| 现象 | 先检查什么 |
|---|---|
| 没有 `install` 或 `control` 命令 | `npm view @ckanner/flywheel version` 与 `flywheel version`；是否仍是旧 0.22.1，PATH 或显式 `flywheelBinary` 是否指向旧版本 |
| 仍看到底部大弹层 | 已发布的 `0.28.0` 是旧布局；核对是否安装了包含会话视图修复的构建，再按宿主提示重新载入 |
| 顶部未直接显示 Flywheel | 检查视图栏更多入口、插件启用状态和实际版本；`/flywheel` 与 CLI 仍可用于诊断 |
| 安装 `ok:true`，但 `ready:false` | 安装已完成；按 `diagnostic.checks` 补齐当前配置需要的 CLI、目录或环境变量，再运行 doctor |
| npm 全局目录权限错误 | 使用自己可写的 `--prefix`，并把对应 `bin` 加入 shell PATH |
| desktop profile 被拒绝 | `--dsh-bin` 是否指向桌面安装的命令，而非单独 npm 安装的 DSH CLI |
| 桌面找不到 worker／kimi／codex | 查看 `installation.json` 对应的自动 launcher、显式 `flywheelBinary` 和工具 PATH；工具变化后重跑 setup，终端 export 不会改变已有桌面进程 |
| launcher 缺失或 hash 不符 | 保存用户修改，修复持久 npm 安装后重跑 setup；不手改安装记录绕过校验 |
| SDK provider 未注册／未登录 | 独立 sdk profile、`DSH_HOME`、provider ID、对应 profile 的正常授权 |
| doctor 拒绝权限或沙箱 | 实际 DSH 版本与 Seatbelt／fs fence；不要改 full access 绕过 |
| 命令在终端通过、引擎内失败 | 检查验收输出、PATH、工具缓存权限和 `verify.env-allowlist`；不要把模型全部凭证注入验收 |
| 改了模型却没有生效 | 任务／项目／全局的完整文件优先级，随后重启任务引擎 |
| profile 不存在或字段拼错 | 对照配置模板检查当前优先级文件；profiles 只在该完整配置内查找，不跨文件合并，诊断不回显可能含凭据的原始字段值 |
| 改 channel 后账号没切换 | channel 是计量/配置别名；实际账号仍由 SDK profile/provider 的授权决定 |
| doctor 有 unknown 或 skipped | unknown 不等于通过，skipped 可能是非任务路由、mock 或没有请求 SDK probe |
| 费用显示未知 | 渠道未提供 cost／usage；unknown 不是免费，hard token budget 可能停止 |
| 启动／停止结果未确认 | 刷新同一任务并查看日志；超时或断连不证明任务未启动或已停止，不能自动重试变更 |
| L1 在 exit 0 后仍拒绝 | 检查候选漂移、输出截断、超时／取消或进程收尾错误 |
| 提交／合并／持久化检查失败 | 检查保留 worktree、hook 和 Git 差异，修复原因后恢复，不强制清理 |
| skills 安装拒绝已有目录 | 查看具体冲突；可能是用户修改、未标记目录、损坏清单或旧标记。按上面的备份恢复流程处理，`--force` 不绕过默认保护 |

### 卸载

先通过 Flywheel 面板或 `flywheel control stop` 停止活动任务并确认结果，再在 DSH 的插件管理入口从所用 profile 移除 `@ckanner/dsh-plugin-flywheel`，按宿主提示重新载入。最后卸载全局 npm 包：

```sh
npm uninstall --global @ckanner/flywheel
```

使用自定义 prefix 时为卸载命令加上同一个 `--prefix`。Flywheel 没有隐式卸载清理脚本：npm 卸载不会删除用户的任务账本、worktree、配置、skills 或安装记录，也不会卸载 DSH、Kimi、Codex 或账号凭证。保留这些文件便于恢复；需要清理时，由用户按实际安装报告逐项审阅所属文件，特别保留自己的修改，不批量删除整个 `~/.agents` 或 `~/.config`。桌面插件应先移除，避免遗留入口继续调用已卸载的 CLI。

引擎只管理本地 Git 交付，不自动 push 或合并远端 PR。分支保护、远端 CI、发布和实际账号权限仍由所用仓库／服务处理。完整证据与实机边界见 [验证记录](validation/2026-11-01-results.md)。

## 9. 开发与备用构建

以下用于贡献代码、检查未发布分支或离线备份；普通用户使用 npm 安装。需要 Git、Go 1.27 或更高、Node.js 22.18.0 或更高：

```sh
git clone https://github.com/ckanner/flywheel.git
cd flywheel
git log -1 --oneline
go build -o ./bin/flywheel ./cmd/flywheel
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
make quality-gate
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

普通 `go build` 的版本可能显示 `dev`。`build:npm` 在隔离目录先构建 worker，再为 macOS／Linux 的 arm64／x64 打包四个平台包、meta 包和 DSH 插件，共六个同版本 npm 包；输出在 `dist-npm/`，同时保存候选文件清单与校验值。构建不能悄悄改写仓库内的版本或复用旧 worker。`test:install` 用这些真实 tarball 和隔离的本地 registry／用户目录测试安装；DSH 命令是协议桩，不触碰自己的桌面 profile 或模型账号。

Actions artifact 和本地 tarball 是构建／审查产物，不代表 npm 已发布，也不是已签名、公证的 Mac 安装程序。备用分发时必须保留同版本依赖，并选择正确平台；单独拿一个 meta tarball 不能证明所有可选平台依赖已在离线缓存。普通 npm 安装完成且依赖齐全后，CLI 与内嵌 skills 不需要下载源码；首次包获取、DSH 插件注册和实际模型调用各自仍有网络需求。

开发者可使用 `flywheel install-skills --link` 链接源码中的 skills；这是明确的开发模式，不是发布安装流程。源码、独立 Go 二进制或自定义 worker 的接入见[插件文档](../plugins/dsh/README.md)，不得将本机协议桩通过写成用户 Mac 或真实账号通过。

维护者发布到 npm 的认证配置、六包顺序和失败恢复见[发行指南](releasing.md)。普通安装者不需要 npm 发布权限，也不需要配置 GitHub OIDC。
