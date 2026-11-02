# npm 一条命令安装与交付计划

基线：`39638bd96e6fcb6cc681bc7c1f90b7b082620936`。用户要求 npm 安装 CLI、DSH 入口和 Agent skills，并复核新 Codex Pro adapter 与完整测试边界。

| 编号 | 工作 | 验收 |
|---|---|---|
| N01 | 核对 Codex Pro 最新 GitHub/npm | 固定实际可见 SHA，运行其离线测试与契约探针；未满足公开契约不冒充已接入 |
| N02 | npm 一条命令安装入口 | 精确版本全局安装、内嵌 skills、正式 DSH CLI 注册、可重复执行、结构化结果 |
| N03 | npm 运行进程与桌面路径 | shim 转发信号并等待 Go 收尾；worker 与桌面入口按安装路径解析 |
| N04 | skills 安全升级 | 用户修改与额外文件保留，staging/rename/rollback，全部结果可查询 |
| N05 | 六包构建与发行 | 四个平台包、插件与 meta 同版本；候选身份、integrity、版本存在与网络错误明确区分 |
| N06 | 真实安装验收 | 隔离 HOME/prefix/cache，从 npm 候选安装后运行 CLI/worker/skills/注册/取消；Linux/macOS |
| N07 | 文档与正式交付 | npm 主教程、使用与排错、验证证据更新；提交/PR/review/CI/merge；新版本 registry 安装验证 |

参考官方 `larksuite/cli@c910e89e97e98a458a051c50dbd36de43a603d30` 的显式 install 向导，保留本项目既有 npm 平台包与内嵌资源设计。安装不执行付费推理，不读取或迁移账号凭据，不修改用户其他插件。

实施后的本地结果见 [npm 验证记录](../validation/2026-11-01-npm-installation.md)；双平台 CI、合并与公开 registry 安装以 PR 和发行 workflow 的实际结果为准。

测试边界：已有 100% Go statement coverage、Node/DOM、真实本地进程与 Git、模拟 SDK 桥接、双平台 CI 已执行；此前没有验证 npm 安装后的整个调用链。真实 DSH Mac UI、Seatbelt、账号授权与真实模型任务仍需在用户设备确认，不由离线成功推断。
