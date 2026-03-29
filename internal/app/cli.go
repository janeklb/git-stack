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

	var root *cobra.Command
	root = &cobra.Command{
		Use:           use,
		Short:         "stacked PR tool",
		Long:          "stack is a stacked PR tool. Equivalent git extension form: git stack <command>",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd == root {
				return nil
			}
			if cmd.Name() == "help" || strings.HasPrefix(cmd.Name(), "__complete") {
				return nil
			}
			for current := cmd; current != nil; current = current.Parent() {
				if current.Name() == "completion" {
					return nil
				}
			}
			return ensureSupportedCloneLayout()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = false

	var initTrunk string
	var initMode string
	var initTemplate string
	var initPrefixIndex bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize stack state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdInit(initTrunk, initMode, initTemplate, initPrefixIndex)
		},
	}
	initCmd.Flags().StringVar(&initTrunk, "trunk", "", "trunk branch")
	initCmd.Flags().StringVar(&initMode, "mode", defaultRestackMode, "restack mode: rebase or merge")
	initCmd.Flags().StringVar(&initTemplate, "template", "{slug}", "branch naming template")
	initCmd.Flags().BoolVar(&initPrefixIndex, "prefix-index", false, "prefix generated name with incrementing index")
	root.AddCommand(initCmd)

	var newParent string
	var newTemplate string
	var newPrefixIndex bool
	newCmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create a new branch in stack",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return a.cmdNew(name, newParent, newTemplate, newPrefixIndex)
		},
	}
	newCmd.Flags().StringVar(&newParent, "parent", "", "parent branch")
	newCmd.Flags().StringVar(&newTemplate, "template", "", "override naming template")
	newCmd.Flags().BoolVar(&newPrefixIndex, "prefix-index", false, "prefix generated name with incrementing index")
	root.AddCommand(newCmd)

	var statusAll bool
	var statusDrift bool
	var statusNoColor bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show stack graph and state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdStatus(statusAll, statusDrift, statusNoColor)
		},
	}
	statusCmd.Flags().BoolVar(&statusAll, "all", false, "show all stacks")
	statusCmd.Flags().BoolVar(&statusDrift, "drift", false, "include drift markers")
	statusCmd.Flags().BoolVar(&statusNoColor, "no-color", false, "disable ANSI colors")
	root.AddCommand(statusCmd)

	var restackMode string
	var restackContinue bool
	var restackAbort bool
	restackCmd := &cobra.Command{
		Use:   "restack",
		Short: "Restack branches onto their parents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdRestack(restackMode, restackContinue, restackAbort)
		},
	}
	restackCmd.Flags().StringVar(&restackMode, "mode", "", "restack mode override")
	restackCmd.Flags().BoolVar(&restackContinue, "continue", false, "continue restack after conflicts")
	restackCmd.Flags().BoolVar(&restackAbort, "abort", false, "abort in-progress restack")
	root.AddCommand(restackCmd)

	var submitAll bool
	var submitYes bool
	submitCmd := &cobra.Command{
		Use:   "submit [branch]",
		Short: "Push branches and create/update PRs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			return a.cmdSubmit(submitAll, submitYes, branch)
		},
	}
	submitCmd.Flags().BoolVar(&submitAll, "all", false, "submit all stack branches")
	submitCmd.Flags().BoolVarP(&submitYes, "yes", "y", false, "auto-confirm merged local branch cleanup when remote branch is deleted")
	root.AddCommand(submitCmd)

	var reparentParent string
	reparentCmd := &cobra.Command{
		Use:   "reparent <branch>",
		Short: "Change the parent branch for a stack branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdReparent(args[0], reparentParent)
		},
	}
	reparentCmd.Flags().StringVar(&reparentParent, "parent", "", "new parent branch")
	_ = reparentCmd.MarkFlagRequired("parent")
	root.AddCommand(reparentCmd)

	repairCmd := &cobra.Command{
		Use:   "repair",
		Short: "Rebuild local stack metadata from git ancestry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdRepair()
		},
	}
	root.AddCommand(repairCmd)

	return root
}
