# npm 发行指南

本页面向维护者。普通用户从 npm 安装，见[入门教程](getting-started.md)；不需要下载 Actions artifact 或手工解压 tgz。

## 1. 发布身份

GitHub Actions 使用 `.github/workflows/publish.yml`，只有 publish job 获得 `id-token: write`；GitHub `contents` 仍为只读。GitHub 登录允许触发有权限的仓库操作，**不会自动授予 npm 包写权限**。

发布工具链固定为 Node **24.22.1**、npm **12.2.1**。后者同时支持 OIDC 的 `npm publish` 和 `npm dist-tag`，不能只检查普通 publish 的最低版本。用户安装仍只要求 Node 22 或更高，不要求 npm 12。

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

preflight 要求 clean HEAD、精确 tag 和六包版本相同。build 从该 commit 的隔离快照打包一次，记录 commit、tree、快照及每个 tgz 的 SHA-512；Ubuntu 与 macOS 下载**同一个** candidate artifact 进行质量门禁和真实 npm 安装。publish 开始前再次检查远端 tag 与源身份。

发行顺序为四个平台包 → DSH 插件 → meta，先发布到 `candidate-X-Y-Z`，不立即移动 `latest`。全部精确版本与 integrity 验证通过后，从公开 registry 运行实际安装冒烟；再按相同顺序将 `latest` 提升到此版本，最后复核六包。npm 没有六包 dist-tag 原子事务；入口包最后更新降低不完整依赖对新安装的影响。

CI 的安装测试使用隔离用户目录和 DSH 注册协议桩。它验证包获取、CLI、worker、skills、配置和安装契约，不代表用户 Mac 桌面、真实账号或付费模型调用已通过。

## 3. 失败恢复

- **认证失败：**检查日志显示的可用路径、npm 包 owner、对应 repository／workflow 及 publish／dist-tag 两项授权。首次新包不能仅补已有包的 trust。不要打印、复制到 issue 或提交发布凭证。
- **部分包已发布：**保留同一 run 的 candidate artifact，在修复外部授权后使用 **Re-run failed jobs**。已有版本必须与原 artifact 的 integrity 一致；不重新构建另一批字节冒充同版本。
- **安装或提升失败：**此时部分精确版本可能已公开，但 `latest` 尚未全部移动。修复原因后重试原候选；脚本会复核已完成步骤，拒绝覆盖不同 integrity 或回退更新的 latest。
- **需要改源码：**已公开版本不可重用。先检查哪些包已存在，再安排新的统一版本；不要移动已有 tag 或强行忽略 integrity 检查。原 artifact 丢失也不能凭重新构建推定字节相同。

发行完成以 publish workflow 成功以及六包 registry 校验为依据。可分别检查 `npm view <包名>@X.Y.Z dist.integrity` 和 `npm view <包名> dist-tags.latest`。需要从干净测试目录重复公开安装冒烟时，在对应源码运行：

```sh
FLYWHEEL_NPM_VERSION=X.Y.Z npm run test:registry
```

用户入口仍是 `npx -y @ckanner/flywheel@latest install`。GitHub Release 页面存在、源码合并或本地 `npm pack` 成功，都不能单独证明这条命令已经能安装新版本。
