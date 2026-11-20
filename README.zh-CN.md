# Flywheel（飞轮）

[English](README.md) · 简体中文

**把明确的编码任务变为经过验证的软件变更。**

Flywheel 是面向 coding agent 的确定性 Go 编排器。模型负责规划、实现和评审；引擎负责任务契约、验收、重试、预算、工作区和交付。既可从 DeepSeek Harness（DSH）桌面使用，也可通过命令行运行。

[快速开始](docs/zh-CN/getting-started.md) · [使用手册](docs/zh-CN/usage-handbook.md) · [文档](docs/README.zh-CN.md) · [参与贡献](docs/zh-CN/contributing.md) · [MIT 许可证](LICENSE)

从 0.33 起，`@ckanner/flywheel` 一个包包含 CLI、DSH 界面／worker、内嵌技能和
四个平台引擎，不再需要独立平台包或首次运行下载引擎。从 0.32.x 升级前先退出 DSH，
再运行原来的安装命令；安装器替换旧桌面包登记，保留配置和任务历史，完成后重载 DSH。
历史 npm 版本继续保留。

## Flywheel 能做什么

- **根据任务选择流程。** Auto 为满足条件、边界明确的任务选择 Quick，需要规划或更广泛验收时选择 Full。
- **验证实际候选代码。** 确定性检查先于独立评审，完成证据必须对应最终提交和集成的同一份代码。
- **隔离不同角色。** 规划、执行和评审使用独立上下文；满足条件的返修可续用同角色会话，保留任务历史。
- **协调有界并行执行。** 显式声明依赖且写入范围不重叠的 packet 可以并发执行，评审和集成仍由引擎统一协调。
- **提供桌面工作台。** 在 DSH 原生 Flywheel 视图中查看任务进度、证据、会话、运行记录和配置。
- **明确呈现失败。** 未知执行结果、用量或清理状态保持未知；无法安全继续时保留成果并停止。

原生能力可以按阶段配置：DSH PTC／Team、Kimi Swarm／Tower、Codex agents、Claude Workflow 和受管 LCU。入口、依赖和实际运行边界见[原生能力指南](docs/zh-CN/native-capabilities.md)。

## 安装

使用桌面入口前，请准备 **Node.js 22.18.0 或更高版本**、npm、Git，以及带可用 `dsh` 命令的受支持 DSH 桌面。插件采用 **DSH 0.2～0.3 兼容系列**范围，从 **0.2.1-rc.5** 起，到 **0.4.0 的任何预发布版之前**；范围内常规升级无需重新发布 Flywheel。这是尽力兼容窗口：如果某个版本确实存在不兼容变更，必需的宿主服务和 SDK 检查仍会失败关闭，再通过代码修复，不伪造宿主版本放行。还需准备所选模型路由使用的账号和 CLI 工具。DSH 自身可能要求更高版本的 Node；DSH 0.2.2 SDK 要求 Node 24.9 或更高。

```sh
npx -y @ckanner/flywheel@latest install
```

安装器配置同版本 CLI、原生二进制、worker、内嵌技能和 DSH 插件。原生包覆盖 macOS／Linux 的 arm64 和 x64，通过 npm 安装不需要 Go。账号登录与 provider 配置是独立步骤；已有配置和用户修改过的技能会保留。

需要安装到用户可写的 npm prefix 时：

```sh
npx -y @ckanner/flywheel@latest install --prefix "$HOME/.local"
```

将该 prefix 的 `bin` 加入 shell PATH 后即可直接使用 `flywheel`。[安装指南](docs/zh-CN/getting-started.md)包含桌面注册、SDK provider、诊断、升级和首个任务的完整步骤。

本 README 描述当前源码。通过 `npm view @ckanner/flywheel version` 查看实际发布版本；源码合并不会更新已有安装。版本变化集中记录在 [CHANGELOG](CHANGELOG.md)。

## 运行第一个任务

在 DSH 中打开目标仓库，描述一个范围明确的变更：

```text
/flywheel-task 修复列表空状态，补齐回归测试并完成变更。mode=auto
```

技能先研究仓库，在 `docs/<task>/` 准备任务契约，再由 Go 校验并接管执行。只准备任务而不启动时使用 `/flywheel-new`；启动或恢复已有任务使用 `/flywheel-run`。在 Kimi 中，对应命令形式为 `/skill:flywheel-task`。

打开原生会话中的 **Flywheel** 视图，即可查看关联任务的画布、运行记录和配置。任务位置来自会话与运行索引，无需手工输入路径。

已有任务可以通过 CLI 启动和查看：

