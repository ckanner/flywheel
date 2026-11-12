# 为 Flywheel 贡献

[English](../en/contributing.md) · 简体中文 · [文档索引](../README.md)

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
| npm 安装或发行 | 根 npm 测试、六包候选构建、安装同一批 tarball |
| 文档 | 链接、锚点、代码围栏、可运行示例、翻译一致性，以及受影响的内嵌／包内容 |

Claude 桥接要求 worker 的依赖树中存在精确 0.4.0 optional machine API；因未安装 peer
而跳过不能算该后端通过。使用源码候选时保留已审查 source／tarball 身份。Codex Pro 的
0.9.0 bundled 源码包同样由实际 integrity 与 [vendor 记录](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)
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

项目默认英文入口是 [README.md](../../README.md)，对应中文为
[README.zh-CN.md](../../README.zh-CN.md)。现行用户与贡献指南成对放在 `docs/en/`
和 `docs/zh-CN/`，使用相同文件名，并提供往返语言链接。

| 内容 | 归属位置 |
|---|---|
| 项目介绍与首个操作命令 | 根目录 README |
| 现行安装、工作流、配置与维护方式 | `docs/en/` 与 `docs/zh-CN/` |
| 权威架构与不变量 | `docs/design.md` |
| 设计提案及当时背景 | `docs/plans/` |
| 已执行检查、环境、证据与限制 | `docs/validation/` |
| 历史 review 与已被替代的材料 | `docs/reviews/` 与 `docs/archive/` |
| 包专属接口与来源记录 | `plugins/dsh/` |

直接更新负责该行为的现有章节；需要新增时创建明确命名的小节并同步导航。不要把开发更新
前插到介绍之前，也不要在许可证之后追加无关操作说明。版本变化写入 changelog，带日期的
验证证据放入 validation 记录。

安装、配置、工作流、命令或运行边界改变时同步中英文。模型 ID、JSON 字段、权限含义、示例
和能力限制必须等价。历史设计、提案与验收记录无需全部翻译，索引和链接应明确其语言。

修改 npm 文档时同时检查源码树与实际包。包内 README 引用的文件必须纳入包，或明确链接到
仓库；旧 tarball 不会因为源码链接更新就自动包含新文档。保留实际被审查候选的证据。

## 准备可审阅的 PR

说明具体问题、改动及其行为、实际运行的检查和仍未验证的范围。独立核对最终 diff 与证据，
不以实现模型的“完成”声明代替 review。遵守生效的合并要求和用户授权，保留必需门禁与仓库
设置。源码合并与 npm 发布是两项操作。

## 发行改动

遵循[发行指南](releasing.md)。六个 Flywheel 包保持同一精确版本、精确内部依赖和固定
Codex Pro runtime。从最终干净源码构建一次，测试同一批 tarball；保留 source commit、tree、
snapshot hash 和各包 integrity，release tag 必须指向该源码。不能把 dirty 候选改称正式
发行，也不能为已发布版本重新构建不同字节。

使用仓库已有脚本完成候选检查、上传、registry 验证和 promotion。先发布平台包与插件，再发
meta 包；移动 `latest` 前安装验证精确 registry 版本，最后核对六包标签和 integrity。部分
发布失败后继续使用原候选。手动发布需要对应的明确授权，并准确记录已完成与不可用的检查。
