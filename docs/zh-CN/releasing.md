# Flywheel 发行手册

[文档目录](../README.zh-CN.md) · [English](../en/releasing.md) · 简体中文

本文面向维护者；用户安装见[快速开始](getting-started.md)。源码提交、tag 或 pack 成功
不等于 registry 已发布。源码、安装、registry 和原生桌面结果分别记录在[验收记录](../validation/README.md)。

## 1. 一个完整包

从 0.33 起，只发布 **`@ckanner/flywheel`**。一个 tarball 包含：

- `bin/`、`npm/` 中的 CLI 与显式安装器。
- `dist/` 中的 DSH Host、Client 和 worker；Node 模块使用 `.mjs`。
- `native/{darwin,linux}-{arm64,amd64}/flywheel` 中的四个平台 Go 程序。
- `native/manifest.json`：同一版本、平台集合、文件大小和 SHA-256 摘要。
- 引擎内嵌的 Skill／模板、双语文档、MIT 许可证和第三方许可说明。

启动器只选择并核对包内当前平台引擎，不在首次运行时下载二进制，也不借用旧平台或
worker 包。`plugins/dsh` 是 **private 源码／构建／测试区**，不是第二个发行目标。
旧四个平台包及 `@ckanner/dsh-plugin-flywheel` 保留为历史版本；新流程不上传它们的
新版本，也不撤销其已发布字节。

