# 本会话多任务导航验证

[验证索引](README.md) · [流程控制台](../zh-CN/flow-console.md)

基于 `cbe84fc` 的 `codex/session-task-navigation` 工作树；本记录是源码和离线验证，未发布或安装新版本。

## 实现与自审

- 零／单任务不增加选择控件；多个任务显示当前 Session 的任务导航，以 taskDir/cwd 去重，同任务多次运行仍只有一个入口。
- 手动选择仅执行只读 status，切换到新身份前清除旧状态。普通索引刷新保留手动选择；新的明确关联继续自动跟随。
- 显示已读取标题；未读取时明确标任务 ID。其他任务状态标“最近记录”，不作为实时运行证明。长路径换行，列表高度有上限，当前项定位只滚动列表。
- 待确认的任务操作、配置保存及读取期间不能切换；控制器在事件处理时再次校验，防止快速连续点击绕过已渲染的 disabled 状态。原只读请求的 epoch／迟到响应处理保持不变。
- Host 允许当前会话投影中已确认的历史任务读取，解决准备任务没有 run 索引时无法回看的问题。仍校验真实 taskDir/cwd，其他会话、未归属运行、其他 cwd 和只读入口中的变更请求均拒绝。
- 设置草稿保护、Provider Session 隔离、默认模型、权限、调度及引擎状态写入契约均未改变。“记录”仍只显示当前选中任务的运行历史。

## 实际检查

- `npm --prefix plugins/dsh run check`：403 项主测试及 10 项旧宿主检查通过，无失败／跳过。
- 最后导航读取提示和滚动定位调整后，重新构建并运行任务发现、客户端及真实 Client/Host Gateway 定向测试：47 项通过。
- 四项 Go→Node 离线桥接（DSH、Codex Pro、Claude Code Pro、Kimi Native）：通过，33.541s。
- Chrome 渲染实际打包客户端生成的 DOM/CSS：1280px 下三项并排，320px 下单列且列表内部滚动，无整页水平溢出。导航高度分别为 112px／204px（后者含标签与有界列表）。使用模拟宿主主题与测量，不代表用户桌面安装验收。
- `git diff --check`：通过。中英文控制台指南、插件说明和设计契约已同步。

覆盖多次运行去重、其他 Session／无归属过滤、手动选择后刷新、新明确关联、准备历史、同目录不同 cwd 拒绝、读失败清除旧画布、快速点击和未知变更锁定，以及 Session 切换后的隔离。真实 Host Gateway 测试确认历史读取不新增聊天事件。

没有 Go 代码、schema、发行脚本或依赖变更，因此未重复 Go 全量覆盖率、发行候选及用户机器安装。此次未启动模型推理、未修改用户配置，也没有提交或推送。

## 2026-11-15 导航样式补充

用户在 0.34.1 的“流程”上看到竖窄背景。源码确认它是 Tab 的 hover 背景：48px 高度、零水平 padding、直角，与普通 outline 按钮 hover 规则叠加。节点子导航的保留样式具有同类几何问题；分段按钮也需要明确的高度与对齐，不能继承父布局拉伸。

流程／记录和节点子导航统一为 36px 圆角目标（触控 44px），选中下划线使用独立标记。分段切换桌面 32px／触控 44px，设置导航保持原本正确的 36px／44px；普通动作、节点卡片和任务导航保留各自合适尺寸。没有改变宿主或其他插件的样式。

新增可选真实浏览器脚本 `plugins/dsh/scripts/check-navigation-layout.mjs`。它需要外部 Playwright 模块与 Chromium；不会加入默认 CI 依赖或访问用户浏览器 profile。本机以 Playwright 1.65.1 和本机 Chrome 检查 320／800／1280px × mouse/touch 六种组合，实际执行 hover 和 focus，检查几何稳定、水平留白、圆角、选中标记、禁用行为和页面溢出。旧 CSS 明确失败于水平留白断言，新 CSS 六组通过；截图已人工查看。该脚本是 CSS 行为 fixture，不代替整套组件测试或安装后的桌面验收。

可重复命令：

```sh
FLYWHEEL_PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs \
FLYWHEEL_CHROME='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
node plugins/dsh/scripts/check-navigation-layout.mjs
```

本轮完整插件检查再次通过 403 项和旧宿主 10 项，先前四项离线桥接通过。0.34.2 的源码、标签、发行产物及本机安装另行记录；保留此前未发布工作树的历史验证范围。

## 后续发行

本记录之后，PR #49 已合并，0.34.2 已正式发布；发布身份与本机安装边界见[0.34.2 发行记录](2026-11-15-release-0.34.2.md)。此前源码／浏览器验证仍保持原范围。
