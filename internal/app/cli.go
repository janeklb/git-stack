package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) Run(args []string, invocation string) int {
	root := a.newRootCmd(invocation)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) newRootCmd(invocation string) *cobra.Command {
	bin := invocation
	if bin == "" {
		bin = "stack"
	}
	use := strings.TrimSpace(bin)
	if use == "" {
		use = "stack"
	}

	root := &cobra.Command{
		Use:           use,
		Short:         "stacked PR tool",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = false

	root.AddCommand(&cobra.Command{
		Use:                "init [--trunk <branch>] [--mode rebase|merge]",
		Short:              "Initialize stack state",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdInit(args)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:                "new <name> [--parent <branch>] [--template <template>] [--prefix-index]",
		Short:              "Create a new branch in stack",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdNew(args)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:                "status [--all] [--drift]",
		Short:              "Show stack graph and state",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdStatus(args)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:                "restack [--mode rebase|merge] [--continue] [--abort]",
		Short:              "Restack branches onto their parents",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdRestack(args)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:                "submit [--all] [branch]",
		Short:              "Push branches and create/update PRs",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdSubmit(args)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:                "reparent <branch> --parent <new-parent>",
		Short:              "Change the parent branch for a stack branch",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdReparent(args)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:                "repair",
		Short:              "Rebuild local stack metadata from git ancestry",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdRepair(args)
		},
	})

	return root
}
