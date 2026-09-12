// flywheel template 命令：打印内嵌契约模板原文（v0.22 分发定稿）。任务生成
// skill（flywheel-new）经它取 spec/packet/config 的格式范本——不依赖源码仓库，
// 二进制走到哪模板跟到哪（templates.FS go:embed）。
package cli

import (
	"fmt"
	"io/fs"

	"github.com/ckanner/flywheel/templates"
	"github.com/spf13/cobra"
)

func templateCmd() *cobra.Command {
	var list bool
	cmd := &cobra.Command{
		Use:   "template <name>",
		Short: "打印内嵌契约模板原文（--list 列出全部：spec/packet 示例、法定配置表、契约 schema）",
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				// embed FS 的 ReadDir 恒成功（覆盖率纪律：不可达分支不写）。
				entries, _ := fs.ReadDir(templates.FS, ".")
				for _, e := range entries {
					fmt.Fprintln(cmd.OutOrStdout(), e.Name())
				}
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("用法: flywheel template <name>（flywheel template --list 列出全部）")
			}
			raw, err := templates.FS.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("未知模板 %q（flywheel template --list 查看全部）", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), string(raw))
			return nil
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "列出全部内嵌模板名")
	return cmd
}
