// Package build 持有编译期注入的版本信息（goreleaser ldflags 注入）。
package build

var (
	// Version 语义化版本号，由发布流程注入（默认 dev）。
	Version = "dev"
	// Date 构建日期（YYYY-MM-DD），由发布流程注入。
	Date = "unknown"
)
