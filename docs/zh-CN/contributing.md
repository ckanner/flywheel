# 为 Flywheel 贡献

[English](../en/contributing.md) · 简体中文 · [文档索引](../README.zh-CN.md)

开始前阅读[项目介绍](../../README.zh-CN.md)、权威[系统设计](../design.md)和
[AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md)。Flywheel 是确定性的 Go 编排器：模型负责有边界的
规划、实现和评审，代码负责状态转移、验证、预算和进程生命周期。贡献必须保持这项分工。

## 许可证与范围

Flywheel 使用 [MIT 许可证](../../LICENSE)。复制软件或其中的重要部分时保留版权及
许可证声明；第三方 notices 和 bundled 依赖的来源记录也须保留，仓库许可证不替代
第三方各自的条款。

Issue、PR 和测试夹具不得包含凭证、私有项目内容或未经脱敏的模型／会话日志。
敏感安全问题应先请求私下报告渠道，再提供细节；仓库没有另行公布响应时限承诺。

## 选择分支与执行目录

为当前改动创建或复用专用 worktree。若预期 remote 为 `origin`、基线为 `main`，
新任务可以这样准备：

```sh
git status --short
git fetch origin
git worktree add -b task/my-change ../flywheel-my-change origin/main
cd ../flywheel-my-change
```

复用前检查已有分支和目录，保留其他工作。需求契约、配置、检查与提交应位于同一任务
集成 checkout。原仓库仍是 Session 的项目工作区，任务集成 checkout 和 packet worktree
是实际执行位置。使用 Flywheel 开发时遵循生效的 `worktree.enabled` 策略；引擎把已验收
packet 合入集成 checkout 的当前分支，不会自动合入 `main`。

Worktree 隔离 checkout 文件和 index，不是安全沙箱。串行执行保留文档说明的 `in-place`
回退；自动管理开启的并行执行必须成功创建独立 worktree。关闭自动管理后，共享执行目录
使用整批验证与交付。目录选择、外部 worktree 登记和恢复见[工作流](workflow.md)及
[控制台](flow-console.md)指南。

较大实现前先明确需求、范围和可执行验收。当已有值得审阅的方案或改动时创建 Draft PR；
一个 PR 可以覆盖多个 packet。保持提交聚焦，标题使用英文 Conventional Commits，例如
`fix: preserve cleanup evidence`。Agent 应遵守用户已有的提交、推送和合并授权，不重复
询问已经授权的步骤，也不把该授权扩大到另一种操作。

## 按可观察契约实现

