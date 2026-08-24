// skills 子命令：内置 skill 目录的查看接口（数据层 internal/skillsembed）。
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/skillsembed"
)

func skillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "查看内置 skills（list / read）",
	}
	cmd.AddCommand(skillsListCmd(), skillsReadCmd())
	return cmd
}

func skillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出全部内置 skills",
		Run: func(cmd *cobra.Command, _ []string) {
			out := cmd.OutOrStdout()
			for _, s := range skillsembed.List() {
				fmt.Fprintf(out, "%s\n  %s\n", s.Name, s.Description)
			}
		},
	}
}

func skillsReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <name>",
		Short: "打印指定 skill 的 SKILL.md 全文",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := skillsembed.Read(args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), raw)
			return nil
		},
	}
}
