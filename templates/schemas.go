// Package templates 持有契约层 JSON Schema（DESIGN.md §5 单一信源）。
// go:embed 把 schema 编进二进制：安装后的 flywheel 无需仓库 checkout 即可做
// 加载期校验（契约层与代码原子同步，AGENTS.md lockstep 硬契约）。
package templates

import "embed"

//go:embed *.json *.md
var FS embed.FS
