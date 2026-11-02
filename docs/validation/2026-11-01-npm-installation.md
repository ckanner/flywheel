# npm 安装与发行：验证记录

日期：2026-11-01（America/New_York）。实施基线为 `39638bd96e6fcb6cc681bc7c1f90b7b082620936`，分支为 `feat/npm-agent-installer`，六包目标版本为 `0.27.0`。本文记录提交前的实际本地验证；PR、双平台 CI、合并和 npm 发布结果分别以 GitHub Checks／合并记录及 npm registry 为准。源码合并不等于已经发布。

范围见 [N01–N07 计划](../plans/2026-11-01-npm-installation.md)，用户入口见 [npm 安装与首任务教程](../getting-started.md)。前轮 [DSH 集成](2026-11-01-results.md) 和 [profiles／预检](2026-11-01-profiles-preflight.md) 的记录继续保留。

## 1. 实际执行结果

本地环境为 Linux amd64、Go 1.27.3、Node.js 24.22.1。测试使用真实 npm、Git、Go／Node 子进程、文件系统和本地 HTTP registry；DSH 注册与模型 SDK 使用明确的 fixture。没有调用付费模型，也没有操作用户 Mac profile。

| 检查 | 结果 | 证据 |
|---|---|---|
| `make quality-gate` | PASS：vet、gofmt、全部 Go 测试 | [输出](evidence/npm-quality-gate.txt) |
| 逐函数 Go 覆盖率 | **419 个函数全部 100%；6,219 个 statement，未覆盖 0** | [逐函数输出](evidence/npm-function-coverage.txt) |
| 七个核心包 `go test -race` | PASS：machine、verify、watchdog、drivers、cli、modules、diagnostic | [输出](evidence/npm-race.txt) |
| `npm test` | **52/52**：runtime、installer、release | [输出](evidence/npm-contract-tests.txt) |
| 六包构建 | PASS：四个平台原生包、DSH 插件和入口包，实际包内 manifest 与 SHA-512 校验 | [输出](evidence/npm-build.txt) |
| `npm run test:install` | **6/6**：同一批实际候选包从隔离 npm registry 安装后执行 | [输出](evidence/npm-install-tests.txt) |
| DSH 插件 `npm run check` | **38/38**，含 React DOM、控制协议与进程生命周期 | [输出](evidence/npm-dsh-plugin-tests.txt) |
| Go → Node worker → SDK 桥接 | PASS，fake SDK，不发送模型任务 | [输出](evidence/npm-dsh-bridge.txt) |

本地执行身份为 root，chmod 故障注入需要移除 DAC 绕过能力，所以 Go 门禁及 race 使用 `setpriv --bounding-set=-dac_override,-dac_read_search --inh-caps=-all --ambient-caps=-all`；没有删去失败断言、放宽覆盖率或将真实进程测试改成 mock。全局配置新增的 16 并发首次创建用例也单独通过 race 检查。

100% 指项目既定的 Go **statement coverage**，且逐函数检查；它不代表所有分支、JavaScript 代码、真实账号或模型质量均已覆盖。此次 npm 门禁补足了此前只测源码／本地构建、没有完整验证安装后调用链的缺口。

## 2. 一条命令安装实际验证了什么

主命令为 `npx -y @ckanner/flywheel@latest install`。集成测试通过真实 `npm exec --package @ckanner/flywheel@0.27.0` 获取候选，再由安装器执行精确同版本全局安装；不是直接运行 checkout 中的安装脚本。测试目录包含空格和字面 `$()`，HOME、prefix、cache、项目目录独立。

- npm 解析同版原生平台包与 DSH worker；安装后的 `flywheel version`、全局 worker bin、公开 worker export 和默认 worker 定位有效。
- 10 个内嵌 skills 安装到 Agent 目录；全局初始配置、8 个阶段绑定及模板可读取。
- 包内自带 `docs/examples/create-first-task.sh`，无需 clone 即可创建教程任务，并验证 packet 与未启动状态。
- 正式 DSH CLI 的 `plugin --profile desktop add/list` 参数、精确版本和失败返回已通过 fixture 核对；未把这个 fixture 写成真实桌面注册证据。
- 安装生成的稳定 launcher 和 receipt 可被安装后的插件解析；显式自定义 binary 优先，损坏的记录／不匹配的 launcher 不会静默回落。
- 重复安装保留用户配置；DSH 注册失败保留可查询的部分结果并可重试；`--no-dsh` 明确跳过注册；用户修改过的 skill 完整保留，安装不能假报成功。
- 安装后的 npm shim 收到 TERM，将取消传入真实 Go watchdog；等待其受管命令写入清理标记并确认退出。正常文本或迟到 exit 0 不能覆盖取消。

