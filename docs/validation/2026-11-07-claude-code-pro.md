# Claude Code Pro 接入、设计复核与验收

本记录对应 2026-11-07 的增量任务：复核近期 PR、会话设计与实现，接入
`dsh-subagent-claude-code-pro`，先提供可配置能力，不预先决定使用哪个 loop。
源码实现、离线验证、PR 合并、npm 发布和用户安装是独立状态。

## 1. 设计与基线

本次读取并核对的实际 GitHub 状态：

| PR | 状态 | 与本次工作的关系 |
|---|---|---|
| [#29](https://github.com/ckanner/flywheel/pull/29) | closed、已合并 | Auto／Quick／Full 与统一任务入口，保留已有实现 |
| [#30](https://github.com/ckanner/flywheel/pull/30) | open | scoped 会话、角色隔离与分期恢复的设计方案 |
| [#31](https://github.com/ckanner/flywheel/pull/31) | open | P1/P2 角色账本、正常续接及已保存回执恢复的实现基线 |

Flywheel 基线为 #31 的 `56bfda287587fec64d0d812de2726695e0d2d727`，
tree 为 `3145b47e12c2f65d2aebf60200c69aa7831b05af`。新增改动堆叠在该实现上，
不将仍然打开的 PR 描述为已经合并。

复核重点包括 `docs/design.md` 的设计不变量、会话方案、真实调用链、状态 CAS／回执
消费、角色保护、计量、配置诊断、技能入口、npm 安装和 CI／发布验证。Go 仍是唯一
编排与状态所有者；skills 只执行校验、调用和结果翻译。

## 2. 本次实现

- 新增独立 `harness:claude-code-pro`，通过受管 Node worker 调用公开 machine API。
  不把 DSH 工具名 `subagent_claude_code` 冒充原生 LLM provider，也不增加父模型委派。
- 配置入口覆盖 Plan、Execute routes、Review、Judge／第二意见、Intake、Garden。
  模型与 effort 明确传递；模板只增加未绑定的 `claude-sonnet` profile，现有默认绑定不变。
- 接通 fresh／scoped、显式 session resume、请求／run／session 身份与原始回执。
  主任务沿用已有角色账本和回执恢复；Intake／Garden 保持独立建议调用。
- 完整模型 ID 要求实际观察值一致；sonnet／opus／haiku 别名核对对应 family。
  CLI 未证明的实际 effort 不会被写成已验证。
- 首版只接受显式 full-access，映射原生 bypassPermissions；原生审批模式不等于
  read-only／workspace-write 的 OS 沙箱。不支持的权限在派发前拒绝。
- 新增 worker 包装器、npm 路径解析、安装报告、doctor／inspect、双语使用说明和技能
  前提。Claude 插件是精确 optional peer 0.3.2；缺少它不影响未选择 Claude 的流程。
- 未知派发、结果或清理不能进入成功路径，也不能通过 fresh、重试或换渠道绕过。
  原始 partial／unknown usage 继续保留；CLI 的 API 等价成本不是订阅账单。

完整配置和能力边界见[中文指南](../claude-code-pro.md)与
[English guide](../en/claude-code-pro.md)。

## 3. 独立审查发现及修复

| 确认问题 | 修复及实际回归 |
|---|---|
| 本地 harness 拒绝丢失 Session 事实 | 原基线 fresh／scoped／保留检查点三个反例均失败；修复后明确 not-dispatched、已清理和零消耗，修正配置后可继续 |
| Kimi／DSH 派发前取消误记 unknown | 只有 watchdog 已证明未启动才记录未派发；实际启动后的取消不冒充可复用终态 |
| 超长 Claude 请求尚未解析 ID 就被 worker 拒绝 | Go 对实际发送的同一 JSONL 字节预检 1 MiB；包括转义和换行，精确上限及多 1 byte 均有回归 |
| 矛盾 Claude 回执可能留下可继续状态 | 核验配置、退出、终态、清理、usage 和输出组合后再赋予可信状态；坏尾帧、重复终态、错 session、错模型等全部拒绝 |
| Intake／Garden 在结果未知时仍可能选择后续候选 | 保存已知计量后停止 retry／fallback，记录 invocation 原因；不以进程已清理推断业务结果 |
| 包含缓存的输入被重复计价 | DSH／Codex Pro／Claude Code Pro 先拆出未缓存输入，再各计一次；保留原始 token 口径及 legacy pi 语义 |
| 强杀 worker 后原生 Claude 仍存活 | 上游先建立独立 guardian 的 IPC lifeline，再派发 CLI；父进程消失仍能清理 native group |

缓存数值反例：100 个未缓存输入、900 个 cache read、10 个输出，输入／输出每百万
$1、cache read 每百万 $0.1，正确估算是 $0.0002，旧实现会算为 $0.0011。
非法缓存子集、负数与溢出式输入返回未知，不生成负成本或重复费用。

guardian 原始反例在 worker SIGKILL 后 300 ms 仍观察到 native 存活。修复后的
独立探针使用最终真实 npm 安装（非 symlink）：对整个 worker PGID 强杀，普通挂起的
native／guardian 均退出，忽略 SIGTERM 的 native／guardian 也在强制清理后退出。
另验证单独强杀 guardian 时，仍存活的父 API 清理已登记 native，并返回 unknown。
这些测试没有将清理成功解释为业务完成；所有测试进程已回收。

## 4. 上游交付与真实依赖

配套 [Claude Code Pro PR #1](https://github.com/ckanner/dsh-subagent-claude-code-pro/pull/1)
新增公开 machine v1、类型、双语契约、严格流校验及进程监督。保留原有 51 项桌面测试，
总计 80／80 本地测试通过。公共 machine.js 的 line／function coverage 为 100%，
branch coverage 为 94.64%；不声称上游全仓 100%。

| 身份 | 值 |
|---|---|
| 最终上游 source commit | `28483f8aafc15463c6d365100b64c04480cb56b5` |
| 最终 source tree | `573e322a7596845d16d84ddf9c2140f967b20fae` |
| package / machine protocol | `dsh-subagent-claude-code-pro@0.3.2` / `1` |
| tarball SHA-256 | `d55b3a52d7b0d350556f65da572194bfe3db89550f4b7ea4cbdbab7997fbf125` |
| npm integrity | `sha512-ly9kmxP+n3+GxhYgiKLrJTdWBxcGf9gRcb5lF5G27Ikk8wiq1cdr5iiTxsOVeK3s72JEeuLOLIqDWjCj1voX9w==` |

npm 12.2.1 生成真实 tarball，60,585 bytes、37 files、零运行时依赖。它被实际安装到
Flywheel 的 Node 依赖树，并核对 machine／guardian／runner／类型的文件字节；
全局隔离安装测试再次安装同一个 tarball。未编辑版本伪装 0.3.1 兼容，也未虚构 npm
registry 的 0.3.2 发行或 lock integrity。

上游[最终 CI run 42393984627](https://github.com/ckanner/dsh-subagent-claude-code-pro/actions/runs/42393984627)
中，Ubuntu／macOS × Node 22／24 四个 job 均成功。远端首先暴露两处测试假设：macOS
临时目录要比较 realpath，孤儿 guardian 的 EPERM 探针应继续等待且只以 ESRCH 认定消失。
两个后续提交只修改测试，保留原截止时间及断言；运行时代码、打包文件和 tarball hash
均未改变。这个结果不代表真实账号或用户 Mac 桌面 live 已通过。

## 5. Flywheel 完整本地验收

环境：Linux amd64、Go 1.27.0、Node 24.22.1、npm 12.2.1。测试使用真实公开插件／SDK
与真实子进程；原生模型服务由离线 CLI／app-server fixture 代替，没有发出账号推理请求。

| 检查 | 最终结果 | 原始记录 |
|---|---|---|
| `make quality-gate` | vet、gofmt、全部 Go 包、每个被测函数 100% statement coverage | [Go 门禁](evidence/claude-quality-gate.txt) |
| 七个核心包 `go test -race` | 全部通过 | [race](evidence/claude-race.txt) |
| `npm --prefix plugins/dsh run check` | 168 个主测试 + 8 个隔离 rc.5 宿主测试通过；0 skipped | [插件测试](evidence/claude-plugin-tests.txt) |
| Go → Node → DSH／Codex Pro／Claude | 三后端及全部子场景通过；Claude 15 个子场景，缺 peer 会失败而非跳过 | [桥接测试](evidence/claude-node-bridges.txt) |
| 根目录 `npm test` | 127／127 通过 | [npm 契约](evidence/claude-npm-tests.txt) |
| 六包隔离构建 | 四平台二进制、DSH 插件、meta 同版本候选成功，源码 clean | [构建](evidence/claude-npm-build.txt) |
| 实际 npm 安装 | 9／9 通过，0 skipped；包含缺 peer 与显式安装 peer 两条路径 | [安装](evidence/claude-npm-install.txt) |
| 原生 CLI | build、version、control／doctor help、JSON inspect 与内嵌 profile 通过 | 实际 8 个默认 binding，未自动选择 Claude |
| 独立审查 | 所有确认问题关闭，最终安装字节上的 worker 整组强杀反例通过 | 本记录 §3 |

安装测试使用隔离 registry、HOME、global prefix 和 DSH 协议桩；确认用户配置、已修改
skills 与重复 setup 行为。安装后的 worker 导入真实 machine API，执行离线 Claude，
确认配置、持久会话、22-token 用量、unknown 订阅成本和进程组消失。

CI 与正式 publish 的 verify 阶段均固定上述上游 commit，单次打包并核对 SHA-256，
两种 OS 消费相同 artifact，三后端 bridge 与真实可选安装都启用。npm 12 的
`--no-save` 不会单独放入该 optional peer；验证环境临时用 `--save-dev` 安装真实包，
构建／测试前恢复两个源 manifest，退出 trap 再恢复。正式六包发布、tag、registry、
promotion 门禁没有改变；本次没有触发发布。

## 6. 已测源码与候选身份

| 项目 | 值 |
|---|---|
| 已测本地代码提交 | `2e2c20bfbfe68981259bcbbd32eb3f2941d9d5a1` |
| 相同代码的 GitHub 提交 | `4234ca9d2de224e09a9751654711b2b902cfe6ab` |
| 两端一致的代码 tree | `f71e267e3a774867fd0b2d6fdac92982ba2f46a9` |
| 六包源码快照 SHA-256 | `57a9c5afdfc129bc9a88abe79a15a47c3e4a3163c3c0af5ee849a1efc63b03cd` |
| candidate.json SHA-256 | `a5544e2dfe94827e89dcefa857662a3c335b68e68bec92f01d4faa6866f56971` |

完整六包身份见实际保存的 [candidate.json](evidence/claude-npm-candidate.json)。
后续验收提交只增加本记录／原始日志、设计链接，并将两个工作流的上游 source pin
推进到只修测试的最终上游提交；没有修改已测运行时代码、测试、技能或打包文件。
PR 上分别记录 Flywheel 远端 Actions 状态，不用本地通过替代远端执行证据。

## 7. 仍然明确保留的边界

- 0.3.2 是配套源码候选，不等于已发布 npm。Flywheel 源码版本仍为 0.28.5，本次不覆盖
  已存在的 registry 版本，不移动 tag／latest，不修改用户现有 Mac 安装。
- P1/P2 的正常终态续接、角色隔离及已保存回执恢复继续成立；P3 的 early durable ack、
  在途只读查询与任意崩溃点恢复没有实现。guardian 解决物理监督，不恢复未知业务效果。
- 同时失去父进程和 guardian，或 guardian 在报告原生身份前被强杀，仍可能缺少完整
  清理证明。未知结果保持阻断，不能凭可能已结束的进程推断成功。
- P4 的真实账号、模型质量、缓存收益和性能对照尚未运行。上游 macOS 离线 CI 通过，
  不等同于 Flywheel 在用户 Mac 桌面、实际 CLI／账号或 Seatbelt 的 live 验收。
