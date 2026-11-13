# DSH 设置开关与最新工作台整合复核

## 范围与源码

用户授权复核并合并已有 PR，再检查最新 DSH 工作台并优化 JEV 与新增运行配置。
PR #38 已合并为 `ea277a488c3aa80a52282f8c5822aef0ed7e1246`；本轮将其与
PR #37 的 `8fb87e4b8ee82e4cd3d5bb8d84cbe1bcda4481ea` 整合为
`a7c54da8c72d34d16a41cacda2f819f05b7eefa7`，核对树
`4baa10fdae817ddb1631f379303475706ccd6b56` 后合并。
主分支合并提交为 `59bfb2dc5f8f6888ba2121967bd5ebccb4f88b74`。
后续 UI 改动位于 `feat/dsh-config-switches`，保留新工作台结构。

兼容整合保留会话归属、只读 Remote、独立设置页、固定导航与保存栏、角色会话策略、
revision 和未知结果保护；补回 JEV 实际采用、执行决策、模型历史与预热打包。
修正自动合并残留的已删除 `submit` 保存函数引用，并清除后置本地化中的旧影子说明。

## UI 行为

- `jev.enabled` 是唯一 JEV 总开关。`scheduler.prewarm` 与 `worktree.enabled`
  各有独立原生 checkbox `role=switch`，关联 label、可见开关状态、焦点样式、
  44px 点击区域和 pending 禁用；forced-colors 下保留原生 checkbox。
- dispatch 保持 batch／ready-queue 选择，串行时仍可查看与保留已选值；说明并行、
  并发数大于 1、自动 Worktree 管理等配置条件，并明确 Full 与独立任务依赖由运行时核对。
- prewarm 说明仅提前初始化支持的 DSH Review SDK，正式准入前不创建／恢复 Session、
  不发送模型请求；Worktree 名称明确为引擎自动管理，不将关闭误写为删除目录。
- JEV 文本和数字的无效输入保留为草稿，不覆盖最后合法值。关闭 JEV 仅放弃本组
  无效草稿，切为 serial 仅清除隐藏的并发数草稿；其他组错误保持。
- 合法连接、`allowed-data:[]`、Provider 并发额度、模型、Session 和预算不因无关
  开关重置。保留合法 `_note`／`_note_*`，endpoint 长度按 Unicode 码点计算。
- 高级 JSON 中检测出的非法字段保留原文与修正入口；未完成 JSON 时禁用结构化
  字段，导航、修正、撤销继续可用，避免关闭开关顺便覆盖整份 JSON 草稿。
  完整 schema、有效配置选择和原子保存继续由 Go 负责。
- 保存栏使用简短状态；JEV 对既有任务的关闭语义、冻结策略与普通设置的生效时机
  放在内容区。中英文配置指南、工作台指南、设计和 changelog 同步更新。

