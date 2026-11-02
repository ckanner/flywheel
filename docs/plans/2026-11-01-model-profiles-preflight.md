# 模型配置与安装预检实施计划

基线：已合并的 `1c515ca0c694047d1f6c7d7c3d52853cc9bf9c2d`。用户要求继续完整实现，并沿用提交、PR、自审、双平台 CI 通过后合并的交付流程。

## 本轮目标

| 编号 | 工作 | 验收 |
|---|---|---|
| P01 | 具名 profiles 与阶段引用 | 加载期展开、显式覆盖、错误前置、旧配置兼容、逐函数 100% |
| P02 | provider 与账号 channel 分离 | 各角色、执行路由、第二意见与后备渠道都传达实际 provider，计量保留两个维度 |
| P03 | config inspect 与 doctor | 相同配置发现逻辑，只读、不推理；缺命令／错配置／SDK权限失败可解释，未知授权不伪造成功 |
| P04 | DSH 配置与诊断界面 | 展示模型与检查清单；准确区分完成／停止／暂停，旧路径结果不混用；真实 React DOM 行为测试 |
| P05 | 文档、skills 与发布 | 安装教程、配置示例、交接文档和验证记录同步；Go、Node、跨进程、race、Linux/macOS CI 全部通过 |

## 接口约定

profiles 是配置引用层，不负责模型自动选型、继承树或并行调度。provider 表示 DSH adapter，channel 表示凭证／计费渠道，实际执行字段必须贯穿到 driver。Kimi 保持原生 CLI；DeepSeek 默认仅 Flash；Codex CLI 保留独立完整 harness。

config inspect 与 doctor 可在创建任务前运行；传入任务时按实际任务检查必需阶段与路由。无推理 probe 只确认 adapter 接受配置、cwd、权限和 SDK 收尾，不能代替账号与真实模型验收。

桌面仍只调用 Go 的 control 协议。config/doctor 可省 taskDir；doctor 允许 probe 布尔选项。诊断失败仍返回完整结构；协议或进程错误单独处理。普通状态展示不等同于任务运行状态机。

## 外部 Codex Pro adapter

只读核对 `ckanner/dsh-subagent-codex-pro@21fd7acccf1f5989b14fecdc1957559eea6914f8`（0.7.0）。其本轮加固已有进展，但公开 provider 的有效配置、完整结果、清理确认及进程组所有权仍不足以满足本引擎契约。实际 rc.5 SDK 也没有直接 external-subagent RPC。保持外仓独立，将固定版本缺口及复现更新到交接文档，不通过嵌套父模型或未公开 deep import 假装完成集成。

## 验证边界

默认验收仅使用离线协议桩、真实本地进程、Git 与 DOM。真实 Mac DSH 安装、Seatbelt、账号和推理须在用户设备验证。源代码、配置与界面实现完成不能替代这些证据。