Codex Pro 0.10.2 内置[已核对的源码候选包](https://github.com/ckanner/flywheel/blob/main/plugins/dsh/vendor/README.md)，
根目录与源码构建区锁文件保持该候选的真实 tarball integrity；这不表示配套插件已发行。Claude Code Pro 0.5.3 是外部
可选 peer。Provider CLI 和账号凭据不在包内。普通 npm 安装只放置文件，`install`／
`setup` 才执行用户设置，禁止在 postinstall 隐藏 profile 或凭据修改。

已审查的 Claude 0.5.3 源码包固定在 vendor 记录中。PR CI 核对随源码候选的包身份、SHA-256
和 integrity，这不构成 registry 发行证据。实际发布与提升标签前，必须从公共 registry
取得精确插件并与已审查的 vendor 记录比对，之后才能修改 Flywheel 的 registry 状态。
本地发行路径还会把该已验证 tarball 显式传入安装测试，不能悄悄跳过可选 Claude
用例。插件尚不存在或字节不同都阻止发行。

## 2. 源码与验证

使用与 workflow 相同的 **Go 1.27、Node 24.22.1、npm 12.2.1**。源码须已审查，
checkout 干净，`vX.Y.Z` 精确指向 HEAD。不能移动发行 tag、将 dirty 候选冒称正式
候选，或为已经上传的版本重新构建不同字节。

执行适用的[贡献检查](contributing.md)：

| 范围 | 证据 |
| --- | --- |
| Go 改动 | 完整 quality-gate、逐函数 100% 覆盖率和核心 race |
| DSH／worker | 锁定源码依赖的完整检查及真实 Go→Node 离线桥接 |
| npm／发行 | 根 npm 测试、完整单包构建、真实安装同一个 tarball |
| 原生／会话改动 | 适用的 CLI 检查及 `npm run test:sessions` |
| 迁移 | 全新安装、旧拆包升级、失败／回退／取消与重复 setup |
| 文档 | 双语指南、链接、包内文档和许可证 |

Claude 后端测试使用已审查的精确源码 tarball；optional peer 缺失或跳过不算通过。
安装测试前将 `FLYWHEEL_CLAUDE_CODE_PRO_TARBALL` 指向已核对的 tarball。
源码测试使用隔离 home／profile 和离线模型，不证明真实账号、模型质量或桌面布局。

## 3. 构建一次，测试同一个产物

在最终干净发行 tag checkout 执行，每步失败即停止：

```sh
FLYWHEEL_RELEASE_VERSION=$(node -p "require('./package.json').version")
FLYWHEEL_RELEASE_SHA=$(git rev-parse HEAD)
node scripts/release-preflight.js --version "$FLYWHEEL_RELEASE_VERSION" --tag "v$FLYWHEEL_RELEASE_VERSION" --json
npm run build:npm
node scripts/release-publish.js verify-candidate --directory dist-npm --source-sha "$FLYWHEEL_RELEASE_SHA"
FLYWHEEL_NPM_PACKAGES_DIR=dist-npm npm run test:install
```

构建器获取隔离源码快照，安装根目录及 private 构建区的锁定依赖并禁用 lifecycle scripts，
构建插件和四个平台引擎，然后只 pack 根包一次。不复用调用方的 dist／node_modules。
候选 **format 2** 记录 commit、tree、snapshot、dirty 状态以及唯一 tarball 的 SHA-512；
验证器还打开 tarball 核对全部原生摘要、公开入口、许可证和内置 adapter 身份。
Linux 和 macOS 安装同一个候选，不分别重新打包。

保留 `dist-npm/`、`candidate.json`、工具链身份和结果。上传前源码变化需要新的已审查
候选；上传后必须保留原字节，源码修复使用新版本。

## 4. GitHub Actions 自动发布

推送发行 tag 触发 [publish.yml](https://github.com/ckanner/flywheel/blob/main/.github/workflows/publish.yml)，
也可手动指定精确已有 tag。构建和双 OS 检查通过后，发布 job 再核对源码与 tag。

仅为 **`@ckanner/flywheel`** 配置一个 npm Trusted Publisher：

| 字段 | 值 |
| --- | --- |
| Provider | GitHub Actions |
| Organization/user | `ckanner` |
| Repository | `flywheel` |
| Workflow filename | `publish.yml` |
| Environment | 留空，与 workflow 一致 |
| 权限 | 同时允许 `npm publish` 和 `npm dist-tag` |

发布 job 仅使用 OIDC 和空 npm user config，不退回存储的 `NPM_TOKEN` 或浏览器交互认证。
账号 2FA 策略独立保留。首次建立信任可能需要维护者安全密钥确认，这是一次性配置，
不是每次发版审批。npm 可能将新连接标为待验证，直到首次 CI 真正发布；保存设置或
`npm publish --dry-run` 都不证明自动发布已经成功。参见 [npm 官方说明](https://docs.npmjs.com/trusted-publishers/)。

公开源码发行要求 provenance；私有源码必须明确关闭不支持的 provenance，不能将
公开仓库虚称私有。Actions 被账单或其他 runner 问题阻断时，单独记录，不将本机检查
冒充云端通过。CI 只核对已存在版本成功，也不证明新的 OIDC 写操作已经跑通。

## 5. 发布与恢复

唯一 tarball 先使用 `candidate-X-Y-Z` 标签，确认精确 registry integrity 并完成真实
registry 安装后，再提升 latest，最后复核标签和摘要。只有一次包上传、一次标签提升，
均由 CI 身份认证，不再有六包事务。

只有结构化 E404 表示版本不存在。认证、传输、取消、无效 JSON 或摘要不符都明确失败。
已存在版本必须与候选一致，不得回退更新的 latest。可见性检查仍有 180 秒边界，只重试
读取、不盲目重复上传；npm 接收后仍在验证时，保留原字节，等精确版本可见再继续。

| 失败 | 恢复 |
| --- | --- |
| OIDC 授权 | 核对包、仓库、workflow 和两项允许的动作 |
| 上传中断 | 先读 registry 身份，再继续同一个候选 |
| registry 可见性超时 | 保留已接收字节，重试只读核对 |
| 安装或 promotion 失败 | 继续原候选，保留更高版本的标签 |
| 上传后要改代码 | 新版本、新审查 tag |
| 候选丢失 | 找回原 artifact，不假定重新构建完全一致 |

仅在明确授权且源码仓库**确实为私有**的应急场景，`npm run release:local --
--repository-visibility private` 保留源码、tag、候选与安装检查，并使用正常 npm
交互认证；必须使用 TTY。历史发行使用它自己的工具、精确源码与原 artifact 格式。
不得用新的单包发行器处理历史 format-1 六包候选。

## 6. 用户迁移与验收

入口仍是 `npx -y @ckanner/flywheel@latest install`。从拆包版本迁移前先等待 Flywheel 任务收尾，再退出 DSH。
安装器读取现有依赖，先安装并确认新的完整桌面包，再通过官方 DSH CLI 移除旧
`@ckanner/dsh-plugin-flywheel`，最后确认只剩新身份。保留用户配置、修改过的 skills
和任务历史，不写 `state.json`。

file/git 自定义来源的旧插件在更改桌面 profile 前明确失败，先前已完成的安装步骤仍保留在报告中。迁移出错保留真实局部状态；只在能确认
旧包仍存在、替代包确为本次新增时回退替代包。取消不启动新的回退进程；重试先重新
读取 profile。两种身份仍并存时不能报告完成。显式 `flywheelBinary` 与受管启动记录
保持原校验和优先级；没有记录时安装包可使用自己的 CLI，宿主仍需能找到 Node。

发行记录应分别列出源码、候选、registry、安装及限制。另行核对 CLI、技能、桌面包、
配置和重载后的原生界面。不能为宣称验收完成而中断运行中的任务；必要时明确记录重载待完成。

已有受管启动记录必须与当前包同版。单独更新 DSH 副本后，应运行该版本的 setup
刷新受管安装；旧记录会明确报错，不把新版界面与旧引擎混用。
