# Changelog

## 0.22.0 (2026-09-12)

三厂商 harness 定稿 + 完全自包含分发（安装用法人人一致，无需源码仓库）。

- **Harness 矩阵（DESIGN v0.22）**：Execute default/multimodal/Intake = zcode（app-server 协议，按会话指定模型与思考档）；Review + Judge 复核面板 = Codex（GPT-6-Astra）；Plan/frontend/hard/Judge 主判/Garden = kimi code + K3。跨厂商制衡升级（GLM 执行 ↔ OpenAI 评审）。
- **自包含分发**：契约 schema、skills 快照、§13 法定配置表全部内嵌二进制；`flywheel install-skills` 默认从内嵌快照物化（`--link` 为开发机源码树软链专用）；`flywheel template` 打印内嵌模板。
- **配置四级发现链**：任务目录 → 项目根 → 全局 `~/.config/flywheel/flywheel.config.json`（首装自动物化，可直接编辑）→ 内嵌法定表兜底。
- watchdog 交互钩子（协议驱动）、multimodal 执行路由、`FLYWHEEL_CONFIG` 覆盖。

更早历史见 docs/design.md 变更记录。
