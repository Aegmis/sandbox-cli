package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Aegmis/sandbox-cli/internal/worktree"
)

// newWorktreeCmd manages the sandbox-owned git worktrees used by `--worktree`.
func newWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage the git worktrees used for parallel per-branch agents",
		Long: "sandbox-cli --worktree BRANCH runs a sandbox in a git worktree for BRANCH\n" +
			"(created if needed) so several agents can work in parallel, each on its own\n" +
			"branch. These subcommands list and remove those worktrees.",
	}
	cmd.AddCommand(newWorktreeListCmd(), newWorktreeRemoveCmd())
	return cmd
}

func newWorktreeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sandbox-managed worktrees for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			infos, err := worktree.List(wd)
			if err != nil {
				return err
			}
			if len(infos) == 0 {
				fmt.Println("no sandbox worktrees")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "BRANCH\tPATH")
			for _, wt := range infos {
				fmt.Fprintf(tw, "%s\t%s\n", wt.Branch, wt.Path)
			}
			return tw.Flush()
		},
	}
}

func newWorktreeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm BRANCH",
		Short:   "Remove the sandbox worktree for BRANCH",
		Aliases: []string{"remove"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := worktree.Remove(wd, args[0]); err != nil {
				return err
			}
			fmt.Printf("removed worktree for %q\n", args[0])
			return nil
		},
	}
}
