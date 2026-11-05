# 订阅模型迁移、完全权限与任务 UI 验收

日期：2026-11-05。基线：`987a8641f09cdab83d81996df66eee183c073b7d`（0.28.1 发行准备）。

## 问题与修复范围

用户截图显示旧全局配置仍使用 Z.AI／GLM 和 Kimi K3，未采用已约定的 ChatGPT 订阅、Kimi Code 订阅和 DeepSeek Flash API 分工。安装会保留已有完整配置，新模板不会覆盖旧全局或更高优先级的项目／任务文件；此外 Kimi driver 的省略模型默认仍为 K3。既有 `approval=never` 只是非交互审批，不能把原来的 workspace-write／read-only 变为完全权限。

本轮增加显式 `config migrate --preset subscriptions`，按配置检查的同一优先级更新实际命中的文件，备份原始字节并保留非模型设置。迁移替换全部约定的模型、thinking 和活动渠道，关闭旧第二意见并报告移除旧多模态路由；未知自定义阶段／路由阻断。未引用的 profiles／channels 保留，所选渠道的旧 fallback 和模型映射移除，旧 Z.AI 不再自动可达。

新增 `permissions.mode`：新模板与显式迁移使用 full-access；旧文件未声明时保留 role-based。所有阶段、渠道降级、配置查看、doctor 和 Plan 恢复身份使用同一个权限解析器。DSH 使用公开 danger-full-access＋never，Codex Pro 映射到其公开 machine API，Kimi 使用原生 print auto。完整访问不关闭契约、冻结验收和候选身份检查。

保留 PR #26 的原生 conversation.view 接入。面板补充已有任务的准备说明、任务资料目录／可选项目目录、检查安装与配置／检查 DSH 接入的用途；配置明确显示真实来源及权限，不自动扫描、创建或启动任务。

## 验证结果

环境：Linux amd64，Go 1.27.3，Node 24.22.1，npm 12.2.1。

| 验证 | 结果 | 证明范围 |
|---|---|---|
| `make quality-gate` | 通过；每个被测 Go 函数 100% statement coverage | vet、格式、完整 Go 回归；包括模型迁移、真实文件失败场景、权限与候选身份 |
| `npm --prefix plugins/dsh run check` | 87/87 | 构建、客户端行为、真实 rc.5 renderer 生命周期、DSH/Codex Pro 协议和进程收尾 |
| `npm test` | 126/126 | 启动器、安装报告、发行协议及取消行为 |
| 两项 Go → Node 离线桥接 | 2/2 | 真实 worker 进程、full-access 请求与回执、DSH 无推理 doctor、真实 Codex Pro 0.8.0 machine API |
| `go test -race`（config、machine、verify、watchdog、drivers、cli、modules、diagnostic） | 8 包通过 | 本轮变动及核心并发生命周期，无 race 报告 |
| 六包隔离构建 | 通过 | 四个平台原生二进制、插件与 meta 同版打包，校验包身份和 integrity |
| 同一候选的真实 npm 安装 | 7/7 | Linux amd64 的安装、重跑、用户配置保留、失败恢复、worker 调用与取消 |
| `git diff --check` | 通过 | 变更无空白错误 |

跨进程检查使用实际安装的 Codex Pro 公开 API；DSH SDK 对端和 Codex app-server 对端使用离线协议夹具。它们没有验证用户的真实账号、额度或模型响应。四个平台均完成交叉构建，实际安装执行仅在 Linux amd64。

## 关键失败场景

- 预览不写文件；应用前备份原始内容；大整数预算、用户说明、验证和 worktree 设置不丢失。重复应用不重新格式化或再建备份。
- 任务／项目／全局优先级、物理路径、陈旧预览、文件身份或权限变化、锁冲突、备份／写入／发布失败均有回归。新建全局文件采用排他发布，不能覆盖抢先创建的用户文件。
- 受限执行端不能用错误回执冒充完全权限。DSH 在 prompt 前拒绝策略不匹配；full-access 如实记录不受 OS 沙箱限制，不伪造 enforcement。
- Codex Pro 的请求枚举、实际 sandbox 和精确策略形状分别校验；兼容 Codex CLI 不伪造该插件才提供的有效配置回执。
- Plan 权限改变或旧状态缺权限身份时冷启动。完全权限的 Reviewer／第二 Judge 若改动候选，即使声称 pass 也不能推进交付。
- 独立审查发现“应用失败时仍展示候选为普通配置”的问题，现以 `configurationStatus` 区分 proposed／effective／unconfirmed，并补充只读文件失败和写入后读取失败回归。复核确认问题关闭。
- 安装报告区分配置 created／existing／error，不将初始化失败误报为已创建。安装本身不偷偷迁移已有模型配置。

## 候选身份与发行边界

本轮开发候选仍使用仓库现有版本 `0.28.1`，基线提交如上，`dirty: true`。源码快照：

```text
67607ab3938e2c883d014fc5921d7599ea2e12a02f18d346948714a23f7f75f5
```

六包为 `@ckanner/flywheel`、`@ckanner/dsh-plugin-flywheel` 及 darwin-arm64／darwin-amd64／linux-arm64／linux-amd64 四个平台包。安装测试直接消费这批 tarball；测试后源码快照再次核对一致。本验收记录在候选验证后补入，不改变被测程序或打包清单中的用户文档。

此候选只作开发验证，不能覆盖 npm 已发布的 0.28.1；本轮没有发布包或推进 dist-tag。检查时 registry latest 为 0.28.1，其中尚无新迁移命令和 permissions.mode。源码提交、PR／CI 状态和后续正式发行须分别报告。

## 实际限制

没有接入用户 Mac，未修改截图中的全局配置，也未完成 Mac 桌面几何、账号登录或真实付费模型调用验收。任务运行中不会热加载配置，下次引擎启动才使用新绑定。

迁移采用同类进程互斥、发布前文件复核和完整文件的原子可见替换。它不声称对任意外部编辑器提供原子 CAS，也不声称目录 fsync 所需的掉电事务保证。迁移期间不要同时编辑目标文件。

完全权限通过后端正式接口请求；宿主静态拒绝、管理员配置和操作系统账号权限仍然有效。它不会自动把桌面全部插件能力复制到独立 worker，既有辅助模型／自主委派／web 工具限制仍在。
