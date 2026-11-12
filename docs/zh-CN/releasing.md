# Flywheel 发行手册

[文档目录](../README.zh-CN.md) · [English](../en/releasing.md) · 简体中文

本文面向维护者，用户安装请看[快速开始](getting-started.md)。源码提交、GitHub Release 或 `npm pack` 成功，都不代表新版本已能从 npm 安装。实际发行结果和验收边界应保存在[验收记录](../validation/README.md)。

## 1. 发行身份与前提

一次发行包含六个精确同版本包，按以下顺序发布：

| 顺序 | 包 | 内容 |
| --- | --- | --- |
| 1–4 | `@ckanner/flywheel-{darwin,linux}-{arm64,amd64}` | 四个平台的 Go 二进制 |
| 5 | `@ckanner/dsh-plugin-flywheel` | 桌面插件和 worker |
| 6 | `@ckanner/flywheel` | 用户入口 CLI、安装器和文档 |

入口包精确固定其余五包版本。每个 Flywheel 包声明 MIT 并包含 `LICENSE`，插件保留第三方许可声明。Codex Pro 固定为 0.9.0，以[已记录来源的候选](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)内置分发；Claude Code Pro 0.4.0 保持可选 peer，不是第七个 Flywheel 发行包。

使用与仓库 workflow 一致的 Git、**Go 1.27、Node 24.22.1、npm 12.2.1**。用户安装的 Node 要求更低，发行工具链则固定版本。源码须经过审查、工作区干净、六包版本一致，且精确的 `vX.Y.Z` tag 指向 HEAD。不得移动已有发行 tag，也不得替换已发布版本的字节。

`candidate.json` 保存源码 commit、tree、快照 SHA-256，以及每个 tarball 的 SHA-512 integrity。构建一次后，验证、测试和发布同一批 tarball。带 `source.dirty: true` 的候选只用于开发。

## 2. 验证发行源码

