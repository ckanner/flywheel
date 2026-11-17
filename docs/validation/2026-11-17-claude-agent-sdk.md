# Claude 官方 Agent SDK 集成与复审

日期：2026-11-17。本文记录 Flywheel 在 `214335e3f6eb66c9d5a88d4ed999c012329a29e2`
之上的 SDK 集成。原有 Kimi 与原生会话修复保留；本次只调整 Claude 配套插件、
精确依赖、真实协议测试、安装验证及相关配置说明，不替换 Codex 的调用实现。

## 根因与设计选择

Claude Pro 是 Node 嵌入式插件，普通委派适合使用官方 Agent SDK `query()`。
SDK 仍会运行 Claude Code；使用 SDK 不会自动证明进程清理、会话可恢复或认证授权。
Flywheel 因此继续只依赖配套插件的公开 machine/lifecycle/recovery 接口，由插件在
原有 guardian 内接入 SDK，并保留 Pro 层的可靠性契约。

[官方 SDK 文档](https://code.claude.com/docs/en/agent-sdk/overview)区分了 SDK 嵌入与
CLI 自动化；[公开 TypeScript 接口](https://code.claude.com/docs/en/agent-sdk/typescript)
提供 `pathToClaudeCodeExecutable`、`spawnClaudeCodeProcess` 和初始化接口。
本次明确选择用户配置的本地命令，缺失或不兼容时不静默换用 SDK 平台程序。

实际发布的 DSH `@deepseek-ai/dsh-subagent-claude-code@0.2.2-rc.3` 使用 SDK 0.3.263，
省略本地程序路径并关闭会话持久化。它印证了 SDK 嵌入路线，但不能直接代替 Pro
对活动、具名历史、独立子代理 effort 和 durable recovery 的处理。
精确版本及源码核对见配套插件的
[迁移验证](https://github.com/ckanner/dsh-subagent-claude-code-pro/blob/25754c3b41446c1f1225e7d7c6c12905aa67e33a/docs/sdk-migration-validation.md)。

## 配套产物身份

| 项目 | 实测身份 |
| --- | --- |
| 插件 | `dsh-subagent-claude-code-pro@0.6.0`，源码候选 |
| PR | [Claude PR #10](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/10)，基于其 PR #9 |
| Source commit | `25754c3b41446c1f1225e7d7c6c12905aa67e33a` |
| Source tree | `7ffd8deef680661c5b36890b459ce61ad468deea` |
| Archive SHA-256 | `0fdeb40a19411ffa8a5e467b934059f90e33a679a18c99c8c3ae8fc49a3366ac` |
| Agent SDK | 精确 `@anthropic-ai/claude-agent-sdk@0.3.326` |
| Build/check toolchain | Node 24.22.1、npm 11.9.0、Go 1.27.4 |

SHA-512、文件名与来源以 [vendor 记录](../../plugins/dsh/vendor/claude-code-pro.json)
为准。独立复核确认全部 62 个归档文件与已提交源码内容／执行位一致；实际安装的
62 个文件与归档一致，没有额外或缺失文件；SDK 的 32 个安装文件与官方原包一致。

Claude 继续是根包的精确 optional peer；根生产依赖及 bundled dependencies 不包含
Claude 或 SDK。只有插件测试环境安装该归档和 SDK。0.6.0 尚未被声明为 npm 正式发行，
Flywheel 发布前仍须通过现有 companion registry 验证。

## 行为及复审修复

普通调用使用 SDK 的语义参数，不从 argv 反向推测输入；`agents`、`title`、
`jsonSchema`、`forwardSubagentText` 通过真实 initialize 请求传递。原生 prompt/tools、
settings sources、权限、独立根／子 effort、图像/PDF、结构化输出、预算和后台工作保留。
Review 与交互 Team 仍使用其专用原生入口。

任务输入前检查同一实际原生进程的 SDK initialize，要求稳定版本至少 2.1.325；
该版本是最低已验证版本，不是所有相关功能的首次发布版本。旧版本没有收到任务
prompt，但回执仍保留真实 PID、unknown outcome 和独立 cleanup。SDK initialize
不等同于后续 `system/init` 配置证明，也不伪造 `thread-ready`。

独立复审发现合法 result 之后的损坏 control 尾部会让 SDK 抛错，初稿却仍保留
completed 和用量。SDK 还可能把原生失败结果之后的异常包装成 `error_result`。
配套插件现先校验 SDK 消费的原始消息字段，再检查完整 SDK/原生流结束证据。
损坏尾部使终态与用量保持 unknown，实际 cleanup 独立保留；合法原生失败 + exit 1
仍保留已确认失败与用量。Flywheel Node 与 Go 桥接都加入成功／失败后的损坏尾部
及正常失败对照，防止可信事实在跨层处理中再次混淆。

桌面配置中新选 Claude 资源使用 `claude-api`，缺失渠道默认映射到
`ANTHROPIC_API_KEY`；已有渠道、凭据映射和绑定不自动迁移。API 认证说明与模板、
UI 保持一致。本地命令与配置可用性不能证明所有订阅均获准用于第三方产品。

安装门禁使用实际 SDK 依赖图，逐包核对锁定的 SHA-512 与内部 name/version，
处理平台 optional 依赖和同名多版本；不使用 SDK shim。辅助脚本复用已有 pack
输出解析器，兼容 npm 11 数组与 npm 12 按包名索引的结构。

## 已完成的检查

| 检查 | 结果与边界 |
| --- | --- |
| 配套插件完整测试 | 205/205，无失败或跳过 |
| 配套实际 npm 安装门禁 | pack、隔离安装、public machine + 实际 SDK 通过 |
| 配套 DSH host schema/PTC | 8 种组合通过 |
| 配套远端 CI | Linux/macOS × Node 22/24 四组全部通过，含 host 与安装包检查 |
| 真实官方 CLI 2.1.325 / 2.1.326 | 各恰好 3 次 loopback 模型 fixture 请求；具名新建、同 UUID 续接、ephemeral、实际进程组退出通过；0 次付费模型请求 |
| Flywheel 根 npm 测试 | 166/166，无失败或跳过 |
| Go build / quality gate | 构建、vet、格式与全部包覆盖率通过；总计及每个函数均为 100% |
| Flywheel 完整插件检查 | 543/543，通过；另有 10/10 legacy host 检查通过，无跳过 |
| Flywheel 最终 Claude Go↔Node 桥接 | 两组及其子项共 31 项通过，实际安装候选 + SDK + 原生协议 fixture |
| 真实引擎配对场景 | 36/36，96 次离线调用；六类场景全部观察到，unknown/unobserved 为 0 |
| 配置编辑器 | 隔离构建的 24 项测试通过，涵盖新渠道与原渠道保留 |
| 产物及代码复审 | 源码／归档／安装内容一致；SDK、映射、运行时、doctor、安装 helper 及 UI 均经独立复审 |

单包安装验收的候选身份与最终结果，在本次 PR 的验证评论及其 CI 中追加。
安装测试应使用上述同一 Claude 归档和一次构建的
四平台 Flywheel 候选；不得将 optional peer 未安装时的跳过算作 Claude 通过。

首次全插件检查曾在同时进行依赖下载和其他回归的负载下触发既有 Codex display
omission 时序断言；单项原样重跑和上述最终完整检查均通过，未修改 Codex 生产
代码或放宽该断言。

## 验证边界

原生 CLI 验证使用隔离 HOME/配置/工作区、合成 API key 与本地 HTTP fixture。
它证明真实 JSONL 持久化、标题、续接与清理，不代表付费模型质量、真实账号授权或
macOS Desktop UI 验收。SDK 本身不会改变官方 picker 对 print/SDK 历史的过滤；
按根 UUID 使用支持的入口续接，见[原生会话验证](2026-11-17-native-session-visibility.md)。

此轮未更改用户登录态、安装配置、默认任务绑定；未合并 PR、发布 npm 或创建 tag。
未完成的旧调用仍应使用原版本的 adapter/recovery 身份核对，不能靠升级或新建
会话绕过 unknown。
