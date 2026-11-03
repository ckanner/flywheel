# Codex Pro 默认接入验证记录

## 1. 固定基线与实际接入

Flywheel 以 `cb7d996eb233565f31d6804b3c6963a18cdb0d93` 为基线，发行目标 0.28.0。上游核验对象为 [Codex Pro v0.8.0](https://github.com/ckanner/dsh-subagent-codex-pro/releases/tag/v0.8.0)，main `04ecdb677735d84f3a0f5dba01a9d9b28c324c0e`。

已直接读取 npm 发布包，核对包名、版本、Node >=22.18.0、公开 `./machine` 和 `./package.json` exports。npm SHA-512 integrity 为：

```text
sha512-hbWPMkjWPsY8DDhcUIzBBCNR3TZexeOSSv9yP1zzIiyP2NvHfedfNkHz2ubreUyyonst+/539t7izwzl3ZOHxA==
```

集成实际导入这个包的公开 `runMachine`，不是复制 runner、调用旧 `codex exec` 或伪造 DSH 父会话。默认模板和 profiles 示例的 hard 都是 `codex-pro`；旧配置不自动覆盖。

## 2. 验收层次

| 层次 | 验证内容 |
|---|---|
| Go 单元／协议 | 配置与 profiles、请求身份、完整 receipt、模型／目录／权限、usage、错误映射、取消与物理终态；逐函数 100% statement coverage |
| Node worker | 实际导入固定 npm 插件，使用 fake app-server 执行真实 machine／guardian 代码；输入错误、超限、EOF、信号、输出断开和清理 |
| Go→Node→插件 | 真实父子进程管道，固定插件 runner／guardian 与脚本化 app-server；同一调用请求、返回、预算和终态验证 |
| npm 候选 | 六包构建一次，在隔离 registry／HOME／DSH 协议桩中安装；取得真实 Codex Pro tarball，验证公开 API、CLI、worker、10 个 skills 和 GUI 入口 |
| CI／发行 | Linux/macOS 消费同一候选，核心 race；发行另核精确 tag／source／integrity、registry 安装和 latest |
| 用户 Mac／账号 | 本环境未提供真实用户桌面与订阅；不据离线结果声称这层已通过 |

## 3. 发现并处理的关键问题

- **旧默认未消费插件：**新增独立 harness 和实际 machine API worker，npm 精确依赖与启动器解析；没有插件时拒绝，不自动使用旧 CLI。
- **参数与计量证据：**effort 仅 accepted、effectiveEffort 为 null；实际 provider 未提供则未知；完整 receipt 留存，partial usage 不当成完整预算或零费用。
- **两层进程收尾：**模型输出不证明执行结束；正常 app-server 收尾可以有终止信号，外层 worker 仍须可靠退出。真实 EOF、取消、父进程／worker 死亡用 PID／进程组消失验证，不只看文本日志。
- **全局停止提前截断：**原 SIGTERM kill switch 的默认 5 秒会绕过 worker 自己的 10 秒宽限。改为登记并尊重各子进程宽限，保留快照所有权与最终强杀／等待。
- **完整策略时点：**用实际 0.8.0 包和故意回报宽 roots／network 的 app-server 复现：插件会先发 turn，完整策略到最终 receipt 才交给调用者。因此 Flywheel 的完整策略校验只是交付资格检查；文档和 doctor 明确警告，不宣传为事前隔离。

## 4. 本轮执行记录

最终命令、结果与远端链接在完成统一门禁后补入。本节未填入的检查不能按通过处理。

## 5. 保留的能力边界

Codex Pro 0.8.0 没有公开的运行前完整 sandboxPolicy guard。workspace-write 允许 Codex 标准临时目录；用户／项目配置仍可能在调用前扩展权限，事后拒绝不能撤销访问。需要预防此类访问的环境须等待上游 guard 或另行采用已验证的宿主隔离，不能靠此回执检查声称具备该能力。

协议桩不证明真实模型质量、账号授权、订阅额度或美元账单。Mac 桌面 UI、Seatbelt、GUI 的 CLI／登录可见性、真实 Flash／GPT／Kimi／Codex 小任务以及本机重启恢复仍需在用户实际环境验证。缺少这些证据时不将其列为“完整实机测试通过”。
