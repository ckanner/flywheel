# 流程工作台界面改造验收

## 范围与状态

按用户已审阅的画布优先设计实现实际 DSH 插件。改动覆盖原生会话视图、画布与节点详情、配置编辑、运行记录和中英文指南；未修改 Go 状态机或 schema，未提交、推送、发布或替换真实桌面安装。

源码基线为 `cd4fdd8a15dccaf4b3da76cc7d6d8b4f98390d1e`。六包仍使用工作区版本 `0.30.1` 进行隔离安装验证，候选明确标记 `source.dirty=true`，不能作为正式 npm 发行。已发布 0.30.1 的原始产物没有重建替换。

## 实现

- 宽画布、平移/缩放/适合宽度、当前步骤跨分页定位、任务子图、列表切换；窄窗口默认列表，用户显式选择优先。
- 插件拥有的原生专注画布对话框，保留任务与分支，Escape 返回且不启停任务；未知操作退出专注模式以显示核对入口。
- 节点详情分进展、会话、本次配置。运行确认冻结 revision；重试保留失败记录并要求原因；依赖未交付的任务与红色 L1 的 Review 不可直接运行。
- Quick 验证同时读取 L1、终验与已记录的缺陷注入结果，不把 L1 单独通过显示为整体验证通过。单阶段结果不冒充整体交付。
- PR 未查询、结果未知、已关闭和已合并分别显示；新的引擎 revision 使旧远端快照过期。
- 配置自动读取一次，分组编辑、相邻保存、中文阶段/推理文案、分钟/秒精确换算。串行隐藏并发控件；Provider 名称不冒充某个默认模型。
- 范围切换/重读/重置保护草稿；当前 Session 内的任务切换保留草稿，恢复时重新读取文件，revision 不同则阻止保存。真实来源是文件级选择，不伪称逐字段继承。
- 运行记录列表与详情分开；对照对象不改变指标查询目标，指标范围明确为本次运行或任务全部调用；整组费用缺失显示未报告。
- 受管会话与 PR 写入沿用 Go 接口；终端续接命令优先使用原会话工作目录。JEV 保持默认关闭及影子建议语义。

## 检查结果

| 检查 | 结果 | 证据 |
|---|---|---|
| 插件构建与全量测试 | 234/234，零跳过 | `/tmp/flywheel-workbench-accepted-check.log` |
| 旧 DSH 宿主兼容测试 | 9/9，零跳过 | 同上；真实发布的 renderer/slots 接口 |
| 根目录 npm 测试 | 141/141，零跳过 | `/tmp/flywheel-workbench-npm-final.log` |
| DSH / Codex Pro / Claude Code Pro 的 Go → Node 桥接 | 三组通过，显式 `FLYWHEEL_TEST_NODE_BRIDGE=1` | `/tmp/flywheel-workbench-bridges.log` |
| 同候选 macOS 安装 | 10/10，零跳过 | `/tmp/flywheel-workbench-final-install-macos.log` |
| 同候选 Linux 容器安装 | 10/10，零跳过，非 root 用户 | `/tmp/flywheel-workbench-final-install-linux.log` |
| CSS Modules 引用与格式 | 无缺失 class；`git diff --check` 通过 | 当前源码构建检查 |

可选 Claude 0.4.0 后端安装测试使用此前审阅的真实源码 tarball `/tmp/flywheel-claude.WYp9T7/dsh-subagent-claude-code-pro-0.4.0.tgz`，没有将 optional peer 缺席的跳过当作通过。

## 候选身份

唯一验收候选目录：`/tmp/flywheel-workbench-final-validated`。

- 源码快照 SHA-256：`4affda190d9c74f62861af23592b496c1f2f94c198b4c5a871024db718344e39`
- 候选 manifest SHA-256：`2cc79687c4c52e6fac47134c3410cfcf4367c69760d2d813de1ef08926291004`
- `package/dist/client.js` SHA-256：`285ebd4dd77bbccd7019e21e264857bc8d6a5546e7443eae833a5e00a6e88161`
- 归档内 client 与本地实际构建逐字节一致。两个平台消费同一组六个 tarball，没有重新打包。

本验收记录在候选完成后补写；实现代码与已安装验证的候选一致。正式发行仍需新版本、批准的提交与标签，以及干净源码构建。

## 可视与真实边界

浏览器加载实际 `plugins/dsh/dist/client.js` 与 DSH 主题，在浅色/深色、桌面与 390px 窄窗口检查画布、子图、详情、专注模式、弹窗 Escape 和固定动作区、配置与模型编辑。合成流程回执用于界面演练；没有把它们称作真实 SIP 任务执行或模型推理。

配置读写通过当前源码构建的 Go 二进制，`FLYWHEEL_CONFIG` 指向 `/tmp/flywheel-workbench-preview/global-config.json`。将串行切换为并行后，Go 保存成功并生成 `global-config.json.backup-208416315`；实际用户配置没有作为写入目标。

截图位于本机设计目录 `~/.codex/visualizations/2026/11/04/01a1a75a-cf63-7e31-a843-85a40a52a620/flywheel-redesign/`：`workbench-config-implemented.jpg`、`workbench-canvas-implemented.jpg`、`workbench-narrow-dialog.jpg`。

未完成的发行边界：正式 npm 发布、真实 DSH 桌面安装/重载和真实 Provider 推理验证。本轮不据此宣称线上发行或桌面运行验收。
