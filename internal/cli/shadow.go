package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Keep a migration error for scripts using the retired command. There is no
// shadow collector or separate network client behind this compatibility entry.
func shadowCmd() *cobra.Command {
	return &cobra.Command{Use: "shadow", Hidden: true, DisableFlagParsing: true,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("shadow evaluation has been retired; configure jev.enabled for live decisions, or use decision replay|evaluate for recorded decision inputs")
		}}
}
