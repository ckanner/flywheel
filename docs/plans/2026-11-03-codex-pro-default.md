# Codex Pro 默认接入实施计划

## 目标与边界

以 Flywheel `cb7d996` / npm 0.27.3 和已发布的 `dsh-subagent-codex-pro@0.8.0` 为基线。默认 hard 路由实际消费插件公开 machine API；保留 DeepSeek Flash 默认执行、Kimi CLI 规划／前端及 DSH 原生评审路径。Go 继续独占状态、预算、工作区、验收与 Git 提交。此次不引入 DSH 父模型、Teams、ACP 或自动跨渠道降级。

## 实施顺序

1. 新增 `codex-pro` Go driver 与独立 Node worker。协商版本化请求／进度／终态协议，严格校验插件回执，保留完整审计数据与 unknown usage/cost。
2. npm 精确安装插件 0.8.0，提供公开 worker 入口，验证 Node 解析、GUI 安装记录、hoisted dependency 和六包候选。新默认与旧显式 `codex` 配置可区分，绝不静默替换用户配置。
3. 扩展配置加载、有效配置查看、无推理 doctor 和模块只读权限。配置错误、入口缺失、版本不符在模型执行前失败。
4. 更新中文设计、README、安装／迁移教程、协议、技能按钮及版本；记录真实完成证据和实机边界。
5. 完成逐函数 100% Go 门禁、Node 行为测试、Go→Node→已发布插件离线桥接、取消／进程清理、六包真实安装及发行协议检查。
6. 提交 PR，独立复核当前候选，收集 Linux/macOS CI 与 race 证据。本次交付保持 PR 开放供审阅；合并与发布须另有明确授权并满足门禁。后续发行沿用既有 OIDC 流水线，构建、验证并发布同一候选，再确认 registry 六包及安装结果。

## 验收重点

- 配置 `routes.hard.harness` 为 `codex-pro`，插件依赖缺失不能回退到旧 CLI。
- machine API 使用请求模型／effort／cwd／只读或工作区写入；effort accepted 不伪装成实际有效值。
- 只在外层 worker 与插件终态都可信时成功；启动超时、父进程退出、取消、截断和清理失败均有失败证据。
- 记录插件完整 receipt；只有 complete usage 才记为已知预算，美元费用未知保持 null。
- 安装入口仍为一条 npm 命令，升级保留用户修改的 config 和 skills；受管 worker 使用同版本包解析。
- doctor 不发送模型请求；离线协议／安装证明与真实 Mac、Seatbelt、授权和模型推理证明分开记录。
- 0.8.0 对完整 sandboxPolicy 的校验只能是结果返回后的交付资格检查；Codex 用户／项目配置扩权的预防需要上游公开 pre-turn guard，不能把此集成宣传为该能力已经具备。

## 本机运行条件

Node 最低版本须满足上游插件 `>=22.18.0`。DSH 使用已安装的官方桌面／CLI 入口；Codex Pro 内部仍需可执行的 Codex 与用户登录。依赖安装不代替这些宿主条件，也不复制或打印凭证。
