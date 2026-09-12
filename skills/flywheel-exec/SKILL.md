---
name: flywheel-exec
metadata:
  version: "1.2.0"
description: Trigger only the Execute Loop for a Flywheel task, optionally for one packet (single state transition). Use when the user says "/skill:flywheel-exec docs/<slug> [packet]" or wants to run just one execution unit.
whenToUse: 当用户只想手动跑某个执行单元（调试单个 packet、重跑失败单元）时使用。
compatibility: "Requires the flywheel CLI (npm) and the route harnesses: zcode (Z.AI Coding Plan login, Execute default/multimodal = GLM-5.3/-flash) and/or kimi CLI (frontend/hard = K3)."
arguments: slug packet

---

# flywheel-exec — 只触发 Execute Loop

Button only: validate + invoke CLI + translate. No loop logic here.

> 参数经 kimi 命名参数注入正文：`$slug`、`$packet`（其他 harness 忽略 arguments 字段，实参随调用文本原样传入，语义相同）。

## 步骤

1. **校验**：`docs/<slug>/` 契约文件齐全。若给了 packet id（如 M02-T03），
   确认对应 `task-packet-*.md` 存在。
2. **调用**（在目标仓库根执行）：

   ```bash
   # 指定 packet
   flywheel run "$slug" --only execute --packet "$packet"
   # 或不指定：取当前里程碑下一个未完成 packet
   flywheel run "$slug" --only execute
   ```

   若 flywheel 不在 PATH：`flywheel run docs/<slug> --only execute [--packet M02-T03]`（flywheel 应已在 PATH 上——npm 全局安装或开发机 go install；不在则先安装，不要绕道源码仓库）。
   （单二进制：`npm i -g @ckanner/flywheel` 后 `flywheel` 即在 PATH；不在则先安装，不要从源码仓库构建。）

3. **翻译结果**：执行是否通过出口契约（进程终态/stderr/交付物三项独立判定）、
   done-report 是否合法、token 与成本增量、state 中该 packet 的新状态
   （executed / 失败与错误码）。提醒：execute 之后还有 L1 与 review，
   单元不算完成——完成由编排器与评审判定，不由执行者宣布。
