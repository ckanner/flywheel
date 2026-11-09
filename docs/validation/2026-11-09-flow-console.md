# 流程控制台实现与验收

日期：2026-11-09。源码基线为 `1087b686fa308284b6b68f238129f6779e0ba113`（0.29.1）。0.30.0 六包已发布并完成 latest 复核；正式源码为 `c24cb0e62042dfe3378bc9c06b0e6951159a7b25`。源码检查、npm 发行、本机安装与原生桌面交互均已完成，证据与边界分别记录如下。

## 已实现范围

- DSH 原生会话视图内的横向画布：Plan／拆分、调度、任务分支、Execute、L1、Review、集成、终验、PR、Merge；按需展开、分页、定位和有界诊断。
- 全局／项目配置编辑：默认模板、内容 CAS、备份、原子写入、Provider／Harness／模型／thinking、角色历史策略、逐绑定权限、预算和调度。
- 节点级执行、失败后有理由地继续一次、已完成节点重开、独立 packet 提前执行、验证后 Git 交付和单独终验；最近控制操作的结束回执与失败原因持久化。
- CLI 运行登记、多任务会话关联、项目／分支／HEAD／run 归属、调用绑定快照及按原 invocation 对账的指标。缺失／部分数据保留，不将调用失败率当作代码质量。
- 受管 packet session 的原 thread 交互、回执核对、验证交回、保持暂停退出。配置 policy-version 1 与会话存储 v2 分离；冻结的 0.29.1 schema 验证旧引擎拒绝新账本。
- 冻结依赖和写入范围下的独立 worktree 并行 Execute／L1；一个协调器串行 CAS／计费，hard 预算在计费确认前不释放调用槽；Review 与合成候选集成串行。
- 按明确 repo／HEAD／base 授权的 PR 操作，检查 fetch/push 目标、不强推、不隐式推 tags/submodules，更新已有 Draft PR，精确 HEAD 合并，远端排队与最终回执分开。

## 已完成的源码验证

- `make quality-gate`：vet、fmt、全量 Go 测试和逐函数 100% 覆盖通过。记录：`/tmp/flywheel-console-quality-final2.log`。
- 七个核心包 race 检查通过：machine、state、contract、config、metrics、drivers、watchdog。记录：`/tmp/flywheel-console-race-final.log`。
- DSH／Codex Pro／Claude Code Pro 三个真实 Go↔Node 离线桥接通过。Claude 使用已核对的 0.4.0 源码 tarball，不宣称 registry 已提供该版本。记录：`/tmp/flywheel-console-bridge-final.log`。
- 根目录 npm 141 项通过。真实引擎／公开适配器离线配对评估 36 条、96 次调用通过独立 oracle；这是 fixture 结果，不是模型质量、缓存或性能结论。记录：`/tmp/flywheel-console-sessions-final.log`。
- 客户端检查涵盖已发布宿主插槽、工具回执、fork／迟到响应隔离、配置冲突、节点证据、Session 接管、明确 PR 授权及多个任务关联。最终插件与六包安装门禁应在冻结候选上再次运行，结果与 candidate.json 一起保留。

## 关键故障检查

| 场景 | 验证结果 |
|---|---|
| 两个独立 Execute 同时在飞 | 实际重叠；第二次 Review 能读到第一次集成成果；没有丢失计费 |
| hard 预算／渠道并发限制 | 前一调用计费后再接纳下一调用，超额不派发 |
| 并行停止 | 等待所有已启动执行器退出；未知回执保持阻断 |
| 同一 thread 被分配给其他 packet | typed lifecycle 在 ACK 前拒绝 |
| 合成候选失败／冲突 | 不使用旧绿灯；保留 worktree、失败证据及成果 |
| 清理成功、最终 CAS 尚未持久化 | 仅在规范 checkout 仍匹配原验收候选时补齐交付状态 |
| 人工调用中断后重启 | 原回执只消费／计费一次；不会重新发送 Prompt |
| 配置／goal／权限或重建策略变化 | 精确接管请求拒绝换成另一个 session |
| 旧引擎读取 v2 | 加载即失败，不能忽略接管标记 |
| PR 预先存在或已合并 | 先核对实际结果；推送后不重复创建 Draft PR |
| 远端未知／排队 | 不把命令退出成功当成最终合并 |
| 200 个节点及大量历史 | 画布响应受限，完整节点按需读取，原始账本不变 |

