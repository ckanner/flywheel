# Changelog

## Unreleased

DSH 桌面集成与可靠交付闭环。此节记录源码变更，不代表已发布 npm 包或 Mac 公证安装包。安装与迁移见 [入门教程](docs/getting-started.md)。

- **配置与模型 profiles**：新增可复用 `profiles`、阶段引用与显式覆盖；DSH `provider` 和账号 `channel` 分离，所有角色和渠道降级均传递实际 provider；指标分别保留两者。严格拒绝未知字段、无效引用和 provider 冲突，兼容旧内联绑定与合法显式零值。
- **只读预检**：新增 `config inspect`、`doctor` 和相应 control actions；检查有效配置、工具和任务所需路由。可选 SDK probe 验证配置、目录、沙箱与关闭，零推理，整体验证有界；账号、额度与实际模型能力明确标为未验证。
- **桌面可观察性**：面板展示配置、逐项诊断、完整 packet 状态与未知计量；区分完成、暂停、停止和升级，路径切换后忽略迟到的旧响应；增加宿主 React 18 对应的真实 DOM 交互回归。
- **DSH 入口与 worker**：新增 `control start/status/stop`、桌面面板、人工命令与模型工具。Go 继续独占状态；worker 校验 provider/model/cwd/effort、权限、可靠终态及 SDK 关闭。
- **模型路由**：默认 DeepSeek Flash 实现，Kimi CLI 规划／前端／整理，DSH ChatGPT provider 评审／终验，独立 Codex CLI 处理困难任务。Kimi 不使用 ACP；外部 Codex Pro adapter 保留独立交接任务。
- **可靠性**：验证证据绑定候选和契约；契约修订跨重启失效旧证据；L1、Review、里程碑 gate 与 Judge 前后检查身份；提交及合并后确认持久化结果，失败保留 worktree。
- **发布前复核**：修复从子目录漏读项目配置；持久化检查独立核对执行位与链接类型；验收输出截断不能因退出码符合预期而通过。CI 增加 Linux／macOS 插件与跨进程检查，构建产物只作为 Actions artifact 分发。
- **Mac CI 修复**：任务目录和 Git 执行根统一使用物理路径，避免 `/var`／`/private/var` 别名造成账本、报告和恢复判断失真；补充真实符号链接启动／恢复回归，并消除 Kimi 路径断言与 Zcode fixture 的写入／退出竞态。进程组清理探测的暂态 `EPERM` 在原宽限内重试，仍须确认 `ESRCH`；持续权限错误继续阻止交付。
- **生命周期与计量**：按运行 token 停止、运行 lease 防重入、超时／取消／清理失败严格区分；未知 usage/cost 不再当成零。
- **测试与文档**：普通单测／回归可随实现修改，冻结验收显式保护；保留逐函数 100% Go statement coverage；更新全部内嵌 skills 的控制接口、单环语义与配置说明，补充完整 Mac 安装和首个任务教程。
- **兼容边界**：旧配置不自动迁移；任务／项目／全局配置选择首个完整文件，不深度合并。Mac 桌面渲染、Seatbelt 和真实账号调用仍需用户环境验收；离线通过不等于实机通过。

## 0.22.1 (2026-09-14)

修复 0.22.0 meta 包缺 bin shim：`bin/flywheel.js` 此前被 .gitignore 的 `/bin/`（postinstall 下载目录时代的遗留规则）挡在 git 之外——CI checkout 无此文件，发布的 meta 包只有 3 个文件、`flywheel` 命令不存在。本版将 shim 入库并全量 bump 版本；旧 0.22.0 meta 请勿使用。

## 0.22.0 (2026-09-12)

三厂商 harness 定稿 + 完全自包含分发（安装用法人人一致，无需源码仓库）。

- **Harness 矩阵（DESIGN v0.22）**：Execute default/multimodal/Intake = zcode（app-server 协议，按会话指定模型与思考档）；Review + Judge 复核面板 = Codex（GPT-6-Astra）；Plan/frontend/hard/Judge 主判/Garden = kimi code + K3。跨厂商制衡升级（GLM 执行 ↔ OpenAI 评审）。
- **自包含分发**：契约 schema、skills 快照、§13 法定配置表全部内嵌二进制；`flywheel install-skills` 默认从内嵌快照物化（`--link` 为开发机源码树软链专用）；`flywheel template` 打印内嵌模板。
- **配置四级发现链**：任务目录 → 项目根 → 全局 `~/.config/flywheel/flywheel.config.json`（首装自动物化，可直接编辑）→ 内嵌法定表兜底。
- watchdog 交互钩子（协议驱动）、multimodal 执行路由、`FLYWHEEL_CONFIG` 覆盖。

更早历史见 [历史设计](docs/design-history-v0.24.md) 的变更记录。
