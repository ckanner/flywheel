// Package skills 持有仓库全部 skill 目录的 go:embed 数据（templates/ 单一信源
// 约定的同款做法）。声明必须放在本目录：go:embed 不能引用父目录，而
// internal/skillsembed 的解析层要消费这份 FS。
// `all:*` 连同未来新增的附属资源一起嵌入；embed.go 自身也会进 FS，
// 消费方一律按 "skills/<name>/SKILL.md" 过滤，不受影响。
package skills

import "embed"

//go:embed all:*
var FS embed.FS
