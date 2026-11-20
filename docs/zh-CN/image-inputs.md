# 冻结图片输入

[文档目录](../README.zh-CN.md) · [English](../en/image-inputs.md) · 简体中文

实现需要截图、图表等视觉参考时，在 packet 中声明可选的 `input-images` 字段。
Flywheel 读取指定文件、校验冻结内容，再通过 Claude Code Pro 的公开 machine API
和官方 Agent SDK 发送真正的原生 image 内容块。新默认的 `multimodal` route 选择
Claude。仅在 `context-files` 或普通 prompt 中写文件路径，不会产生图片附件。

## 声明真实文件

将已授权使用的参考图片保存为任务仓库 checkout 中的普通文件，例如
`references/layout.png`。在整个 packet 执行期间保留该文件，包括重试、验收和
尚未应用的结果恢复。计算它的 SHA-256：

```bash
sha256sum references/layout.png
# macOS: shasum -a 256 references/layout.png
```

在 packet frontmatter 中加入下面字段，将校验和占位符替换为上述命令输出的
**真实 64 位小写十六进制值**。路径、校验和和 MIME 都属于冻结契约，沿用正常的
契约准备、审查或修订入口。

```yaml
route: multimodal
input-images:
  version: 1
  images:
    - path: references/layout.png
      sha256: "<references/layout.png 的真实小写 SHA-256>"
      media-type: image/png
```

出现 `input-images` 时必须包含 `version` 和 `images`；每项只包含必需的
`path`、`sha256`、`media-type`。纯文本任务省略整个字段，不能填空数组或 null。
原生图片内容块按数组顺序发送，Execute 的文本 prompt 也列出相同顺序的冻结引用，
便于用“第一张图”表达要求，而不是把文件名冒充视觉输入。

路径使用 `/`，相对于**任务原始仓库 checkout**（`--cwd`），不相对于 packet
worktree 或另行选定的执行 checkout。绝对路径、`..`、未经规范化的路径、反斜杠、
盘符或 URI 前缀、控制字符、`.git` 路径段及重复路径均被拒绝。任何路径段都不能是
符号链接；目录、空文件、FIFO 等特殊文件同样不能用作附件。读取限定在该 checkout
内，即使文件在打开期间改变，也受到读取字节上限约束。

这个字段不会自动导入聊天或剪贴板附件。先将已授权的真实内容放进选定仓库，再声明
引用；不能编造校验和，也不能用文字描述替换无法取得的原图。

## 上限与格式核验

| 边界 | 上限 |
|---|---:|
| 每个 packet 的图片数 | 1–4 张 |
| 每张图片字节数 | 4 MiB |
| 图片合计字节数 | 8 MiB |
| 宽或高 | 8,192 像素 |
| 宽 × 高 | 每张 16,000,000 像素 |
| Claude prompt | 1 MiB |
| Claude 完整 JSONL 请求，含换行 | 16 MiB |

字节上限针对 base64 编码前的真实文件内容。PNG、JPEG、GIF、WebP 分别使用
`image/png`、`image/jpeg`、`image/gif`、`image/webp`。Flywheel 先检查图片头部与
尺寸，再解码图像，并要求实际格式与声明一致。把文本文件改名成 `.png` 或仅改
MIME 不会通过。GIF 解码器验证第一帧图像；需求依赖特定帧或动画内容时，应准备
对应的静态参考图。

worker 再次检查数量、规范 base64 及相同字节上限。完整编码请求还必须满足 16 MiB
边界，其中包含文本的 JSON 转义开销。这个扩大后的帧上限只适用于 Claude，其他
worker 仍保持原 1 MiB 请求边界。必要时先降低分辨率或压缩原文件，再冻结新校验和；
Flywheel 不会静默缩放、重编码、截断或省略图片。

## Execute、Review 与权限

只有 **Execute** 收到这些原生图片内容块。当前实际绑定必须为 `claude-code-pro`，
无论它来自直接 route、profile 还是 execution policy。默认 `multimodal` 已配置
这个绑定；其他 route 明确选择同一受支持 harness 时也可使用。若当前后端或后备
目标不支持图片，就在派发前明确失败，不会改成只发文本。

[Claude 的前提](claude-code-pro.md)继续适用：匹配的配套插件、已有本机登录、
配置了订阅策略时的真实准入，以及明确的 `danger-full-access` 权限。图片字段不会
改变这些选择；订阅预检必须在同一个进程内通过，才会发送包括图片在内的任务消息。

Plan、独立 Review 和 Judge 不会自动收到原图。Review 收到冻结引用、实现和测试
证据，同时被明确要求不能声称原生看过图片。某项视觉要求缺少可复现检查或其他
可核验依据时，应返回 `verification-gap`。代码测试通过本身不能证明视觉还原准确；
需要渲染、截图比较或人工视觉验收时，应写入真实任务契约。

## 冻结证据与安全恢复

派发前，Flywheel 对照 packet 核验 SHA-256 与图像格式；保存 Execute 结果时、
复用验收证据前、应用已记录终态前，都重新检查原始源文件。源文件变更、丢失或损坏
后，旧结果不能继续充当当前验收证据，即使 packet worktree 内还有旧副本。

修改任一图片路径、校验和、媒体类型或数组顺序，都会改变目标／契约身份。
经过正常审查的修订会重建受影响角色的历史，不会悄悄续接看过另一张图片的会话。
模型、思考等级及有效绑定变更仍保留原有的历史隔离规则。

调用 state 和回执只保存版本化 manifest：每张图片的 MIME、SHA-256、字节数，
不保存原始内容或 base64。公开配套插件从实际发送的字节计算同一份 manifest；
普通回执或恢复回执缺少、变更这些指纹时，Flywheel 都会拒绝。原生 CLI 可以在
已授权的会话历史中保存图片。

若存在未决调用，必须**先对账，再读取新源文件或派发下一条 prompt**。公开恢复
只查询原请求、run 与私有 journal，并使用原设置；查询不含图片内容或新 prompt，
原图 manifest 仅在 Go 本地用于校验回执。即使图片变更导致旧输出不可用，已知
usage 仍只计入原调用一次。保留原任务与恢复证据；换模型、换图或选择 fresh
都不能解除未知调用阻断。

## 离线端到端验证

下面的源码回归从 packet 加载和 Go Execute 出发，经过构建后的 worker、固定的
公开配套插件与真实 Agent SDK。fake native CLI 必须实际收到 PNG 内容块，才会
写测试产物和合法 done-report；它同时验证 Go 回执丢失、原调用只读恢复，以及
源图改变后拒绝应用且不重发 prompt：

```bash
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/machine \
  -run '^TestPacketImagesClaudeSDKOfflineBridge$' -count=1
```

需要已构建的 DSH worker 和精确匹配的已审查 Claude 配套候选。该测试不使用
真实账号或模型推理，证明的是传输、进程归属、记账与恢复行为，不代表模型视觉准确性。
