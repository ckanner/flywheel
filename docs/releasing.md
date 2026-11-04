# npm 发行指南

[English](en/releasing.md) · 简体中文

本页面向维护者。普通用户从 npm 安装，见[入门教程](getting-started.md)；不需要下载 Actions artifact 或手工解压 tgz。

## 1. 发布身份

GitHub Actions 使用 `.github/workflows/publish.yml`，只有 publish job 获得 `id-token: write`；GitHub `contents` 仍为只读。GitHub 登录允许触发有权限的仓库操作，**不会自动授予 npm 包写权限**。

CI、六包候选构建、双平台验证和 publish 统一固定 Node **24.22.1**、npm **12.2.1**；各 npm job 在运行门禁前安装该版本并输出实际 Node/npm 版本。preflight 只使用同版 Node，不额外安装 npm。npm 12.2.1 同时支持 OIDC 的 `npm publish` 和 `npm dist-tag`，不能只检查普通 publish 的最低版本。用户安装要求 Node 22.18.0 或更高，不要求 npm 12。

私有 GitHub 仓库也可发行公开 npm 包，但 npm **不支持私有源码仓库的 provenance**。workflow 从 GitHub 事件传入明确的仓库可见性：public 要求生成证明，private 显式传 `--provenance=false`，缺失或未知值拒绝发布。OIDC 身份认证与 provenance 是不同能力；本项目的源码、候选 SHA-512 与双平台安装门禁始终保留。不要为发包修改仓库可见性，也不要把缺失证明写成已生成。

推荐使用 npm trusted publishing。六个包分别配置如下，不能用其他 DSH 插件仓库的授权代替：

| npm 包 | GitHub owner | Repository | Workflow filename |
|---|---|---|---|
| `@ckanner/flywheel-darwin-arm64` | `ckanner` | `flywheel` | `publish.yml` |
| `@ckanner/flywheel-darwin-amd64` | `ckanner` | `flywheel` | `publish.yml` |
| `@ckanner/flywheel-linux-arm64` | `ckanner` | `flywheel` | `publish.yml` |
| `@ckanner/flywheel-linux-amd64` | `ckanner` | `flywheel` | `publish.yml` |
| `@ckanner/dsh-plugin-flywheel` | `ckanner` | `flywheel` | `publish.yml` |
| `@ckanner/flywheel` | `ckanner` | `flywheel` | `publish.yml` |

当前 workflow 没有 GitHub environment，npm 对应字段留空。每个 trusted publisher 必须同时允许 **Allow npm publish** 和 **Allow npm dist-tag**；后者默认关闭，与直接发布权限独立。新建 trust 还须在 npm 规定的有效期内完成首次发布。

如果仓库已有 `NPM_TOKEN`，流程保留 token 配置作为后备；当前 npm 会自动尝试 OIDC，不能把这条分支理解为强制禁用 OIDC。无 token 时使用独立的空 user npmrc，避免 setup-node 的占位认证配置。日志只显示可用认证方式，不输出 token，也不把 token 写进 `GITHUB_ENV`。不要用 `npm whoami` 代替实际 publish／dist-tag 授权验证。

**首次创建新包：**npm 要求包已存在才能配置 trusted publisher。新包需要维护者通过已获授权的账号或发布 token 完成首次创建；也可采用 npm 官方的 staged publishing 并人工批准。不能靠 GitHub 登录或给 workflow 加 `id-token: write` 创建 npm 信任。Flywheel 兼容仓库现有 token，让首次发行仍可由同一流水线发布已验证的候选；完成后再逐包迁移到 OIDC。不要为绕过首次授权而发布与本候选不同字节的同版本包。