按[贡献指南](contributing.md)和当前 [CI workflow](https://github.com/ckanner/flywheel/blob/main/.github/workflows/ci.yml)执行门禁：

| 范围 | 必须记录的证据 |
| --- | --- |
| Go | `make quality-gate` 和 workflow 中核心包的 race 检查 |
| DSH 插件 | 锁文件依赖安装、构建和行为测试 |
| 后端边界 | DSH、Codex Pro、Claude Code Pro 离线跨进程测试 |
| npm | `npm test` 与六包候选的真实安装 |
| 原生 CLI | 构建、版本／帮助和 JSON 配置检查 |
| 会话策略 | 使用已构建原生二进制运行 `npm run test:sessions` |
| 文档 | 链接、双语对应、包内指南和许可证 |

按 workflow 固定的源码 SHA 准备可选 Claude tarball，并检查 SHA-256。插件／桥接测试阶段临时将它安装为开发依赖，随后恢复源码 manifests；候选安装测试前，通过 `export` 将 `FLYWHEEL_CLAUDE_CODE_PRO_TARBALL` 设置为该已核对 tarball 的绝对路径。缺少可选 peer 不代表 Claude 后端验证通过。

这些门禁使用隔离 profile 和离线模型夹具，不证明真实账号、模型质量或实际桌面布局可用。记录真实执行的平台；交叉编译不能代替目标平台运行验收。

## 3. 构建同一批候选

从经过审查、工作区干净且位于发行 tag 的 checkout 执行，每步失败即停止：

```sh
FLYWHEEL_RELEASE_VERSION=$(node -p "require('./package.json').version")
FLYWHEEL_RELEASE_SHA=$(git rev-parse HEAD)
node scripts/release-preflight.js --version "$FLYWHEEL_RELEASE_VERSION" --tag "v$FLYWHEEL_RELEASE_VERSION" --json
npm run build:npm
node scripts/release-publish.js verify-candidate --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

运行最后一条命令前，先设置上一节说明的 Claude tarball 环境变量。构建器在隔离源码快照中安装锁定的插件依赖、禁用 lifecycle scripts，再构建 worker 和四个平台 Go 二进制，不复用调用方的 `node_modules` 或 `dist`。无需在仓库根目录先执行 `npm install`。

保存 `dist-npm/`、`candidate.json`、源码身份、工具链版本和验证结果。核对六包 MIT 许可证及根入口、插件的双语文档。尚未上传时源码变化，应从新审查源码重新准备候选；已有任何上传后，保留原候选并按失败恢复规则处理。

## 4. 通过 GitHub Actions 发布

推送经过审查的 tag 会触发 [publish.yml](https://github.com/ckanner/flywheel/blob/main/.github/workflows/publish.yml)，也可在手动运行入口提供一个已有 tag。Linux 和 macOS 验证同一个候选 artifact；发布 job 刷新远端引用，并要求 tag 仍指向已验证源码。

GitHub 访问权和 npm 发布权是两项授权。六个包分别配置 trusted publisher：owner 为 `ckanner`、repository 为 `flywheel`、workflow 为 `publish.yml`；除非 workflow 改为使用 environment，否则该项留空。需要同时允许 **npm publish** 和 **npm dist-tag**。workflow 也支持已授权的 `NPM_TOKEN` 后备路径。

首次新建包需要先完成 npm 的包创建授权，才能配置该包的 trusted publisher。使用正常授权流程，不得以不同占位内容提前占用发行版本。账号设置参考 npm 官方 [trusted publisher](https://docs.npmjs.com/trusted-publishers/) 和 [dist-tag](https://docs.npmjs.com/adding-dist-tags-to-packages/) 文档；`npm whoami` 只能说明登录身份，不能证明发布权限。

发布必须明确仓库实际可见性：`public` 要求 provenance，`private` 关闭不支持的私有源码 provenance。发 npm 包不会改变仓库可见性，OIDC 身份认证与 provenance 也是不同能力。

## 5. 从私有源码本地发布

维护者可从普通 Git checkout 构建。按当前发行政策，公开仓库的新上传必须走第 4 节：发布器要求 provenance，而 npm 只能在支持的云 CI/CD 环境生成它，普通本地浏览器登录不能提供。详见 [npm provenance 前提](https://docs.npmjs.com/generating-provenance-statements/)。下面的本地上传示例仅适用于源码仓库实际为私有的情况，不得把公开仓库声明为私有来绕过要求。无需下载候选压缩包。先完成源码审查和上述门禁。Actions 无法运行时，记录维护者授权、本地真实结果和未执行检查，不能把不可用的 CI 写成通过。

在相同的干净 tag 源码及其原候选目录执行：

```sh
npm login --auth-type=web --scope=@ckanner --registry=https://registry.npmjs.org/
npm run release:local -- --repository-visibility private
```

在终端完成正常浏览器／2FA 提示；发布器要求 TTY，不要将输出通过管道传给 `tee`。

`release:local` 按顺序执行：

1. 核对干净源码、精确本地／远端 tag、tree、snapshot、候选清单和包完整性。
2. 运行候选安装测试，再次确认源码和候选未变化。
3. 按平台包 → 插件 → 入口包上传到 `candidate-X-Y-Z`。
4. 从官方 npm registry 安装精确版本。
5. 再次核对源码和远端 tag，按相同顺序提升 `latest`，验证六包标签和 integrity。

该命令复用既有发行检查，仅让 npm publish 和 dist-tag 写操作继承终端 I/O；registry 查询保留严格 JSON 解析和有界超时。命令不负责登录、创建／移动 tag 或修改账号安全设置。

已审查的旧发行若确实需要新工具，可通过 `--source-directory` 指向旧版精确 tag worktree，通过 `--directory` 指向原候选。源码默认是工具所在 checkout，候选默认是选定源码目录下的 `dist-npm/`；显式相对路径从调用目录解析。不要将新工具复制进旧发行源码，也不要重建已经发布的同版本。

## 6. 可见性、标签提升与重试

所有包先使用 candidate 标签。精确版本和 integrity 均可查询、registry 安装通过后，才提升 `latest`；顺序为平台包、插件、最后用户入口包。npm 不提供六包标签的原子事务。

只有结构化 E404 表示所查询版本不存在。认证、网络、取消、异常输出和缺失 integrity 都应失败。查询接受已支持的标量或单元素数组，多结果拒绝。

上传后每个包的可见性检查预算为 180 秒，E404 从 2 秒起步退避，最大 15 秒；每次读取上限为 30 秒及剩余预算中的较小值。标签提升后，用相同的有界方式复核 `latest` 与 integrity，只重复读取，不重复写入。发现更高的 `latest`、integrity 冲突或非法版本时拒绝继续。

| 故障 | 恢复方法 |
| --- | --- |
| 认证或浏览器审批未完成 | 处理正常 npm 授权，保留候选。 |
| 部分包已上传 | 重试原候选；已有版本必须与其 integrity 一致。 |
| registry 可见性超时 | 保存 artifact，外部问题解决后重试。 |
| 安装或标签提升失败 | 继续使用同一候选，不能回退更新的 `latest`。 |
| 上传后需要修改源码 | 选择新的统一版本和经过审查的新 tag。 |
| 原候选丢失 | 找回原候选，不能假定重建字节相同。 |

Actions 使用原 artifact 重跑失败 job；本地使用保留的源码和候选重跑最后一条 `release:local` 命令。

## 7. 记录完成状态

只有六个精确版本和 integrity、registry 安装及最终 `latest` 复核全部通过，才能记录发行完成。源码审查、本地测试、CI 和桌面验收分别说明。需要额外检查精确版本安装时：

```sh
FLYWHEEL_NPM_VERSION=X.Y.Z npm run test:registry
```

将 `X.Y.Z` 替换为已发布版本。在带日期的验收记录和 release notes 中保存 source SHA、候选身份、实际平台、失败／重试和剩余限制。[0.30.2 发行记录](../validation/2026-11-11-release-0.30.2.md)是历史示例，只证明该次发行，不代表当前 registry 的实时状态。
