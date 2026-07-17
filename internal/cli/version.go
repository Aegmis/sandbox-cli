package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amitghadge/sandbox-cli/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sandbox version and base image tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("sandbox %s (base image: %s)\n", version.Version, version.BaseImage())
			return nil
		},
	}
}