官方依据：[trusted publishing](https://docs.npmjs.com/trusted-publishers/)、[npm trust](https://docs.npmjs.com/cli/v12/commands/npm-trust)、[dist-tag 与 OIDC](https://docs.npmjs.com/adding-dist-tags-to-packages/)。参考的同账号流程为 [Codex Pro](https://github.com/ckanner/dsh-subagent-codex-pro/blob/21fd7acccf1f5989b14fecdc1957559eea6914f8/.github/workflows/release.yml)、[ChatGPT](https://github.com/ckanner/dsh-plugin-chatgpt/blob/388868f91bcdc05795fb94d21b868ded6d7219c9/.github/workflows/release.yml) 和 [LCU](https://github.com/ckanner/dsh-plugin-lcu/blob/360d8ab909b3bd6a58c021dc22197c74a2e6f741/.github/workflows/release.yml)；Flywheel 保留自己的六包验证与提升顺序。

## 2. 触发和候选身份

1. 合并已通过 review 和 CI 的源码，六个 `package.json` 版本必须一致。
2. 在该精确 commit 创建 `vX.Y.Z` tag。推送 tag 触发 publish；GitHub Release 使用相同 tag。
3. 已有 tag 可从 Actions 的 publish 页面手动运行，`tag` 输入完整的 `vX.Y.Z`。不要输入分支或未经检查的新 commit。

preflight 要求 clean HEAD、精确 tag 和六包版本相同。build 从该 commit 的隔离快照打包一次，记录 commit、tree、快照及每个 tgz 的 SHA-512；Ubuntu 与 macOS 下载**同一个** candidate artifact 进行质量门禁和真实 npm 安装。publish 在工具链准备后，使用官方 checkout 的认证刷新完整远端 refs，HEAD 仍固定为已验证 source；再下载候选，检查新取得的 tag 必须指向该 HEAD。标签移动或删除都失败，不保留 Git 凭证供后续 npm 命令使用。

构建解析器接受 npm 11 的单元素数组及 npm 12 按完整包名索引的单包对象 `pack --json` 输出，仍严格核对对象键与包的 `name`、`version`、`filename`，候选身份和 integrity 检查保持不变。

registry 查询解析器兼容 `view --json` 的字符串和单元素数组，多结果或其他错误结构拒绝。缺失 latest 可表示尚未设置该标签；缺失 integrity 不能表示版本不存在，只有失败响应中的结构化 E404 才有此含义，认证、网络及取消错误仍失败。协议测试用隔离本地 registry 调用真实 npm CLI，覆盖有效字段、缺失字段、E404 和 E403；在与发布相同的 npm 工具链执行，避免只用手写返回值验证输出格式。

上传成功与精确版本可查询之间可能存在延迟，不能仅凭 `npm publish` 返回 0 就继续提升 `latest`。每个包的可见性验证使用单调时钟，最多等待 **180 秒**，查询耗时也计入预算；只有结构化 E404 会按 2 秒起步、最多 15 秒的退避间隔重试。这个可见性等待窗口内，每次只读查询的超时不超过 30 秒及剩余预算，超时会终止该查询。认证、网络、取消、异常响应及 integrity 不一致均立即失败，不进入可见性重试。超过期限仍未可见也失败，保留原候选供恢复。

`dist-tag` 返回成功后，标签查询也可能暂时仍返回旧值。提升后的复核和独立 `verify-latest` 使用相同的每包 **180 秒**预算，同时检查 `latest` 与目标版本的 integrity；这两个查询共享期限，每次启动前都重算剩余时间。旧的稳定版本、缺失的 `latest` 或目标版本 metadata 的结构化 E404 可以等待；更高的 `latest`、非法版本、integrity 冲突及查询错误立即失败，绝不回退新版本。等待只重复读取，不重新执行 `dist-tag`。查询与退避同样计入预算，过期结果不能报告成功。

发行顺序为四个平台包 → DSH 插件 → meta，先发布到 `candidate-X-Y-Z`，不立即移动 `latest`。全部精确版本与 integrity 验证通过后，从公开 registry 运行实际安装冒烟；再按相同顺序将 `latest` 提升到此版本，最后复核六包。npm 没有六包 dist-tag 原子事务；入口包最后更新降低不完整依赖对新安装的影响。

CI 的安装测试使用隔离用户目录和 DSH 注册协议桩。它验证包获取、CLI、worker、skills、配置和安装契约，不代表用户 Mac 桌面、真实账号或付费模型调用已通过。

## 3. 失败恢复

- **0.27.0 的发行记录：**[首次发行 run](https://github.com/ckanner/flywheel/actions/runs/41385226607) 的双平台验证均通过，但 publish job 在上传前的远端 `git fetch` 因私有仓库认证缺失失败；六包均未上传。私有仓库发行修复随后进入 0.27.1，0.27.0 tag 保留不移动。
- **0.27.1 的发行记录：**[第二次发行 run](https://github.com/ckanner/flywheel/actions/runs/41387321980) 的 macOS 验证通过，但 Ubuntu 验证因控制面测试夹具直接覆写状态文件产生半写 JSON 而失败；publish job 跳过，六包均未上传。夹具原子发布和 npm 12 打包协议修复进入 0.27.2，0.27.1 tag 保留不移动。
- **0.27.2 的发行记录：**[PR #19 CI](https://github.com/ckanner/flywheel/actions/runs/41388914605) 与 [main CI](https://github.com/ckanner/flywheel/actions/runs/41389529756) 均通过。随后预核真实 npm 12 的 `view --json` 发现单值结果变为单元素数组，[第三次发行 run](https://github.com/ckanner/flywheel/actions/runs/41389716920)在 preflight／build 成功后的 verify 阶段主动取消；两个 verify job 与 publish 均取消，没有 npm 上传。这是预防性取消，不是 CI 失败。查询协议修复和一致工具链使用新的统一版本 **0.27.3**，原 tag 不移动；不要重新触发旧 tag 来代替修正版，0.27.3 的发行结果见下一条。
- **0.27.3 的发行记录：**[发行 run 的第 8 次尝试](https://github.com/ckanner/flywheel/actions/runs/41391651488/attempts/8)成功，六包精确 integrity、公开 registry 安装冒烟、`latest` 提升及最终复核均通过。此前遇到上传后精确版本暂不可见、标签修改后仍读到旧值的失败，始终复用同一 run 的 candidate artifact 恢复，没有重建同版本字节或移动 `v0.27.3` tag。额外从公开 registry 验证了精确版本安装，以及 `npx -y @ckanner/flywheel@latest install` 安装后实际运行 native CLI 和安装 10 个 skills；这些检查使用隔离 profile 与 DSH 协议桩，没有调用模型，不代表真实 Mac 桌面与账号已验证。随后补充的有界可见性等待仅修改后续发行工具，不替换已发布的包。
- **认证失败：**检查日志显示的可用路径、npm 包 owner、对应 repository／workflow 及 publish／dist-tag 两项授权。首次新包不能仅补已有包的 trust。不要打印、复制到 issue 或提交发布凭证。
- **部分包已发布或可见性等待超时：**保留同一 run 的 candidate artifact。在精确版本已可查询或外部故障修复后使用 **Re-run failed jobs**；脚本先核对已公开版本的 integrity，与原 artifact 一致才跳过该包，继续剩余包。不要重新运行构建来生成另一批同版本字节，也不要把查询暂未可见直接判断为上传失败或 staged publishing。
- **安装或提升失败：**此时部分精确版本可能已公开，但 `latest` 尚未全部移动。修复原因后重试原候选；脚本会复核已完成步骤，拒绝覆盖不同 integrity 或回退更新的 latest。
- **需要改源码：**已公开版本不可重用。先检查哪些包已存在，再安排新的统一版本；不要移动已有 tag 或强行忽略 integrity 检查。原 artifact 丢失也不能凭重新构建推定字节相同。

发行完成以 publish workflow 成功，或维护者明确授权且如实记录的手动发行门禁，加上六包 registry 校验和实际安装通过为依据。可分别检查 `npm view <包名>@X.Y.Z dist.integrity` 和 `npm view <包名> dist-tags.latest`。需要从干净测试目录重复公开安装冒烟时，在对应源码运行：

```sh
FLYWHEEL_NPM_VERSION=X.Y.Z npm run test:registry
```

用户入口仍是 `npx -y @ckanner/flywheel@latest install`。GitHub Release 页面存在、源码合并或本地 `npm pack` 成功，都不能单独证明这条命令已经能安装新版本。

## 4. CI 无法启动时的授权手动发行

维护者可以明确授权以手动运行替代不可用的 Actions 编排。本次 0.28.0 的主分支 [CI](https://github.com/ckanner/flywheel/actions/runs/41825311905) 因账号付款／支出限制没有启动 job；这不是测试失败，也不能记录为 CI 通过。手动发行须保留授权、精确源码、工具链、真实测试结果和未执行检查的说明，不修改正常 workflow 或伪造绿色状态。

先合入本次全部实现与文档，再在最终 clean HEAD 创建精确 `vX.Y.Z` tag。通过已认证 Git 或 GitHub API 新鲜核对远端 tag；本地缓存不能代替远端事实。按英文 [manual runbook](en/releasing.md#authorized-manual-release-when-actions-cannot-run) 运行与 CI 相同的 Go quality/race、插件、跨进程、npm 和真实六包安装门禁。只在 Linux 执行时，记录 Linux 结果；macOS 交叉编译不代表 macOS runtime、桌面、账号和付费模型已验证。

发行仍须先运行 `release-preflight.js`，再以相同 source SHA 检查 `candidate.json`。`release-publish.js` 本身不检查 Git tag；手动运行不能遗漏外层 preflight。不要使用此前带 `source.dirty: true` 的开发候选。构建一次后保留原六个 tgz 和 candidate，上传前同时绑定当前 commit、tree、snapshot 与包的 SHA-512；源码变化须在尚未上传时重新准备。

手动环境使用 npm 正常登录或已授权的发布凭证；GitHub 登录不替代 npm 权限，Actions OIDC 也不能在本地仿冒。普通浏览器登录命令为 `npm login --auth-type=web --registry=https://registry.npmjs.org/`。会话若要求发布或 dist-tag 的浏览器／2FA 确认，应在真实 TTY 完成；不关闭 2FA、不修改包安全设置。

手动流程先设置 `export npm_config_registry=https://registry.npmjs.org/`，替代 CI setup-node 的 registry 配置；确认没有包 scope 指向其他 registry，不能继承安装测试的隔离 registry 或私有镜像用于发行写入。

当前标准发行 runner 捕获 npm 输出，不把 TTY 传给 npm。需要交互认证时，可使用仓库外人工 runner：继续调用现有 `publish`／`promote`／`verifyRemote`／`verifyLatest`，仅给 `npm publish` 与 `npm dist-tag` 写操作继承 TTY，查询继续捕获输出并使用严格 JSON 解析。保留原 timeout、killSignal、输出上限和所有六包预检；runner 自身先执行 preflight 并核对完整来源，写操作要求 TTY。不得把这层交互适配变成绕过候选、完整性或防降级检查的逐包裸上传。

先上传 `candidate-X-Y-Z`，全部精确版本 integrity 匹配后运行 `FLYWHEEL_NPM_VERSION=X.Y.Z npm run test:registry`；通过后才按平台包 → 插件 → meta 提升 latest，并执行 `verify-latest`。发布和提升前分别刷新远端 tag。部分成功时复用原候选恢复，认证或浏览器审批未完成不能记录为已发布。保存的发行记录必须区分“代码已合并”“候选已构建”“npm 已上传”“latest 已提升”和未完成的实机检查。