## 视觉与真实运行边界

实际构建的客户端在隔离 HTTP 预览中接入真实 Go 控制接口，使用 mock drivers。检查亮暗主题、390px 容器、中文长路径、表单、展开、横移和节点操作。浏览器组名为“Flywheel 控制台验证”。预览不读取真实用户配置，也没有发送真实模型 Prompt。

旧版实现曾把原任务概况挤在画布之前；本轮已将概况和启动选项折叠。普通表单操作采用 36px 控件，紧凑工具保留 28px，触摸区域至少 44px，节点字重不超过 500，主题使用 DSH 别名。

真实 DSH 桌面版本已核对为 0.2.2-rc.1，源码验收时正式安装是 0.29.1。正式更新需在提交／发行授权及候选门禁后执行。真实账号模型质量／计费、完整 Provider 原生历史 UI 跳转和里程碑 session 的交互接管不由离线结果证明；当前交互通过控制台与受管 CLI 连接精确 packet session。

## 开发候选与发行准备

开发候选快照 `4d18b03eed0f2f5d304851df6cc7d3241594a81106f52a3863801658f133f1f6` 的同一组六包分别通过 macOS Node 24.22.1、Linux ARM64 Node 24.22.1 容器及用户 macOS Node 26.12.1 安装门禁，每次 10 项通过、零跳过。可选 Claude 使用真实 0.4.0 tarball，SHA-256 为 `125951f39d27a87072a9dc20c622a19089ad7ba84b69ba6228e1fbb03f46f581`。开发候选带 dirty 标志，不用于上传。

2026-11-09 用户已批准本轮提交、推送 main、创建并推送 `v0.30.0`、正式发布及桌面安装。正式候选需在提交后从干净精确 tag 重新构建一次，核对同组六包安装后才发布；本记录不预先声称这些后续步骤成功。

## 正式发行与本机安装回执