```sh
flywheel control launch --cwd /path/to/repo --task-dir /path/to/repo/docs/my-task --mode auto
flywheel control status --cwd /path/to/repo --task-dir /path/to/repo/docs/my-task
```

在目标仓库中运行以下只读配置检查：

```sh
flywheel config inspect --cwd "$PWD" --json
flywheel doctor --cwd "$PWD" --json
```

Doctor 检查本地接入条件，不代表真实模型调用已经可用。单阶段操作、人工接管、运行对比和明确授权的 GitHub 交付入口见[工作台指南](docs/zh-CN/flow-console.md)。

## 选择工作流

| 模式 | 适用场景 | 执行方式 |
| --- | --- | --- |
| **Auto** | 已准备好范围和验收条件的任务 | Go 根据契约判断，满足 Quick 条件时选择 Quick，否则使用 Full。 |
| **Quick** | 一个里程碑、一个有明确边界且验收充分的 packet | Execute → 必需检查 → 独立最终 Review → 本地交付；发现新风险可升级为 Full。 |
| **Full** | 多个 packet、设计工作或影响较广的变更 | Plan → packet 执行、检查、评审与本地交付 → 集成验证和 Judge。 |

Quick 保留仓库必需检查和交付检查。首次成功路径有两次阶段 agent 调用，不含宿主准备、重试和 agent 内部工具调用。没有工作流元数据的旧任务继续使用 Full。详见[工作流与交付](docs/zh-CN/workflow.md)。

根因尚不明确时，Task 可以先执行[有界调查](docs/zh-CN/investigation.md)。只调查的目标以证据报告结束；已授权的修复保留原范围，再进入 Auto。New → 审阅 → Run 和单阶段直接入口继续保留。

## 模型与配置

内嵌模板提供以下可编辑绑定。这些是初始配置，不是模型排名，也不承诺具体账号可用。

| 阶段或路由 | 后端 | 模型 | 推理等级 |
| --- | --- | --- | --- |
| Plan、前端执行、Garden | Kimi CLI | `kimi-code/k3.1-pro` | `high` |
| 默认执行 | DSH / `deepseek-account` | `deepseek-flash` | `high` |
| 高难度执行 | Codex Pro 公开 machine API | `gpt-6.1-sol` | `high` |
| Review | DSH / `chatgpt` | `gpt-6.1-sol` | `high` |
| Judge | DSH / `chatgpt` | `gpt-6-astra` | `high` |
| Intake | DSH / `deepseek-account` | `deepseek-flash` | `low` |

[Claude Code Pro](docs/zh-CN/claude-code-pro.md) 是可选后端，目前没有默认分配阶段。Profiles 支持在各阶段和路由间复用或覆盖绑定。[配置指南](docs/zh-CN/configuration.md)说明文件优先级、provider、权限、预算和迁移；[模型策略](docs/zh-CN/model-policy.md)说明推理等级的选择依据与限制。

新的内嵌默认配置选择 scoped 会话、full-access 权限、自动管理 worktree，以及最大并发数为 2 的并行调度。并行资格仍要求显式声明依赖和写入范围。已有完整配置缺少字段时保留兼容默认值，升级不会自动改写配置。

## 工作区与恢复

**项目工作区**是会话所属的仓库，**执行目录**是任务或 packet 修改、验证代码的位置，两者可以不同。自动管理 worktree 与调度方式独立；关闭自动管理不会自动关闭并行。共享目录批次必须基于最终共同候选进行验收。

会话历史复用和调用恢复是不同机制。派发、结果或清理未知时，即使请求新建会话也不能继续执行。各后端能力不同，具体边界见[中断恢复与上下文质量](docs/zh-CN/session-recovery.md)。离线评估可以验证协议行为，不能证明真实模型质量、缓存节省或性能收益。

## 文档与开发

[文档索引](docs/README.zh-CN.md)提供中英文指南、架构规范和历史记录入口。

- [配置参考](docs/zh-CN/configuration.md)
- [DSH 插件](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/README.zh-CN.md)与 [worker 协议](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/PROTOCOL.md)
- [架构规范](docs/design.md)——以中文维护
- [贡献指南](docs/zh-CN/contributing.md)与[发行手册](docs/zh-CN/releasing.md)

开发需要 **Go 1.27 或更高版本**以及贡献指南指定的 Node/npm 工具链。Go 质量门禁包含格式、vet，以及每个被测函数 100% statement coverage；包与适配器变更另有构建、跨进程和安装检查。

## 许可证

Flywheel 使用 [MIT License](LICENSE)。第三方代码保留各自声明，详见 [DSH 插件第三方声明](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/THIRD_PARTY_NOTICES.md)和[内置适配器来源](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)。
