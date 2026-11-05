# 0.28.3 registry 安装检查恢复（2026-11-05）

## 故障与修复

六包原候选均已上传并确认 registry SHA-512；进入公开安装门禁后，registry-smoke.js 仍硬编码旧 provider `deepseek-official`，而原生 CLI 正确返回本版已约定的 `deepseek-account`。同一源码中的候选安装检查已经断言账号 provider，两个检查的重复预期发生漂移。

候选与公开 registry 安装现在共用 `assertFlashAccountProfile`，仍严格断言账号 provider，不删除或放宽检查。以后过期预期会在发布前的真实候选安装中失败。此修复仅涉及未打包的 npm/test 校验代码和本记录，不修改运行时代码、用户配置、package metadata 或已上传 tarball。

单元门禁另外复现了夹具启动时序问题：1 秒外部步骤预算在 global-cli 阶段已经耗尽，未到用例预期的挂起 doctor。把该用例预算设为 3 秒用于准入真实 Node 夹具，保留 preflight 阶段、超时错误、失败报告及 trap 清理的全部断言；不修改生产超时或运行时代码。

## 固定的交付身份

发行源提交与 v0.28.3 仍为 `2f86fe8a5af39e3e89eba5d3b0d62cd97de7971e`，源快照为 `e15b6428b149ba93ef997ab0e2b58bb7664f58a6597e24f1a40a6064524369eb`。不移动旧 tag、不重建或替换六个已发布包。修正检查器保存在独立维护 checkout；本次 registry 门禁使用该检查器对公开的精确 0.28.3 运行，候选和发布来源仍绑定原干净 tag。

后续交互工具只将 `npm run test:registry` 的 cwd 指向已批准、干净且固定提交的维护 checkout；保留 FLYWHEEL_NPM_VERSION、官方 registry、安装环境、超时及其全部断言。原来源／tag／tarball identity、TTY、每次写入前检查和 latest 防倒退约束不变。检查器提交与最终 registry 结果在 GitHub Release 单独记录；维护源码不冒充发行源。

## 验证

macOS arm64，Node 24.22.1／npm 12.2.1：

- 根目录 npm 单元／协议检查 126/126 通过（141.910 秒）；原 1 秒启动预算的失败已复现并说明，3 秒夹具准入后的所有原断言通过。
- 原六包候选安装 7/7 通过，共用账号 profile 断言已在发布前路径执行。
- 公开 registry 精确 0.28.3 安装通过：实际 npm 安装、原生 CLI、10 个 skills 和 DSH CLI 协议夹具验证；真实桌面未验证，推理请求为 0。
- 再次核对原发行源码仍干净、快照一致，原六包与保留备份逐项完整性一致。

Go、DSH runtime 与发行包字节未修改，已通过的 Go quality/race、插件和桥接结果仍绑定原交付源码；不重新生成包。latest 尚未在本记录中声明完成，最终提升与新检查器提交身份以 GitHub Release 为准。
