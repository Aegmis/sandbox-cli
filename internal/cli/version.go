package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aegmis/sandbox-cli/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sandbox-cli version and base image tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("sandbox-cli %s (base image: %s)\n", version.Version, version.BaseImage())
			return nil
		},
	}
}
