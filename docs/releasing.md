# npm 发行指南

[English](en/releasing.md) · 简体中文

本页面向维护者。普通用户从 npm 安装，见[入门教程](getting-started.md)；不需要下载 Actions artifact 或手工解压 tgz。

当前流程控制台的统一发行目标为 **0.30.0**，源码验证见 [控制台记录](validation/2026-11-09-flow-console.md)。同一个正式候选必须从干净的 `v0.30.0` 构建、通过 macOS／Linux 安装，再上传 npm；开发候选不能直接发布。会话存储升级为 v2 后，旧引擎会拒绝继续，新引擎仍可读取旧 v1。

历史会话视图排序版本 **0.28.5** 的复核见 [0.28.5 记录](validation/2026-11-05-view-order.md)。该记录不代替 npm 上传、latest 提升或桌面验收。0.28.5 起包含交互发行工具，可从自身的干净精确 tag 构建并运行 `npm run release:local -- --repository-visibility private`，无需沿用下文 0.28.0 的双 checkout 示例。

自适应工作流和会话 P1–P4 的统一发行目标为 **0.29.1**。当前候选 bundle 已审查的 Codex Pro 0.9.0 公共 npm 包，lock 指向真实 vendor tarball 和实际 integrity；安装门禁确认不会向 registry 请求尚未发布的 Codex 版本。来源与许可证见 [vendor 记录](../plugins/dsh/vendor/README.md)。Claude 0.4.0 保持 optional peer，CI 从固定源码 SHA 构建并检查相同 tarball hash，再运行显式后端门禁；默认用户不选择它就无需安装。

P4 的 `npm run test:sessions` 使用真实 Go 与公开 adapter、离线 native fixture；CI 在两种 OS 执行全部配对场景。该门禁不提供真实模型效果或账号就绪证明。

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

包含原生 Conversation View 的新源码在隔离快照内通过 `npm ci` 安装插件锁文件中的开发和可选依赖，禁用 lifecycle scripts，再编译 CSS Modules；不复用开发目录的 `node_modules` 或 `dist`。这些构建依赖不会变成已安装插件的运行时依赖。

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

标准 CI runner 保持捕获 npm 输出。需要浏览器／2FA 交互时，使用仓库内 `npm run release:local`，见下一节。它继续调用现有发行函数，仅给 `npm publish` 与 `npm dist-tag` 写操作继承 TTY，查询继续捕获输出并使用严格 JSON 解析；保留原 timeout、killSignal、输出上限和所有六包预检。

先上传 `candidate-X-Y-Z`，全部精确版本 integrity 匹配后运行 `FLYWHEEL_NPM_VERSION=X.Y.Z npm run test:registry`；通过后才按平台包 → 插件 → meta 提升 latest，并执行 `verify-latest`。发布和提升前分别刷新远端 tag。部分成功时复用原候选恢复，认证或浏览器审批未完成不能记录为已发布。保存的发行记录必须区分“代码已合并”“候选已构建”“npm 已上传”“latest 已提升”和未完成的实机检查。

## 5. 从源码本地构建并发布

预构建归档是复用已测试产物的一种交付方式，不是本地发行的前置条件。维护者可以在自己的 Mac/Linux 上拉取源码、构建并发布。需要 Git、Go 1.27（本次验证使用 1.27.3）、Node 24.22.1、npm 12.2.1，以及正常的 GitHub 读取和 npm 发布权限。完整源码质量门禁仍按上一节执行并记录；本地发布命令负责候选安装及 registry 安装门禁，不替代 Go quality/race 或源码评审。

本次 `v0.28.0` 已固定在 `c865d2c130aa009f2348a883c01d0ac29da59624`。后补的交互发布工具保存在最新 `main`，因此使用两个 Git 工作目录：最新 checkout 提供维护工具，独立 worktree 保留准确的发行源码。它们都来自 Git；不移动既有 tag，不把新维护工具误打入旧版本。下面命令从最新 `main` 的仓库根目录执行，任一步失败即停止：

```sh
export npm_config_registry=https://registry.npmjs.org/
git fetch origin --tags
git worktree add --detach ../flywheel-v0.28.0 c865d2c130aa009f2348a883c01d0ac29da59624
node ../flywheel-v0.28.0/scripts/release-preflight.js --version 0.28.0 --tag v0.28.0 --json
npm --prefix ../flywheel-v0.28.0 run build:npm
npm login --auth-type=web --scope=@ckanner --registry=https://registry.npmjs.org/
npm whoami --registry=https://registry.npmjs.org/
npm run release:local -- --source-directory ../flywheel-v0.28.0 --repository-visibility private
```

候选输出在 `../flywheel-v0.28.0/dist-npm/`。无需在源码根目录先执行 `npm install`；构建脚本自行构建 worker 和四个平台的 Go 二进制。`--source-directory` 默认是工具所在仓库；`--directory` 可指定另一处候选目录，显式相对路径均相对于调用目录解析。不论工具与源码是否同一 checkout，源码必须干净、版本相同，且本地／远端 release tag、HEAD、tree、snapshot 和候选完整性全部吻合。

`release:local` 自动按以下顺序运行：

1. 核对完整来源与候选，通过源仓库 `origin` 的 `git ls-remote` 新鲜核对远端 tag。
2. 在本机运行候选 `test:install`；再次检查源码、候选和 tag 未变化。
3. 按平台包 → DSH 插件 → meta 上传到 `candidate-X-Y-Z`，沿用既有版本 integrity 和防倒退检查。
4. 从官方 npm registry 实际安装精确版本；失败时不提升 `latest`。
5. 再次核对来源、候选和远端 tag，再提升并复核六包 `latest`，最后打印完成结果。

请使用真实终端，按 npm 正常浏览器／2FA 提示完成认证，不把发布输出通过管道传给 `tee`。这条命令不会登录账号、创建／移动 tag、改变仓库可见性或修改 npm 安全设置。`private` 对应当前仓库可见性；公开源码的发布仍须使用 `public` 并满足 provenance 要求。

若中途中断，保留源 worktree 和同一 `dist-npm/`，解决失败原因后重跑最后一条命令即可。已有版本只有 integrity 完全一致才会跳过；不要重新运行构建去覆盖可能已上传的同版包。若本地刚构建的候选与已经发布的同版本内容冲突，停止并使用原候选，或按新的统一版本安排发行。

测试在隔离用户目录中安装并使用 DSH 协议桩，不调用模型。Mac 上运行会验证本机架构的 Darwin CLI/worker；真实 DSH 桌面、账号和模型推理仍须单独验收。完成后将源 SHA、候选清单、实际平台与测试结果记录到 GitHub Release，再更新发行状态。
