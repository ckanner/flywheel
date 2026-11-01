# Mac 安装与首个 Flywheel 任务

本文覆盖从源码构建、DSH 接入、配置和 skills，到启动、观察、停止和恢复一个小任务。接口目标是已核对的 **DSH Mac 0.2.1-rc.5**；Go 引擎和 DSH 插件来自同一份源码。真实桌面、Seatbelt 和账号调用仍需在自己的 Mac 验收，CI 不能代替这些检查。

## 目录

1. [准备运行环境](#1-准备运行环境)
2. [获取并构建同一份源码](#2-获取并构建同一份源码)
3. [安装桌面入口和配置 SDK](#3-安装桌面入口和配置-sdk)
4. [配置模型与可选 skills](#4-配置模型与可选-skills)
5. [运行无推理检查](#5-运行无推理检查)
6. [准备首个小任务](#6-准备首个小任务)
7. [启动观察停止与恢复](#7-启动观察停止与恢复)
8. [升级与排错](#8-升级与排错)

## 1. 准备运行环境

| 依赖 | 用途与检查 |
|---|---|
| Git，已配置提交身份 | Flywheel 使用 worktree 和本地提交交付；执行 `git --version` |
| Go 1.27 或更高 | 构建引擎，并运行本文的小任务；执行 `go version` |
| Node.js 22 或更高，npm | 构建／运行 DSH worker；执行 `node --version`、`npm --version` |
| DSH Mac 桌面及其安装的 `dsh` 命令 | 在 About 核对具体版本；执行 `command -v dsh`、`dsh --version` |
| 已登录的 Kimi CLI | 默认 Plan、frontend、Garden 使用它；执行 `kimi --version`、`kimi --help` |
| 已登录的 Codex CLI | `hard` 执行路由使用它；执行 `codex --version` |
| SDK profile 的 DeepSeek 和 ChatGPT provider | 默认实现／评审使用；配置与授权见下一节 |

CLI 的登录和安装使用各自正式流程。Kimi 订阅、ChatGPT 订阅和 DeepSeek API 是三条独立渠道；安装 Flywheel 不会替这些工具登录。只运行默认小任务时不会使用 `hard` 路由，但所选 Plan、Execute、Review、Judge 均需要可用。

## 2. 获取并构建同一份源码

PR 合并到 `main` 后可以从它构建；审查中的 PR 则使用其实际分支。先用 `git log -1 --oneline` 记录自己的源码版本，不要假定 npm 的旧发行版已经包含 DSH 集成。

```sh
git clone https://github.com/ckanner/flywheel.git
cd flywheel
git log -1 --oneline
go build -o ./bin/flywheel ./cmd/flywheel
./bin/flywheel version
./bin/flywheel control --help
./bin/flywheel doctor --help
cd plugins/dsh
npm ci --ignore-scripts
npm run check
npm pack
```

产物是源码根的 `bin/flywheel` 和插件目录的 `ckanner-dsh-plugin-flywheel-0.1.0.tgz`。普通 `go build` 的版本可能显示 `dev`；`control --help` 应存在。后续示例将它安装到用户自己的目录：

```sh
cd ../..
mkdir -p "$HOME/.local/bin"
install -m 0755 ./bin/flywheel "$HOME/.local/bin/flywheel"
export PATH="$HOME/.local/bin:$PATH"
flywheel control --help
```

将实际路径加入自己的 shell 配置。此处的 `export` 只影响当前终端，不能改变已经运行的桌面应用环境。

也可下载对应 PR／提交的 GitHub Actions artifact。CI 按实际 runner 命名为 `flywheel-<OS>-<ARCH>`；解开外层 ZIP 后，包内 `flywheel-native.tar.gz` 保存可执行文件的权限，另含插件 `.tgz`。选择与 Mac CPU 对应的产物，不能把 Linux runner 的二进制用于 Mac。它们是 CI 构建，不是已签名、公证的正式安装包，也不意味着 npm 已发布。

## 3. 安装桌面入口和配置 SDK

用同一个插件包安装 worker 与桌面插件：

```sh
npm install --global /absolute/path/to/flywheel/plugins/dsh/ckanner-dsh-plugin-flywheel-0.1.0.tgz
dsh plugin --profile desktop add /absolute/path/to/flywheel/plugins/dsh/ckanner-dsh-plugin-flywheel-0.1.0.tgz
```

第二条必须使用桌面安装的 `dsh` 命令。独立 npm CLI 不能管理保留的 `desktop` profile。也可以在桌面 Plugins 页面选择本地包；安装后按宿主提示重新载入。确认 `command -v flywheel-dsh-worker` 指向新安装版本。

### 原生 SDK profile

worker 默认使用独立的 `sdk` profile。桌面已安装 provider，不代表 SDK profile 也已加载或授权。DeepSeek 使用 `deepseek-official` provider；GPT 原生路径需在 SDK profile 中配置 ChatGPT adapter，例如本轮核对的版本：

```sh
dsh plugin --profile sdk add dsh-plugin-chatgpt@0.1.17
```

随后按该 provider 的文档在对应 profile 完成授权，并保持一致的 `DSH_HOME`。不需要把凭证复制到任务契约或 Git。此处不把 `subagent_codex_pro` 当成原生 LLM provider；困难任务继续走独立 Codex CLI。

### Mac GUI 的 PATH

如果桌面找不到命令，创建一个自己的启动脚本，将其中每个 `/absolute/path` 替换为已检查过的实际路径：

```sh
#!/bin/sh
export PATH="/absolute/path/to/cli-bin:$PATH"
export FLYWHEEL_DSH_WORKER="/absolute/path/to/flywheel/plugins/dsh/dist/worker.js"
export FLYWHEEL_DSH_BIN="/absolute/path/to/dsh"
export FLYWHEEL_DSH_PROFILE="sdk"
export FLYWHEEL_KIMI_BIN="/absolute/path/to/kimi"
exec /absolute/path/to/flywheel "$@"
```

给脚本执行权限，并在桌面 profile 的 `cordis.patch.yml` 中配置：

```yaml
- id: flywheel
  config:
    flywheelBinary: /absolute/path/to/flywheel-launcher
    controlTimeoutMs: 60000
```

PATH 应包含 Node、所选 CLI，以及任务验收所需的 Go／其他工具。不要猜测 `.app/Contents/...` 的内部路径。权限及 worker 环境变量的完整说明见 [插件文档](../plugins/dsh/README.md)。

## 4. 配置模型与可选 skills

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

### 安装 skills

skills 是可选操作入口，不替代引擎，也不需要为了桌面面板而安装。默认安装到用户目录；加 `--project` 则安装到目标项目：

```sh
flywheel install-skills --project /absolute/path/to/target-repo
flywheel skills list
flywheel skills read flywheel-run
```

安装输出会给出 `.agents/skills` 的准确位置。按宿主支持的发现规则启用目录，不能从文件已复制推断 DSH／Kimi 已自动发现。也可直接使用 Go CLI。

重跑默认安装会刷新带 `.flywheel-snapshot` 标记的 Flywheel 快照，包括其中的手工修改；先备份定制内容。未标记的真实目录会拒绝覆盖，`--force` 也不改变这个规则；`--force` 只用于 `--link` 的开发模式。安装会在全局配置缺失时创建它，但不会改写已有配置。

## 5. 运行无推理检查

在实际目标仓库中，使用和桌面启动脚本一致的环境执行：

```sh
flywheel doctor --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --probe --json
```

第一条只查有效配置、本地命令版本、目录及已声明环境变量；不会检查凭证文件、发送模型 prompt 或运行 packet 验收。第二条对选中的 DSH 绑定做 SDK 初始化、cwd／沙箱握手及正常关闭；provider 初始化可能联网查询目录，但**没有推理请求**。两者都不创建 Flywheel 状态、运行锁、worktree 或全局配置。

结果按 pass／fail／warn／unknown／skipped 展示；有 fail 时退出码 1，JSON 仍完整可读。即使 `ok:true`，unknown 的账号授权、订阅额度与实际推理仍未验证。Kimi/Codex 在这里只检查命令，不会自动登录或试调用。DSH 同一账号、模型、effort、目录和权限只探测一次，总预算 40 秒另加清理窗口；超时后明确失败。

未指定任务时检查全部配置绑定，可能包括首个任务用不到的 Codex hard 路由。准备好任务后可把范围缩小为所需的 Plan／Execute route／Review／Judge（以及显式启用的 judge-panel）：

```sh
flywheel doctor --cwd "$PWD" --task-dir docs/add --probe --json
```

尚无 packet 时按规划前状态预检默认执行路由，规划后应再检查。坏契约会报告阻断项，不能用探测通过代替 schema 校验。若 cwd、权限、provider 或沙箱检查失败，先修正相应配置，不把权限改成 full access 绕过。桌面面板也提供“查看配置／检查环境／SDK 无推理探测”，使用同一个 CLI 后端；GUI 与终端环境不同的问题应在面板内复验。

这允许在准备契约时提前排查安装，但正式启动仍需至少一个已审阅的合法 packet。完成下一节的小任务准备后，再运行带 `--task-dir` 的检查。

## 6. 准备首个小任务

首次建议使用单独的小仓库，便于分清安装问题和业务代码问题。仓库附带的准备脚本会创建一个 Go 加法函数任务：初始实现故意错误，执行者需修复实现并补普通单测；独立 acceptance 测试不可修改。

```sh
bash /absolute/path/to/flywheel/docs/examples/create-first-task.sh /absolute/path/to/new-flywheel-demo
cd /absolute/path/to/new-flywheel-demo
git init
```

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

对于已有业务仓库，可以通过 `flywheel-new` 生成并审阅契约，或者用 `flywheel template --list` 查看模板后手工准备。示例 auth 模板不是通用可运行任务，其中路径和验收命令必须改为真实项目的命令。不要照抄后直接启动。

## 7. 启动、观察、停止与恢复

以下从目标仓库根运行，且会使用真实配置调用模型：

```sh
flywheel control start --task-dir "$PWD/docs/add" --cwd "$PWD"
flywheel control status --task-dir "$PWD/docs/add" --cwd "$PWD"
```

`ok: true` 只表示控制操作成功；`running` 表示引擎运行状态，最终任务结果看 `state.status`。启动 readiness 超时后先刷新状态和看日志，不重复启动。也可以在 DSH 输入区的 Flywheel 面板填入同样两个绝对路径，先查看配置和检查环境，再点击“启动／刷新状态／停止”。面板区分未初始化、已完成、暂停、停止和升级；修改路径会清除旧视图，但不会停止旧任务。

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

## 8. 升级与排错

升级顺序：停止活动任务 → 保存配置和定制 skills → 更新源码并重新构建 Go／插件 → 安装同一版本产物 → 对照新模板迁移所需配置 → 按宿主提示重载 → doctor → 小任务验证。旧配置不会自动覆盖；特别检查任务级文件是否遮住了刚修改的项目或全局设置。

| 现象 | 先检查什么 |
|---|---|
| 没有 `control` 命令 | `command -v flywheel` 指向的是否是新构建，GUI 是否仍使用旧绝对路径 |
| 桌面找不到 worker／kimi／codex | GUI 启动脚本的 PATH 与绝对路径；终端 export 不影响已有桌面进程 |
| SDK provider 未注册／未登录 | 独立 sdk profile、`DSH_HOME`、provider ID、对应 profile 的正常授权 |
| doctor 拒绝权限或沙箱 | 实际 DSH 版本与 Seatbelt／fs fence；不要改 full access 绕过 |
| 命令在终端通过、引擎内失败 | 检查验收输出、PATH、工具缓存权限和 `verify.env-allowlist`；不要把模型全部凭证注入验收 |
| 改了模型却没有生效 | 任务／项目／全局的完整文件优先级，随后重启任务引擎 |
| profile 不存在或字段拼错 | 对照配置模板检查当前优先级文件；profiles 只在该完整配置内查找，不跨文件合并，诊断不回显可能含凭据的原始字段值 |
| 改 channel 后账号没切换 | channel 是计量/配置别名；实际账号仍由 SDK profile/provider 的授权决定 |
| doctor 有 unknown 或 skipped | unknown 不等于通过，skipped 可能是非任务路由、mock 或没有请求 SDK probe |
| 费用显示未知 | 渠道未提供 cost／usage；unknown 不是免费，hard token budget 可能停止 |
| 启动超时但日志继续增长 | 先用 control status 观察就绪状态，不重复启动第二个引擎 |
| L1 在 exit 0 后仍拒绝 | 检查候选漂移、输出截断、超时／取消或进程收尾错误 |
| 提交／合并／持久化检查失败 | 检查保留 worktree、hook 和 Git 差异，修复原因后恢复，不强制清理 |
| skills 安装拒绝已有目录 | 它是未标记的真实目录；先审阅备份，不期待默认模式的 `--force` 覆盖 |

引擎只管理本地 Git 交付，不自动 push 或合并远端 PR。分支保护、远端 CI、发布和实际账号权限仍由所用仓库／服务处理。完整证据与实机边界见 [验证记录](validation/2026-11-01-results.md)。