- `main` 与 `v0.30.0` 已推送。annotated tag 对象为 `c013931cf4fe6b690421745f9603fa3ddf016f94`，提交为 `c24cb0e62042dfe3378bc9c06b0e6951159a7b25`，tree 为 `0c33f6a3b4c5bea6be9cd440c6a02380e23053e0`。
- 正式候选从干净精确 tag 构建一次，快照为 `f943be278fe65d55014c5096acaf214635f6430c665a5e42264e25aaf3ee50dc`。同组六包在 macOS Node 24、Linux ARM64 Node 24 和用户 macOS Node 26 各通过 10 项安装检查、零跳过；发布重试一直复用这些 tarball。
- Linux 工作流要求的 machine／verify／watchdog／drivers／cli／modules／diagnostic 七包 race 检查通过。首次 root 容器使权限拒绝夹具失效，并缺少 gh 与 init；保留该失败，改用普通用户、gh 和 init 后通过，未修改源码或用例时限。
- [main CI](https://github.com/ckanner/flywheel/actions/runs/42601424905) 和 [publish](https://github.com/ckanner/flywheel/actions/runs/42601517516) 因账号付款／支出限制未启动 job。使用用户授权的手动发行，不将 Actions 记为通过。
- 六包按平台 → 插件 → 入口发布到 `candidate-0-30-0`，逐包核对完整性，随后官方 registry 安装通过（真实 npm、native CLI、11 个 skills、零推理请求）。六包 latest 更新与完整性最终复核通过，记录为 `/tmp/flywheel-console-0300-final-registry-fresh.log`；精确来源和远端 tag 又作了核对。
- 认证期间发生过超时及 npm 服务端拒绝旧 authId；仅重开对应官方认证，不重建包。最终只读 npm 查询也出现过超时，失败记录保留；使用独立查询缓存和更短的单次 fetch 等待后，同一 `verify-latest` 门禁通过，没有放宽完整性或版本判断。
- 本机 CLI 与 DSH desktop profile 插件均确认 0.30.0，安装报告 `ok:true, ready:true`。源码目录内的首次 npx 调用命中旧全局入口，版本核对发现后改为精确全局安装和明确路径 setup，未将旧报告当作升级成功。
- 用户要求的全局配置完整覆盖已通过 revision 检查、备份和原子写入完成，归一化值与 0.30.0 内嵌默认值完全一致。当前为 scoped／policy-version 1、full-access、serial／max-parallel 2。配置 SHA-256 为 `aff154c7f9d61afe3192f4f9b54bc32d50c7b84120d19756e350ca3fda8d6835`；旧配置另存备份。
- 无推理 doctor 通过，四项 DSH SDK 探测通过。可选 Claude 0.4.0 使用上文记录的真实源码包安装到同一全局 prefix，worker doctor 确认接口可用；不宣称它已公开发布或真实模型已验证。
- 桌面重载前确认当前显示的两个后台任务均已结束。Mac 锁屏导致的暂停已在用户解锁后解除。专用会话使用已发布的原生 AgentLoop／SDK 和本地固定响应适配器，真实模型请求为 0；该离线轮次只用于进入原生会话视图，不伪造真实模型效果。

## 原生桌面交互验收

- DSH 0.2.2-rc.1 的“统计”之后显示 Flywheel，实际进入“Flywheel 流程控制台”。会话为 `flywheel-console-desktop-d0634ce3-c14a-422d-a46d-28bf13c3c618`，名称明确标记“离线控制台验证”。空白新会话首页不显示这排会话视图，测试使用已有完整离线轮次的专用会话。
- 通过原生面板读取隔离任务，横向主线和任务分支正常出现；展开 M01-T01 后可见 Execute、L1、Review 和 Git 交付，节点详情带目录、分支、HEAD、run、失败次数与证据入口。
- 从桌面填写人工继续原因并点击“人工继续一次”，真实 Go 控制接口接纳后显示运行中并禁用重复操作；结束后恢复暂停，显示预设 mock 场景的 L1／spin 失败原因。尝试与累计失败均由 5 增至 6，最终 revision 为 45，run 为 `YANVY5DOODUCY356S6TXLQFYY3`。随后 CLI 独立只读核对 `running:false`、`status:paused` 和同一失败回执。失败是夹具预期，不表示编码任务已完成。
- 原生“全局 / 项目配置”读取的目标是全局配置文件，显示 scoped、完全权限、串行与最大并发 2。资源菜单包含 DeepSeek Account · Flash、ChatGPT · DSH、Kimi Code · K3.1 Pro、ChatGPT · Codex Pro、Claude Code Pro；打开后取消，没有改变默认绑定。
- 验证后全局配置摘要仍为上文记录值。桌面留在已暂停的离线测试画布，真实模型推理、账号计费、实际外部 PR／Merge 和完整 Provider 历史 UI 跳转不由此次检查证明。

## 原始失败与处置

- Codex 自带 Git shell 包装器叠加测试负载，触发三个既有 CLI 用例的时限失败。失败日志保留；随后定位工具链差异，使用系统 Git 与固定 Node 24 验证，原用例时限没有放宽。包级总等待窗口为 30 分钟，用于容纳增长后的套件。
- 全量检查发现空节点详情的空值访问和提前取消的错误分类，已修复并回归。
- 并行执行重试耗尽后保留原 Plan 升级路径，在集成之前完成失败单元的收尾／修订，避免把局部成功误当作整批完成。

本轮没有通过测试夹具制造真实 PR、Merge、npm 发布或模型业务验收结果。发布候选的 SHA、tarball integrity 与安装记录由发行工具产出；源码门禁不能替代它们。
