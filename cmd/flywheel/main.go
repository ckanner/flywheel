// Flywheel — Loop Engineering 编排器（Go 实现，docs/design.md 权威设计）。
// CLI 入口保持极薄：所有逻辑在 internal/cli.Execute()，main 仅做退出码映射，
// 使整个命令树可被单测完整覆盖。
package main

import (
	"os"

	"github.com/ckanner/flywheel/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
