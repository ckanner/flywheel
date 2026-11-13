# 工作台重设计实现与验收

## 范围

承接 [会话隔离与静默读取修复](2026-11-12-session-console-ux.md)，实现用户审阅的新版布局、
可选 Token 预算和分阶段 Session 策略。基线为
`e035b5b5736c2d26da7ada2bf1ca97ee020fd5a2`，开发分支 `codex/session-console-ux`。
本轮未提交、推送、合并、发布或修改用户桌面安装。

主界面保留流程与记录，设置独立进入；项目与执行目录在底栏直接展示。配置导航与
内容分配独立高度，保存区固定；后台状态读取保留画布，不再插入加载行使页面跳动。
节点结果、处理入口、本次配置和真实会话摘要直接显示；详细证据与受管会话各有入口。
PR 与 Merge 分开显示。画布仍以引擎真实编号表示未提供名称的任务，不使用设计稿中的虚构名称。

普通阶段与失败单元重试由显式点击提交一次 revision-bound 请求。重试理由只记录
“控制台手动请求”这一事实，不伪造用户对失败的分析；引擎仍限制为一次 Execute/L1。
重开已完成任务仍需填写原因并确认，保留状态版本漂移、依赖、L1/Review、预算和未知结果阻断。

模型与会话表直接编辑角色模式，Execute 路由共用角色策略，各 packet 历史独立。
显式 Plan 选择处理旧 resident-session 开关；已有冲突需要明确点击同步，不自动改文件。
已启用的第二意见拥有独立角色行。节点动作来自实际会话记录，而非 scoped 配置推断。
预算 off 隐藏门限但继续计量；soft/hard 仍按任务、UTC 日桶处理，不改变安装默认值。

## 已执行检查

| 检查 | 结果 |
| --- | --- |
| 插件 `npm run check` | 最终 264/264，通过，无跳过 |
| 固定 rc.5 宿主检查 | 10/10，通过，含公开 Gateway 静默读取与持久关联 |
| Go → Node 三后端离线桥接 | 通过，25.725s；未调用真实模型 |
| 根 `npm test` | 142/142，通过 |
| 六包隔离构建 | 通过，四平台二进制及插件／入口同版本 |
| 同候选 macOS ARM64 npm 安装 | 10/10，通过，无跳过 |
| 实际插件浏览器验证 | 亮／暗主题、390px/680px/桌面宽度；导航保持 54px，按钮 36px；390px 页面宽度无溢出 |
| 隔离配置保存 | 实际 Go `config-save` 保存 off；Plan fresh 同步 resident-session=false；随后恢复测试配置的 soft 与 Plan 继承设置 |
| 用户配置 | SHA256 仍为 `12acfcf0799b150d49f9ae10890dbab3aaca776a6be3363fad6614d61c7e77aa` |
| Go 全量门禁、Linux 安装、CI | 本轮未运行；没有改动 Go 源码；不沿用历史结果作为本轮通过 |
| 发布与用户 DSH 原生安装验收 | 未执行；浏览器为真实插件构建加合成运行回执，不等于桌面部署验收 |

## 保留的失败及修正

1. 旧 UI 用例依赖折叠、二次确认、固定按钮文案。按新操作路径更新，保留对单飞、未知结果、
   状态漂移、红色 L1、接管锁和精确保存字段的断言；新增预算关闭、角色覆盖、Plan 冲突与
   第二意见、未保存草稿、真实 Session 动作及后台刷新布局测试。
2. `post-dispatch failures` 的 Node 夹具两次未在 1 秒窗口内写出 start 日志，单独复测也失败；
   后续定向回归通过不能抹去这个时序问题。改用 shell 内建先记录派发并产生所需故障，
   timeout 分支用 `exec sleep`；保持 1 秒控制期限、实际派发日志和 unknown 阻断断言。
   修改后单项与最终全量检查通过。原失败日志为 `/tmp/flywheel-v2-sixth.log`、
   `/tmp/flywheel-v2-timing-rerun.log`；修复后单项为 `/tmp/flywheel-v2-timing-fixed.log`。
3. 安装检查曾因历史临时 Claude tarball 被移除失败。未改版本、未重打包；从 npm 缓存找回
   原字节，核对 SHA256 后继续。最终路径为 `/tmp/flywheel-v2-artifacts/claude-reviewed-0.4.0.tgz`，
   SHA256 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`。

## 最终候选与边界

候选：`/tmp/flywheel-v2-reviewed-candidate`；源快照 SHA256：
`b6d12f9bb9bd93b0925a2cc3b9bac8625cc04108dda216ce6d7e55aa9bc2de8d`。
沿用源码 0.31.1 版本号的 **dirty 开发候选，不可发布**，不是 registry 已发布的 0.31.1。
本验收记录与索引在测试后添加，不在该候选快照中。

最终日志为 `/tmp/flywheel-v2-delivery-plugin.log`、`/tmp/flywheel-v2-delivery-build.log`、
`/tmp/flywheel-v2-delivery-install.log`、`/tmp/flywheel-v2-bridge.log`、`/tmp/flywheel-v2-npm.log`。
截图保存在本次会话的 `flywheel-implementation-v2/` 可视化输出目录。

未伪造原生 Provider 跳转：进入会话复用已有受管历史、Prompt 与 CLI 入口。真实模型质量、
账号额度、Provider 完整原生会话界面，以及用户桌面的最终布局仍需后续安装后验收。
