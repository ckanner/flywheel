# 原生依赖正式包对齐

[验证索引](README.md) · [原生能力](../zh-CN/native-capabilities.md)

基于 PR #44 的 `eebba77a918c4be0f5d2a02b62ab88a3fba6e0c1`，独立核对 registry、源码树、tarball 内容与本机 Node 解析树。

Codex Pro 0.10.0 和 LCU 0.5.0 的正式 tarball 与原候选逐字节一致。Claude 0.5.0 文件内容相同，但归档权限不同；已采用正式 tarball、实际 integrity 和 SHA-256，并同步 CI／publish，取消从源码重新打包后与 registry 混用。ChatGPT 正式 0.1.20 与旧 0.1.19 源码候选仅 package.json 的版本不同，测试包和中英文指南已同步。旧候选身份保留在历史验证记录，不能重新解释成正式发行。

Generator 0.4.0 的源码包标记 private，继续按已审查源码 bundle，不声称 npm 已发布。四个公开包的记录保留真实 registry URL、摘要和核对时间，验证工具链不冒充发行方构建工具链。

本轮检查：根 npm 152 通过；插件 395 及旧宿主 10 通过；更新后的单包开发候选 Host／Client 10、实际安装 15 通过，无跳过。正式 Codex／Claude 的 machine 入口在不安装 Schemastery、home-paths、Cordis、React 的隔离 npm consumer 中可导入；Claude 安装后模块权限为 0644。未发模型请求或启动 GUI。

本机核对发现 desktop 已更新，而全局 Flywheel 默认 worker 仍用 Codex 0.9／Claude 0.4、缺少 LCU；独立 sdk profile 的 ChatGPT 仍为 0.1.17。指南明确区分三处依赖树。实际用户升级与正式发布在源码门禁通过后单独记录；本记录不表示已修改用户安装或原生功能开关。

开发候选（本记录写入前构建，非正式发行）：source snapshot `1d97a3c1bbf2880f982c2a13d581cad13829dd1bede85ac03f74430b6355f41a`；包 integrity `sha512-fM3ggV1LSzhhU+dWTdZm6SITGkAYOp6nDIrkYbQ8k6mg/bCuI1uj+jYlZEM61v3bPO/+EkbiMUlXrk0+V5qWBA==`。最终提交的双平台 CI 与正式标签候选另行核对。