[设计不变量](../design.md#2-设计不变量)和 AGENTS 定义核心约束：

- Go 与 `internal/state` 拥有任务状态。写入使用 revision compare-and-set 和原子替换；
  模型与 skills 不编辑 `state.json`。
- 契约 schema 定义合法输入，在加载时验证。修改错误分类等线协议前先更新设计。
- 通过可观察结果验证完成、取消、超时和清理；终态文字或 worker 退出不能证明子进程组已停止。
- 验证和独立评审必须绑定最终提交、合并的同一候选。门禁或 Git 交付失败时保留成果与证据。
- 缺少 usage、cost、权限或能力证据时保持 unknown，不能写成成功或零成本。
- Skills 校验输入并调用 CLI，桌面插件消费 Go 控制接口；二者都不实现另一套状态机或重试循环。

测试应覆盖有意义的失败行为，使用范围明确的 I/O 注入点。每个被计量的 Go 函数必须达到
100% statement coverage，仅排除 `cmd/flywheel` 的单行入口桩。保留该门禁，通过行为断言
验证结果，不为提高百分比而照抄实现逻辑。

## 构建与验证

源码开发需要 **Go 1.27 或更高**及 Git。Flywheel npm 运行时需要 **Node 22.18.0 或更高**，
所选 DSH 版本可能有更高要求。CI 与发行固定使用 **Node 24.22.1／npm 12.2.1**；复现包构建
和安装证据时使用这组版本。

在仓库根目录执行与改动相关的检查：

```sh
make quality-gate
npm --prefix plugins/dsh ci --ignore-scripts
npm --prefix plugins/dsh run check
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/drivers -run '^Test(DSH|CodexPro|ClaudeCodePro)NodeWorkerOfflineBridge$' -count=1
npm test
npm run build:npm
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

| 改动 | 必需验证 |
|---|---|
| Go 引擎 | `make quality-gate`：vet、格式、测试和覆盖率 |
| DSH 插件或 worker | 插件检查及适用的 Go → Node 跨进程桥接 |
| npm 安装或发行 | 根 npm 测试、单包候选构建、安装同一个 tarball |
| 文档 | 链接、锚点、代码围栏、可运行示例、翻译一致性，以及受影响的内嵌／包内容 |

Claude 桥接要求 worker 的依赖树中存在精确 0.5.0 optional machine API；因未安装 peer
而跳过不能算该后端通过。使用源码候选时保留已审查 source／tarball 身份。Codex Pro 的
0.10.0 bundled 源码包同样由实际 integrity 与 [vendor 记录](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)
固定。

生命周期、状态或进程改动还应复现 Linux CI 的 race 检查：

```sh
go test -race ./internal/machine ./internal/verify ./internal/watchdog ./internal/drivers ./internal/cli ./internal/modules ./internal/diagnostic -count=1
```

会话质量或评估工具改动使用[会话评估指南](session-evaluation.md)中的固定离线配对场景：

```sh
make build
FLYWHEEL_EVAL_BINARY="$PWD/flywheel" npm run test:sessions
```

默认测试用离线夹具执行真实 Git、进程、worker 和公开适配器包。安装测试在隔离 registry
与用户目录中消费真实候选 tarball，桌面注册使用 CLI 夹具；依赖下载仍需 registry 接入或
文档规定的已验证夹具。这些检查不证明真实账号可用、模型质量、订阅额度或 Mac 桌面通过。

CI 在 Ubuntu 和 macOS 使用同一候选，Linux 另跑 race。按实际执行结果报告：无法运行的 CI
仍是未运行，交叉编译 Mac 二进制也不证明其运行行为。适用门禁与具体剩余风险验证完成后，
无需重复扩大测试范围。

## 保持文档结构清晰

行为变更和说明它的文档应在同一个 PR 交付。实现前先识别受影响的指南，再同步更新对应
中英文。局部改动不要求重写全部指南，也不要求运行所有仓库测试。

### 按问题选择权威来源

| 要说明的内容 | 权威来源 | 使用方式 |
|---|---|---|
| 架构、职责与不变量 | [系统设计](../design.md)，尤其是设计不变量 | 作为预期约束，不是可任选的描述。 |
| 合法字段、类型、必填值与线协议 | [Schema 与模板](https://github.com/ckanner/flywheel/tree/main/templates)、对应解析器及 [worker 协议](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md) | 用适用 schema 和已实现接口核对示例；schema 接受字段不等于后端支持该能力。 |
| 默认值、优先级、迁移与模型绑定 | [本次源码版本内嵌的模板](https://github.com/ckanner/flywheel/tree/main/templates)与[配置解析、规范化实现](https://github.com/ckanner/flywheel/tree/main/internal/config) | 描述已安装版本时，再核对该版本的实际模板及 `config inspect` 输出。区分默认值、字段缺省的兼容行为和用户生效配置，不能用旧安装推导新源码默认。 |
| CLI、worker 与桌面行为 | 当前 [CLI](https://github.com/ckanner/flywheel/tree/main/internal/cli)、[driver](https://github.com/ckanner/flywheel/tree/main/internal/drivers)、[DSH 插件](https://github.com/ckanner/flywheel/tree/main/plugins/dsh)实现及相关测试 | 按实际执行路径核对命令、生命周期边界、权限支持和界面标签。 |
| 打包、发行顺序与自动门禁 | [Package manifest](https://github.com/ckanner/flywheel/blob/main/package.json)、lockfile、[scripts](https://github.com/ckanner/flywheel/tree/main/scripts) 和 [workflows](https://github.com/ckanner/flywheel/tree/main/.github/workflows) | 说明实际存在的命令与门禁；文案规定不等于 CI 已自动执行。 |
| 已完成的测试、发行或评估 | [带日期的验收记录](../validation/README.md)与[评估证据](../evaluation/README.md) | 结论只适用于记录的源码、平台、输入和结果，历史结果不证明当前 registry 或真实账号状态。 |

设计和实现不一致时，先记录差异，再依据任务已经接受的要求和设计约束确认预期行为。
实现违反契约就修正实现；任务本身包含设计变更时，先更新设计，再在同一 PR 对齐代码、
测试和指南。不能通过弱化不变量默认为现有代码正名，也不能把计划能力写成已实现。
预期行为仍未明确时，如实说明限制，不自行编造结论。

### 将内容放在固定位置

根目录和插件的 `README.md` 默认英文，对应中文为 `README.zh-CN.md`；`README.en.md`
只保留兼容入口，不维护第三份正文。现行用户与维护指南放在 `docs/en/` 和 `docs/zh-CN/`，
使用相同文件名，提供往返语言链接和对应的文档首页入口。

| 内容 | 归属位置 |
|---|---|
| 项目介绍与首个操作命令 | 根目录 README |
| 现行安装、工作流、配置与维护方式 | `docs/en/` 与 `docs/zh-CN/` |
| 当前架构与不变量 | `docs/design.md` |
| 设计提案及当时背景 | `docs/plans/` |
| 已执行检查、环境、证据与限制 | `docs/validation/` 与 `docs/evaluation/` |
| 历史 review 与已被替代的材料 | `docs/reviews/` 与 `docs/archive/` |
| 包专属接口与来源记录 | `plugins/dsh/` |
| 贡献者与 coding agent 操作规则 | 双语贡献指南和 [AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md) |

本仓库的项目文档与 Flywheel 在目标仓库生成的 `docs/<task>/` 契约、运行账本是不同内容。

### 从改动定位受影响文档

使用下表检查影响范围，仅更新因本次变更而受到影响的说明；涉及设计、schema 或协议
变更时，还须遵守对应契约规则。

| 改动 | 需要检查并按影响更新的文档 |
|---|---|
| 安装、升级、环境要求或 setup | 根目录与插件 README、`getting-started.md`、安装示例及包内容 |
| CLI 命令、控制响应、worker 或后端支持 | `getting-started.md`、`workflow.md`、对应后端指南、插件 README／`PROTOCOL.md` 及相关技能说明 |
| 配置、默认值、账号、模型或权限 | `configuration.md`、`model-policy.md`、相关 README 表格、实际模板说明及迁移步骤 |
| 调度、worktree、执行位置或交付 | `workflow.md`、`configuration.md`、`flow-console.md`、设计章节及相关 skills |
| 会话续用、恢复、接管或计量 | `session-recovery.md`、配置／工作流／控制台相关章节、设计及受影响的 worker 契约 |
| 桌面 UI、任务发现或操作控制 | `flow-console.md`、插件 README，以及提到该界面的快速开始步骤 |
| 评估工具、场景或结果解释 | `session-evaluation.md` 或 `jev.md`、对应输入输出契约；实际执行新检查后补带日期证据 |
| 包布局、许可证或发行行为 | `releasing.md`、相关安装步骤、manifest／lockfile、包内链接及适用的许可／来源记录 |
| 文档移动、重命名或标题变化 | 两份文档索引、语言链接、所有入链路径／锚点、包清单、内嵌引用、脚本与测试 |
| 贡献者或 AI 操作规范 | [AGENTS.md](https://github.com/ckanner/flywheel/blob/main/AGENTS.md)、两份贡献指南及导航，保持要求一致 |

受影响的英文与中文指南必须在同一 PR 更新。翻译要保持含义一致：命令、模型 ID、JSON
字段、示例、默认值、权限、失败行为和能力限制均须对应，不能在另一语言中承诺更大范围
的能力或省略已有边界。

### 保持结构易读、历史可信

直接更新负责该行为的现有章节，仅在必要时增加明确命名的小节。每页保留一个主标题、
合理的标题层级、简洁介绍、带语言类型的代码围栏和可读表格，标题及列表前后保留空行。
标明片段和占位符，统一项目工作区、执行目录、任务、packet 与 session 等术语。
写具体操作说明，不保留聊天式过程记录。不要把开发更新前插到介绍之前，也不要在许可证
之后追加无关状态段落。版本变化写入 `CHANGELOG.md`，带日期的执行证据放入验收记录。

历史方案、评审、归档和验收记录保留原语言及当时背景。保留原有日期、commit、tree、版本、
hash、输入、平台以及通过／失败／跳过／未运行结果。修正历史错误时明确标注“更正”，
说明更正内容与原因；不能静默改写旧结果，或用新候选身份替换旧证据。后续完成情况写入
新的带日期记录，需要时从旧记录链接过去。历史材料无需全部翻译，索引应明确其语言和状态。

### 验证受影响的文档

请求审查前，核对本次改动适用的项目：

- [ ] 在同一 PR 更新负责该行为的章节及对应中英文，保持含义等价、文件名相同，
  并提供往返语言链接。
- [ ] 用相关设计、CLI、schema 和源码模板核对行为及默认值；已完成测试的声明必须有
  对应的带日期证据，尚未解决的差异和验证边界需明确说明。
- [ ] 路径或标题变化时，用 `rg` 反查指南、README、skills、模板内嵌代码、manifest、
  scripts 与 tests 中的路径和锚点；从引用文件所在位置检查，保留历史不可变 commit 链接。
- [ ] 按需检查本地链接、锚点、标题、围栏、JSON／YAML 示例和安全的 shell 语法，
  执行 `git diff --check`。只使用实际存在的仓库文档检查器，人工或临时检查如实标注。
- [ ] 涉及包或内嵌内容时，检查相关构建／打包结果及实际候选中的链接。相对链接目标
  必须入包，仅在源码仓库的材料使用仓库 URL；旧 tarball 不能验证新编辑的文档。
- [ ] 在 PR 记录实际通过／失败／跳过／未运行结果、方法、环境及适用的候选身份。
  不暗示尚未配置的 CI 检查或超出证据的验收结论，只执行与改动相关的门禁。

示例优先通过源码／帮助、语法或隔离夹具检查。不要仅为检查文档而调用真实模型、变更
账号、安装到真实用户 profile、发布或部署；这些动作须已在任务范围内并获得授权。

## AI 协作操作流程

1. 确认当前任务范围与已有授权。读取适用仓库约定，检查分支、HEAD 和未提交工作，保留
   无关改动；继续已有任务时复用正确的 PR／worktree。
2. 实现前阅读相关权威来源并识别文档影响，区分当前行为、提议变更与历史证据，指出尚未
   解决的设计／代码差异。
3. 在同一 PR 实现范围内的改动及对应双语文档，保留引擎状态所有权、冻结契约和用户定制。
4. 执行适用检查并处理具体失败，不把小幅文档变更扩展为无关运行时修改或真实账号实验。
5. 核对最终 diff、相关分支／基线变化、翻译、引用和验证证据；后续修改使结论失效时重新检查。
6. 完成已授权的提交、推送和 PR 工作并报告结果；同一动作沿用已有授权，不重复询问。
   仅要求提 PR 不等于授权合并、发布、部署或修改仓库可见性。

## 准备可审阅的 PR

说明具体问题、改动及行为、受影响的双语指南；有相关性但有意保持不变的文档，应在理由
不明显时简要解释。独立核对最终 diff 和证据，不以实现模型的“完成”声明代替 review，
遵守生效的合并要求和用户授权，保留必需门禁和仓库设置。

对每项适用检查分别记录**通过、失败、跳过或未运行**，包含实际命令／方法、相关环境，
以及跳过或无法运行的原因。本地结果、远端 CI 与真实验收分别说明。涉及打包／发行时，
按适用范围记录所测候选的 source commit／tree、snapshot hash、tarball integrity 及
干净／dirty 状态。不能把另一份源码或候选的结果直接当作 PR 最终状态的证据而不说明
限制。源码合并与 npm 发布仍是不同操作。

## 发行改动

遵循[发行指南](releasing.md)。Flywheel 只发布一个完整包，包含四个平台引擎和固定的 Codex Pro runtime；
private DSH 源码构建区不单独发布。从最终干净源码构建一次，测试同一批 tarball；保留 source commit、tree、
snapshot hash 和各包 integrity，release tag 必须指向该源码。不能把 dirty 候选改称正式
发行，也不能为已发布版本重新构建不同字节。

使用仓库已有脚本完成候选检查、上传、registry 验证和 promotion。只上传完整候选包；移动 `latest` 前安装验证精确 registry 版本，最后核对标签和 integrity。部分
发布失败后继续使用原候选。手动发布需要对应的明确授权，并准确记录已完成与不可用的检查。