依据仓库钉住的 [DSH UI 规范](https://github.com/deepseek-ai/deepseek-harness/blob/b830ec950faba0a46dda564be3e2d854afed94f1/.agents/skills/dsh-client-ui-ux/SKILL.md)
复用颜色变量与既有布局，不引入宿主内部 UI 模块。新增样式没有使用超过 500 的字重。
触控导航按钮为 44px，窄导航仍为 54px，并以 5px 上下留白容纳按钮。

## 已执行检查

环境为 Linux amd64、Go 1.27.0、Node 24.22.1。

| 检查 | 结果与适用范围 |
| --- | --- |
| 整合后的 `make quality-gate` | 通过，每个被测 Go 函数 100%；后续 UI 改动没有修改 Go、schema 或嵌入技能 |
| 整合后的插件与 rc.5 宿主 | 287/287 与 10/10，通过，无跳过 |
| 根 `npm test` | 142/142，通过；后续 UI 阶段未修改 npm 运行／安装实现 |
| 三种 Go → Node public bridge，`-race` | DSH、Codex Pro、Claude Code Pro 均通过，没有后端跳过 |
| 最终 workbench 定向行为 | 34/34，通过，新增 8 个用例覆盖开关、草稿、缓存与配置保护 |
| 最终插件完整检查与 rc.5 宿主 | 295/295 与 10/10，通过，无失败或跳过 |
| 实际插件 DOM → 真实 Go 保存 | 1/1，通过；仅操作临时项目，global 为合成只读回执 |
| 六包隔离构建 | 通过；四平台二进制、插件与入口均为开发候选 0.31.1 |
| 同一候选 Linux amd64 隔离安装 | 10/10，通过，无跳过，包含可选 Claude 的真实安装路径 |
| 文档与差异 | 相对链接、锚点、代码围栏及 `git diff --check` 通过 |

DOM 保存检查用真实控件将无效概率 `1.5` 改为关闭 JEV，确认 Go 保存
`enabled:false`，保留原 endpoint、合法阈值 `0.6`、空数据授权。dispatch、prewarm、
Worktree 分别保存为 ready-queue、true、false，其他配置不变。随后用过期 revision
再次实写，Go 返回 exit 1、`ok:false` 和 `configuration changed`，原文件字节完全不变。
临时项目已删除，没有读取或写入真实 global 配置。

独立审查覆盖 Go 配置语义与 UI 实际 diff，没有剩余代码阻塞问题。
使用官方主题变量对开关滑块／轨道做静态颜色计算：浅色开启 4.235:1、关闭 5.799:1，
深色开启 8.118:1、关闭 9.252:1。这仅是颜色计算，不是浏览器渲染验收。

## 候选与证据身份

- 候选目录：`/workspace/scratch/fd8cf2c66d48/ui-candidate`。
- 源快照 SHA256：`665e37d7372a7ea2108446690dfc83599bc645249d58cde6e8b063f07f8bac6f`。
- 实际与包内 client bundle SHA256：`05a8909c583ee314479ce7615e6398e53e5f7882c7f1196fb2f91460019c40d9`。
- DOM 保存所用 Go 二进制 SHA256：`e773dbcb00893417de2e5f57e055a8423dd8f384814ee5d1395d5c9961e9e780`。
- Claude 0.4.0 使用 vendor 记录的精确 SHA256
  `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`；从已安装的
  已审阅包恢复后核对原哈希，没有采用缓存中另一份同版本但不同字节的包。

包内 client、`console-read.js` 和 `prewarm.js` 与最终本地产物逐字节相同。
候选在最后一处 test-only 选择器修正及本验收记录／索引写入之前生成；之后产品源码
与 bundle 未改变。它是 dirty 开发候选，不是 registry 已发行的 0.31.1，不可直接发布。

日志位于本轮工作目录：`ui-merge-quality.log`、`ui-merge-plugin-recheck.log`、
`ui-merge-npm-recheck.log`、`ui-merge-bridges.log`、`ui-final-plugin-verified.log`、
`ui-final-candidate.log`、`ui-final-install.log`，以及 `ui-browser/go-save-validation.log`。

## 保留的失败与未验收部分

1. 整合时 Gateway 测试写死 macOS `/private/tmp`，Linux 在到达会话隔离断言前失败。
   改用真实临时目录，保留隔离、未派发变更和不写会话日志的全部断言，最终整套通过。
2. 根 npm 首次与编译并发运行时，一个 500ms 子进程夹具未及时启动；未改断言或时限，
   定向和原完整命令复测均通过。初次日志 `ui-merge-npm.log` 保留。
3. UI 最终检查首次的 `client.test.js` 子进程退出为 SIGKILL；定位发现原测试的
   全局 `[role=status]` 会选中隐藏设置页的新状态提示。改为实际“操作反馈”region，
   保留单飞、旧响应失效和新目标结果断言。单文件 26/26 通过；最终全套结果见上表。
4. 整合 CI [43430598190](https://github.com/ckanner/flywheel/actions/runs/43430598190)
   没有执行测试步骤：npm-candidate 失败且无 steps，下游跳过。这不是 CI 通过，
   没有修改 workflow 或分支保护来放行。
5. Playwright Chromium 不在当前环境；系统 Chromium 的 ProcessSingleton socket
   被运行环境拒绝，连接浏览器也不可用，cloud browser 无法访问本地夹具并返回
   明确的 URL 安全策略拒绝。已停止尝试，没有绕过限制。实际浏览器尺寸、亮暗截图、
   原生 Space 键操作与用户 Mac DSH 安装验收均未完成。DOM 不冒充视觉验收。

本轮未调用真实模型，未验收真实延迟、资源、质量或成本收益；默认 dispatch=batch、
prewarm=false 与唯一默认关闭的 JEV 设置保持不变。未发布 npm、创建发行标签或修改
用户桌面安装。后续真实环境验收不能由本记录中的离线结果替代。