安装成功 `ok:true` 与任务就绪 `ready:true` 分开。测试故意令 Kimi 不可用以验证 `ok:true, ready:false`；不把缺少 harness 或账号信息隐藏为安装成功后的可运行保证。安装不替换既有 DSH、Kimi／Codex harness，也不代替登录。

## 3. Review 修复与保留的边界

本轮实现与独立只读 review 修复了这些具体问题，并保留对应回归：

1. npm wrapper 同步等待时无法可靠转发取消；改为异步子进程，等待 Go 的清理结果，不另起一套模型生命周期。
2. 安装命令 leader 退出后仍可能有后代运行；保留进程组探测、TERM／KILL 和有界收尾，超时、取消或未确认清理不能报告成功。
3. 单凭 managed marker 无法判断 skill 是否被用户修改；使用包含路径、类型、mode、大小、hash 的完整快照，升级前后复核，失败回滚或保留备份，冲突不覆盖。该操作不宣称十个目录组成跨目录事务或断电原子性。
4. 首次配置检查与写入之间可能覆盖新出现的用户文件；完整写入同目录 0600 临时文件，再用 no-replace hard link 发布，已有文件或悬空链接完整保留。
5. `npm view` 成功但缺少 integrity 不能证明版本不存在；只有结构化 E404 能进入首次发布路径，空／null integrity 拒绝，缺少 latest tag 单独处理。
6. npm 的 `130/143` 等异常退出码不能因 registry 已接受字节而当作成功；取消后停止发行，不继续后续包。
7. 安装完成、异步释放锁期间收到 SIGTERM 时，最终报告不能仍为 `ok:true`；统一将取消设为失败，同时保留已有清理错误。真实 installer 子进程的 unlock 窗口回归覆盖正常释放与释放失败。

最终全套 npm 回归也暴露了测试 fixture 的 PID 文件创建窗口：空文件被 `Number("")` 读成 0，`kill(0, 0)` 查询测试自己的进程组，从而误报 worker 存活。受控实验确认该场景真实 worker 已清理。现统一等待完整正整数 PID、原子发布 fixture PID，并预置空文件覆盖窗口；保留真实 PID 退出断言，不通过放宽存活要求消除失败。修复后完整 `npm test` 52/52 通过。

README、完整安装教程、首任务使用方法、插件 README／PROTOCOL、设计、AGENTS、CHANGELOG 和全部 10 个 skills 已同步。文档经过内部链接、shell 语法和 JSON 示例校验；skills 仍只负责输入校验、CLI 调用和结果解释。

## 4. 发行验收链路

普通 CI 构建一次六包候选，Ubuntu／macOS 下载同一 artifact 后安装验证。正式发行要求精确 tag 指向 clean HEAD，构建记录源 commit／tree／快照与实际 tarball SHA-512；验证任务通过后才进入发布。

六包先发布到候选 dist-tag，已有版本必须与验证过的字节完全一致。公开 npm registry 的精确版本安装测试通过后，按依赖在先、入口包在后的顺序移动 latest，并再次验证六包版本和标签。npm 不支持六包 dist-tag 原子事务，因此失败可以按已确认的状态重试，但不承诺多包瞬时切换，也不允许回退更新的 latest。

本地 dirty 开发候选仅用于测试，发布器拒绝直接发布。GitHub 发布 workflow 会从已提交的精确 tag 重建并重新验证，本文本地输出不能冒充该远端候选的通过证据。

## 5. Codex Pro 最新复核

截至 **2026-11-02 01:57:41 UTC**，GitHub `main`、独立 Git 拉取及 npm `latest` 仍对应 [`21fd7acccf1f5989b14fecdc1957559eea6914f8`](https://github.com/ckanner/dsh-subagent-codex-pro/commit/21fd7acccf1f5989b14fecdc1957559eea6914f8)／[`0.7.0`](https://registry.npmjs.org/dsh-subagent-codex-pro/0.7.0)。此次重新执行 typecheck 与测试，**106 pass、6 skip、0 fail**；跳过项含真实模型与本机缺少 Codex 的 preflight，不能算作 live 成功。

[本轮离线探针](evidence/codex-pro-070-probes.txt)仍复现有效配置回执缺失可接受、坏 usage 通知后仍标 complete、默认 full access、公共 run 子路径未导出。完整公开收据、权限和进程所有权的接入要求见 [adapter 交接文档](../integrations/codex-pro-adapter-handoff.md)。Flywheel 保留独立 `codex exec` 通路，没有把 DSH LLM provider 标签当成 external-subagent 接入。

## 6. 尚需实际设备／账号证据

用户 Mac 的 DSH 桌面插件加载与渲染、GUI PATH、Seatbelt、既有订阅授权、真实 Flash／GPT／Kimi／Codex 响应以及真实任务停止／恢复，仍需在对应环境验收。macOS CI 和本地 fixture 不替代这些结果。本轮没有模型质量、价格或吞吐量测量，不声称新的性能排名或成本节省比例。
