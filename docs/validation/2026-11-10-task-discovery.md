# 任务发现与单阶段画布验收

## 故障与修复范围

基线为 `5d682291e39c46c4e5e7c275f3d195ffce7e5d6c`，当前工作分支 `codex/jev-workflow-shadow`；本轮准备统一版本 0.30.1，含前一轮默认关闭的 JEV 影子评估，具体源码身份以 v0.30.1 标签为准；本记录不表示 npm 已发布或桌面已更新。

用户的 loveops 会话实际加载过两次 flywheel-task，但没有调用 flywheel_launch／flywheel_control；后续经 Bash 四次执行 verify、四次执行 review。Go 记录最后 review pass 0.92、整体 pending、revision 48、running=false。旧插件只从专用工具或 `/flywheel` 命令关联任务，因此视图为空。上轮通过 UI 填入已核实路径并只读刷新，确认关联由 null 变成 confirmed，画布恢复；没有启动用户任务或改写账本。

本轮覆盖：

- 默认任务选择与本机最近任务；彻底移除路径输入框；位置有真实值时自动显示为只读信息。
- CLI 索引新增 hostSessionId；来自公开 DSH_SESSION_ID，仅作发现信息，不授予权限，不传给 Provider 或验收子进程。
- 插件控制子进程使用自身 Session.id，清除错误的环境继承；独立目录读取不清除未确认操作或手动选择。
- 当前会话唯一任务自动只读打开；多个、历史无归属任务需明确选择；其它会话不能自动选入。
- 单阶段使用 run.kind 与 control.kind 匹配回执，Review 默认 L1 → Review；本次通过与整体交付分开，失败／取消／未知不能继承旧绿色裁决。
- Run、HEAD 和摘要收起，任务位置自动只读显示，显示评审记录评分；完整运行需用户展开选择。
- 单阶段技能执行后显式查询 status 关联；任务技能区分正文加载、准备和实际启动，文档任务不因载体而静默跳过已请求流程。

DSH 环境依据：本机 deepseek-harness `63ef376c8c486925140a98b466db6fcebac051de` 的 `packages/terminal/terminal-bash/src/index.ts` 在 childEnvironment 中设置 `DSH_SESSION_ID: spec.owner.id`；原生 Session.id 来自持久 header。不是解析 Bash 文本或扫描项目目录猜测任务。索引身份仍不是调用授权；原有租约、revision、结果未知阻断和 Go 状态所有权保持不变。

## 检查结果

- `make quality-gate` 通过：全部 Go 行为测试、vet、格式检查、逐函数 100% 覆盖率。日志 `/tmp/flywheel-no-input-quality.log`。
- 新增 Go 索引检查：有效／缺省／非法／过长宿主 ID、单阶段类型、终态回执；不向阶段和验收子进程传递宿主身份。
- 额外 CLI 集成检查通过：在临时 Git 项目真正执行 `run --only verify`，再从 `control runs` 读回 hostSessionId、kind、真实任务路径和完成时间；没有读取／修改用户的 SIP 项目。
- 插件全量 219 项测试、两版本原生宿主兼容检查 9 项通过；日志 `/tmp/flywheel-readonly-locations-final.log`。真实 renderer 测试保留旧绑定结果隔离、未确认操作与冻结目标验证，目录发现使用独立只读夹具并另外覆盖并发。
- 新增目录／画布测试覆盖没有路径输入框、未知位置不显示字段、准备中任务关联、同会话唯一候选、多候选与旧记录、跨会话晚到响应、手动选择、未确认操作、读取失败保留画布、Review 失败／取消及随后完整运行。索引先于状态发布／CAS 失败时，每次目录快照至多触发一次状态核对，不因 busy→idle 形成循环。
- DSH、Codex Pro、Claude Code Pro 三组 Go/Node 离线桥接通过；日志 `/tmp/flywheel-task-discovery-bridges.log`。Claude 0.4.0 仍是已核对的本地公开接口候选，不冒称新的 registry 发行。
- 根目录 `npm test` 141 项通过；日志 `/tmp/flywheel-readonly-locations-npm.log`。

## 可见界面验证

实际构建的客户端 bundle 使用 DSH 设计 tokens，在本机只读开发页显示真实评审的有界 Go 快照。预览阻止启动、停止、重跑和配置保存，不把快照展示当成正在运行的真实任务。

已验证默认任务卡片、自动选入、切换任务、当前运行／完整流程切换、任务位置自动出现且不可编辑；浅色、深色和 390px 窄屏无页面横向溢出，页面不存在路径输入框。评审默认选择 Review 节点，记录评分 0.92；技术元信息收起。截图：

- `/tmp/flywheel-task-ui-20261110/task-canvas.jpg`
- `/tmp/flywheel-task-ui-20261110/dark-390.jpg`
- `/tmp/flywheel-task-ui-20261110/empty-390.jpg`

这是客户端实际构建的浏览器验收，配合两版原生 renderer 离线检查；不是已安装 DSH 桌面新版的验收。本轮没有重启用户正在使用的 DSH。正式安装后的实机复查在批准发布／安装后进行。

## 发行准备

六包统一 0.30.1。开发候选从隔离快照打包；macOS／Linux 安装结果以 `/tmp/flywheel-task-install-macos-full.log`、`/tmp/flywheel-task-install-linux-full.log` 和候选 manifest 为证据。本记录不把候选验证当成 npm 已上传、latest 已提升或桌面已更新。正式发布须在批准提交／推送／标签后，从干净精确标签构建并重新验证正式同候选。
