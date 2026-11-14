# 0.33.0 单包分发工程验证

[验证索引](README.md) · [发行指南](../zh-CN/releasing.md)

## 范围与身份

基于 `63d8781ee1ece0cce443fa2e1ead62d1d6288817`，将 Flywheel 自身的六个发行包
合并为一个 `@ckanner/flywheel@0.33.0`。`plugins/dsh` 保留为 private 源码测试区；
CLI、Host、Client、worker 和四个平台引擎装在同一个 tarball，skills 和模板继续内嵌。
不改 Go 状态机、模型默认值、权限或用户账号，不修改历史 npm 版本及其标签。

开发候选：candidate format 2，dirty=true，snapshot
`e2dfdcf243c0c77fdafc96f9c077e9059c2233d30e6eff3527434eecbd8d963a`，
唯一 tarball `ckanner-flywheel-0.33.0.tgz` 为 18,621,751 bytes，integrity：

```
sha512-NJzZPwy6qBJaVBLwsw0xlfcKDkWMYmVkC7pQkVjd1h0jwmDZkqQP9UD9exo3PHaCu8fL4knRw9bYiVtwXEtnIA==
```

该候选仅用于工程检查；后续文档与 changelog 整理不回写此身份。正式发行必须从
干净标签另建一次正式候选，双 OS 验证并上传原始 tarball。本记录不声明已发布。

## 检查

工具链 Node 24.22.1 / npm 12.2.1 / Go 1.27.3。Claude 使用已审查的真实 0.4.0
源码包，SHA-256 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`；
Codex 0.9.0 维持 vendor 锁定及 bundle，未假设它们已在 registry 发布。

- macOS 根 npm 单元及协议检查：151 通过，无跳过。
- macOS 插件 304 通过，旧宿主兼容检查 10 通过，无跳过。
- macOS 同候选：公开 Host／Client 10 通过，真实 npm 安装 13 通过，无跳过。
  覆盖完整包入口、四引擎完整性、DSH SDK、Codex／Claude Machine、旧包迁移和重复安装。
- Linux 非 root 容器：插件 304、旧宿主 10、根 npm 151、打包 Host／Client 10、
  同候选安装 13 全部通过，无跳过；三项 Go → Node 桥接通过。
- macOS 三项 Go → Node 离线桥接通过；Go 源码无改动，正式 CI 仍跑全量质量门禁。
- 真实 DSH CLI 在隔离 HOME／DSH_HOME 完成旧 0.32.1 插件 → 完整包 → 移除旧名，
  profile 最终只登记新包。该早期开发候选检查不代替最终正式候选和用户桌面验收。
- 已改动且随包分发的 Markdown 相对链接检查通过；Git diff 空白检查通过。

## 自审

- 启动器只使用本包引擎和 worker，缺失、版本或摘要不匹配明确失败。
- DSH 的模块 ID、Remote descriptor、bundle 与安装名一致；实际 tarball 的 `.mjs`
  与客户端 bundle 经过真实公开 Host API 测试，而非只测源码区的 `.js`。
- 迁移先安装并确认替代包，再移除旧名；损坏列表、定制来源、取消、部分失败均不报成功。
  回退只移除已确认由本次新增且身份未变的替代包；预存或并发替换的包保持不动。
- 显式 flywheelBinary 保持优先。完整包无受管记录时用自带 CLI；旧版受管记录必须
  重新 setup，不混用新版界面和旧引擎。现有配置、skills 修改和任务数据保持原归属规则。
- 正式 publish 使用 OIDC、同候选和 provenance；精确 registry 安装成功后才推进 latest。
  保存 Trusted Publisher 设置不等于已完成首次自动发布，须以实际 CI 回执为准。

## 初始失败及限制

早期开发全量 npm 检查出现一次原有 registry 超时 fixture 的 pid marker 缺失；
未改该用例、deadline 或断言，单独复查与后续全量 151 条均通过。此处保留初次
失败，不据重跑断言其时序根因已消除。开发安装用例曾使用旧 `.js` 名及 macOS
`/var` 别名断言，已分别更新为打包入口和物理路径后通过。

尚未完成的交付项：精确提交 CI、正式标签候选、npm OIDC 发布／latest、用户 profile
迁移与 DSH 重载后界面验收。离线协议检查不代表真实模型推理、计费或质量结论。
