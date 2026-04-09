package app

import (
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) Run(args []string, invocation string) int {
	root := a.newRootCmd(invocation)
	root.SetIn(a.in)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		a.printferrln("error: %v", err)
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
		Short: "Initialize or repair stack state",
		Long:  "Initialize or repair persisted stack state. This is a migration/repair command; normal mutating workflows should auto-bootstrap state when possible.",
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
	var newAdopt bool
	newCmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create or adopt a branch in stack",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return a.cmdNew(name, newParent, newTemplate, newPrefixIndex, newAdopt)
		},
	}
	newCmd.Flags().StringVar(&newParent, "parent", "", "parent branch")
	newCmd.Flags().StringVar(&newTemplate, "template", "", "override naming template")
	newCmd.Flags().BoolVar(&newPrefixIndex, "prefix-index", false, "prefix generated name with incrementing index")
	newCmd.Flags().BoolVar(&newAdopt, "adopt", false, "track the current existing branch instead of creating a new one")
	_ = newCmd.RegisterFlagCompletionFunc("parent", completeBranchRefs(true))
	root.AddCommand(newCmd)

	var statusAll bool
	var statusDrift bool
	var statusNoColor bool
	statusCmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"stat"},
		Short:   "Show stack graph and state",
		Args:    cobra.NoArgs,
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
	var submitNextOnCleanup string
	submitCmd := &cobra.Command{
		Use:   "submit [branch]",
		Short: "Push branches and create/update PRs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			return a.cmdSubmit(submitAll, submitNextOnCleanup, branch)
		},
	}
	submitCmd.Flags().BoolVar(&submitAll, "all", false, "submit all stack branches")
	submitCmd.Flags().StringVar(&submitNextOnCleanup, "next-on-cleanup", "", "switch to this local branch if submit cleans up the current merged branch")
	submitCmd.ValidArgsFunction = completeSingleBranchArg(false)
	_ = submitCmd.RegisterFlagCompletionFunc("next-on-cleanup", completeBranchRefs(false))
	root.AddCommand(submitCmd)

	var reparentParent string
	var reparentPreserveLineage bool
	reparentCmd := &cobra.Command{
		Use:   "reparent <branch>",
		Short: "Change the parent branch for a stack branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdReparent(args[0], reparentParent, reparentPreserveLineage)
		},
	}
	reparentCmd.Flags().StringVar(&reparentParent, "parent", "", "new parent branch")
	reparentCmd.Flags().BoolVar(&reparentPreserveLineage, "preserve-lineage", false, "keep the existing lineage parent")
	reparentCmd.ValidArgsFunction = completeSingleBranchArg(false)
	_ = reparentCmd.RegisterFlagCompletionFunc("parent", completeBranchRefs(true))
	_ = reparentCmd.MarkFlagRequired("parent")
	root.AddCommand(reparentCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose local stack metadata health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdDoctor()
		},
	}
	root.AddCommand(doctorCmd)

	var advanceNext string
	advanceCmd := &cobra.Command{
		Use:   "advance",
		Short: "Advance after the current branch merges",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdAdvance(advanceNext)
		},
	}
	advanceCmd.Flags().StringVar(&advanceNext, "next", "", "checkout target after advancing a merged branch")
	_ = advanceCmd.RegisterFlagCompletionFunc("next", completeBranchRefs(false))
	root.AddCommand(advanceCmd)

	var cleanupYes bool
	var cleanupAll bool
	var cleanupIncludeSquash bool
	var cleanupUntracked bool
	cleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete merged local branches and reconcile stack state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdCleanup(cleanupYes, cleanupAll, cleanupIncludeSquash, cleanupUntracked)
		},
	}
	cleanupCmd.Flags().BoolVar(&cleanupYes, "yes", false, "apply without confirmation prompt")
	cleanupCmd.Flags().BoolVar(&cleanupAll, "all", false, "clean all tracked branches instead of only the current stack")
	cleanupCmd.Flags().BoolVar(&cleanupIncludeSquash, "include-squash", false, "allow cleanup for squash-integrated branches")
	cleanupCmd.Flags().BoolVar(&cleanupUntracked, "untracked", false, "also clean eligible untracked local branches")
	root.AddCommand(cleanupCmd)

	return root
}
