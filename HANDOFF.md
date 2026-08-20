# Flywheel — 新 Session 交接上下文

> 给接续工作的 agent：你在 `~/Documents/projects/flywheel/`（git 已 init 但**尚无 commit**）。仓库已含完整实现：bin / orchestrator / templates / skills / tests / drills / README（核心 2026-08-20 建成，npm test 166/166 全绿）。
> **第一步先通读 `DESIGN.md`（v0.15，约 500 行）**，它是唯一权威设计文档。§2 的十条设计铁律是最高约束，任何实现决策不得与之冲突。

## 项目一句话

Flywheel（飞轮）是一套 loop engineering 系统：外部编排器进程（不是 agent）驱动多个 agent loop（规划/执行/评审），接收已冻结的需求，自动完成软件任务的规划、实现、评审与分层验证，全程可无人值守，异常升级给人。

## 已定型的关键决策（细节全在 DESIGN.md）

- **编排器是确定性代码**（Node.js CLI），不是 skill、不是 agent；skill 只是触发按钮。
- **三个主 loop**：Plan = kimi code + Kimi K3（长驻会话）；Execute = pi + DeepSeek V4 Flash（每单元全新会话，CLI `pi -p` 驱动，容器内）；Review = pi + DeepSeek V4 Pro（全新会话）。
- **全部模型 reasoning_effort=max**；会话级模型隔离（一个会话从生到死只绑一个模型，跨角色只传文本契约）。
- **执行路由**：packet 带 `route` 字段；`frontend` → kimi code + K3，`default` → pi + V4 Flash，`hard` → K3（已决，§11.6；V4 Pro 为降级渠道）。
- **验证体系**：L1 确定性（含变更代码 100% 覆盖率闸门 + 存量 ratchet）→ Review（LLM 评审）→ 里程碑关闭前 L2/L3（真实环境端到端）。
- **state.json 只由编排器写入**（v0.7 修订）：agent 只写 done-report，编排器校验推进后自行更新；缺陷注入为编排器确定性脚本执行的抽检；Plan 长驻会话死亡走冷启动重建协议。
- **无容器沙箱**（v0.10 决策）：Execute/Review 本机直跑，信任模型同日常 kimi code auto 模式；测试/boundaries 完整性靠 git diff 事后判定；executor 环境只注入低价值 executor key；接入不可信输入前重新评估容器化。
- **契约层**：spec.md（编号需求 R-01…）+ task-packet（带 contract-version）+ state.json（编排器唯一信源）+ retro.md；全部进 git、全部已提交。
- **触发方式**：skill 族（`/skill:flywheel-new/run/plan/exec/review/verify/status/stop`），编排器以独立进程拉起，不住在任何 harness 会话里。
- **渠道**：Kimi 走 kimi code 订阅；DeepSeek 走 OpenCode Go 订阅（注意 Pro 月额度仅 $15），DeepSeek 直连 API 作降级备份。

## 本机环境事实（已验证）

- pi 0.84.0 已装；`OPENCODE_API_KEY`、`KIMI_API_KEY` 已在环境变量中；pi 的 `opencode-go` provider 目录里有 `deepseek-v4-flash` 和 `deepseek-v4-pro`。
- 编辑格式 A/B 已跑（结果在 DESIGN.md 附录 C）：**Execute Loop 用 pi 原生编辑协议，不引入 hashline**。
- pi CLI 驱动链路（`pi -p --model opencode-go/deepseek-v4-flash --no-session`）实测可用。

## 下一步任务（按顺序）

1. ~~实证门槛 §10 第 2 项~~（**已完成 2026-08-18**：Go 透传 `reasoning_effort` 确认，max 档推理量为 low 档 2.4–3.4 倍；Execute/Review/Judge 保留 Go 通道；pi L0 冒烟通过。数据与注意事项见 DESIGN.md 附录 D）。
2. ~~实证门槛 §10 第 3 项~~（**2026-08-20 用户确认关闭**：kimi code 长驻/1M/max 可用性（含 frontend 路由）经用户确认，不再实测；驱动冒烟已通过，看门狗兜底）。已知限制：kimi stream-json 无 usage 暴露，Plan 的 token 暂不可观测。
3. ~~实证门槛 §10 第 4 项~~（**已关闭 2026-08-20**：403 演练完成（401/403 分类修正）；v0.15 裁决 DeepSeek 直连不采用、占位链已移除，降级链 = opencode-go → opencode(Zen 按量)；真实 403 报文不再专门验证）。
4. ~~核心建设~~（**全部完成 2026-08-20**）：`orchestrator/`（Node CLI，bin 名 flywheel）、`templates/`、`skills/`（10 个 SKILL.md）。当日完成：kimi 驱动、Judge、needs-live-credentials、403 演练；**7 块并行设计符合性审计 → v0.14（DC1=B 验收测试由 Plan 编写，DC2–DC6 六项裁决）→ 22 项修复**（缺陷注入 CAS 崩溃、Plan 修订重载、hard 降级断链、评审真 diff、pause 恢复、stop 杀在途工作等）；**外挂模块已建**：Intake（`flywheel intake`）、Garden（`flywheel garden`）。`npm test` **166/166 全绿**（含真实 pi/kimi 冒烟、/tmp 真 git 演练）。已知缺口：kimi stream-json 无 usage（v0.15 接受现状）、L2/L3 隔离环境（接功能任务时建）、局部并行（暂缓）。v0.15 裁决：DeepSeek 直连降级不采用（链 = Go→Zen 按量）、缺陷注入常备无开关、真实 403 不专门验证。§11.2/§11.3/§11.6 均已决；决策原则：质量/效率/速度优先，成本不计入。

## 工作规则

- **不要擅自 git commit / push**；任何 git 变更先问用户。
- 不偏离 DESIGN.md 的设计；发现设计错误时先与用户讨论，改文档（变更记录升版本），再改代码。
- 首批任务：目标仓库 mimir（`~/Documents/projects/mimir`，§11.2 已决），取其头部任务 S4-0/S0-00/M1-1 经 `/skill:flywheel-new` 生成契约；live 凭证任务（S2/S3 等）等核心链路稳定后再接。
- 与用户交流用中文；仓库内代码与文档语言遵循现有惯例（DESIGN.md 为中文）。
- **决策原则（2026-08-20 用户确立）：质量/效率/速度优先，成本不计入**。模型/通道选型、架构取舍时不再权衡价格因素；订阅额度与账单归用户管理。
