package app

import (
	"strings"

	"github.com/spf13/cobra"
)

const canonicalBinName = "git-stack"

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
		bin = canonicalBinName
	}
	use := strings.TrimSpace(bin)
	if use == "" {
		use = canonicalBinName
	}

	var root *cobra.Command
	root = &cobra.Command{
		Use:   use,
		Short: "Manage personal stacked PR branches",
		Long: `git-stack is an opinionated CLI for personal stacked PR development.

It tracks branch parentage, restacks descendants after history changes, and submits pull requests in stack order. When git-stack is on your PATH, Git also exposes it as git stack <command>.

Mutating commands require a clean worktree. Commands such as new, state, restack, submit, reparent, and clean can infer and persist stack state automatically when the workflow is unambiguous.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd == root {
				return nil
			}
			if cmd.Name() == "help" || cmd.Name() == "version" || strings.HasPrefix(cmd.Name(), "__complete") {
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
			a.writeCompactRootHelp(cmd.OutOrStdout(), cmd)
			return nil
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpFunc(a.helpFunc)
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			a.println(formatVersion())
		},
	})

	var initTrunk string
	var initMode string
	var initTemplate string
	var initPrefixIndex bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize or repair stack state",
		Long: `Initialize or repair persisted stack state.

This command writes stack metadata under .git/stack/state.json. It is primarily a repair and reconfiguration flow: normal mutating commands should auto-bootstrap state when possible instead of requiring init first.

init requires a clean worktree. When --trunk is omitted, stack detects trunk from origin/HEAD. Existing persisted branch relationships are preserved when possible while trunk, restack mode, and naming settings are refreshed.`,
		Example: "  git-stack init\n  git-stack init --trunk main --mode rebase\n  git-stack init --template \"feature/{slug}\" --prefix-index",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdInit(initTrunk, initMode, initTemplate, initPrefixIndex)
		},
	}
	initCmd.Flags().StringVar(&initTrunk, "trunk", "", "set trunk explicitly instead of detecting it from origin/HEAD")
	initCmd.Flags().StringVar(&initMode, "mode", defaultRestackMode, "default restack mode for future restack and forward runs: rebase or merge")
	initCmd.Flags().StringVar(&initTemplate, "template", "{slug}", "default branch naming template for stack new; supports {slug} and {n}")
	initCmd.Flags().BoolVar(&initPrefixIndex, "prefix-index", false, "prefix generated branch names with the next zero-padded index when the template does not include {n}")
	root.AddCommand(initCmd)

	var newParent string
	var newTemplate string
	var newPrefixIndex bool
	var newAdopt bool
	newCmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create or adopt a branch in stack",
		Long: `Create a new tracked branch, or adopt the current branch into stack state.

Without --adopt, stack creates a new branch from the chosen parent and starts tracking only that new branch. The default parent is the current branch. If [name] is omitted, stack generates a temporary slug. The final branch name is built from the configured template, where {slug} expands to the normalized name and {n} expands to the next zero-padded index.

With --adopt, stack does not create a branch. It tracks the current existing branch instead. If --parent is omitted during adopt, stack infers the parent from local branch ancestry. This command requires a clean worktree and auto-bootstraps config defaults if needed.`,
		Example: "  git-stack new add-search\n  git-stack new api/auth --parent main\n  git-stack new polish-login --template \"feature/{slug}\"\n  git-stack new --adopt\n  git-stack new --adopt --parent feature/base",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return a.cmdNew(name, newParent, newTemplate, newPrefixIndex, newAdopt)
		},
	}
	newCmd.Flags().StringVar(&newParent, "parent", "", "parent branch; defaults to the current branch, or inferred during --adopt")
	newCmd.Flags().StringVar(&newTemplate, "template", "", "override the configured naming template for this branch; supports {slug} and {n}")
	newCmd.Flags().BoolVar(&newPrefixIndex, "prefix-index", false, "prefix the generated branch name with the next zero-padded index when the template does not include {n}")
	newCmd.Flags().BoolVar(&newAdopt, "adopt", false, "track the current existing branch instead of creating a new branch")
	_ = newCmd.RegisterFlagCompletionFunc("parent", completeBranchRefs(true))
	root.AddCommand(newCmd)

	var stateAll bool
	var stateDrift bool
	var stateNoColor bool
	stateCmd := &cobra.Command{
		Use:     "state",
		Aliases: []string{"st"},
		Short:   "Show stack graph and state",
		Long: `Show the tracked branch graph for the current stack.

By default, state shows the connected tracked component rooted at the topmost tracked ancestor of the current branch. When run from trunk, it shows every tracked stack. If persisted state is missing, state shows an inferred local graph without requiring initialization.

	Each branch line includes current stack-sync state: local-only when no PR metadata exists, needs-restack when the recorded parent is no longer an ancestor, and needs-submit when a tracked PR branch no longer matches origin/<branch>. Existing PRs are shown inline as (PR #...). Use --drift to add precise parent mismatch details such as drifted-from-ancestor or missing-parent conditions.`,
		Example: "  git-stack state\n  git-stack state --all\n  git-stack state --drift --no-color",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdState(stateAll, stateDrift, stateNoColor)
		},
	}
	stateCmd.Flags().BoolVar(&stateAll, "all", false, "show all tracked stacks instead of only the current stack component")
	stateCmd.Flags().BoolVar(&stateDrift, "drift", false, "include drift markers when stored parentage does not match git ancestry")
	stateCmd.Flags().BoolVar(&stateNoColor, "no-color", false, "disable ANSI colors even when stdout is a TTY")
	root.AddCommand(stateCmd)

	var restackMode string
	var restackContinue bool
	var restackAbort bool
	restackCmd := &cobra.Command{
		Use:   "restack",
		Short: "Restack branches onto their parents",
		Long: `Rewrite tracked branches so each branch is based on its recorded parent.

On a fresh run, restack requires a clean worktree and initialized tracked state, then processes tracked branches in stack order using the configured restack mode. The default mode comes from stack state and is usually rebase unless changed with git-stack init.

If git stops for conflicts, stack records the in-progress operation under .git/stack/operation.json. Resolve the conflicts with normal git commands, then run git-stack restack --continue. Use --abort to abandon the recorded restack operation.`,
		Example: "  git-stack restack\n  git-stack restack --mode merge\n  git-stack restack --continue\n  git-stack restack --abort",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdRestack(restackMode, restackContinue, restackAbort)
		},
	}
	restackCmd.Flags().StringVar(&restackMode, "mode", "", "override the configured restack mode for this run: rebase or merge")
	restackCmd.Flags().BoolVar(&restackContinue, "continue", false, "continue an in-progress restack after conflicts are resolved with git")
	restackCmd.Flags().BoolVar(&restackAbort, "abort", false, "abort a recorded in-progress restack operation")
	root.AddCommand(restackCmd)

	var submitAll bool
	var submitNextOnClean string
	submitCmd := &cobra.Command{
		Use:   "submit [branch]",
		Short: "Push branches and create/update PRs",
		Long: `Push tracked branches to origin and create or update GitHub pull requests.

By default, submit operates on the current stack component in topological order. If [branch] is given, submit uses the stack containing that tracked branch. Use --all to submit every tracked branch. This command requires a clean worktree and initialized tracked state.

For each eligible tracked branch, stack force-pushes the local branch to origin with force-with-lease, then creates or updates its PR against the recorded parent branch. Branches are skipped when the local branch is missing or when there are no commits beyond the parent. If a tracked branch already has a merged PR and its remote branch has been deleted, submit may also clean up the local merged branch after confirming it is fully integrated.`,
		Example: "  git-stack submit\n  git-stack submit feat/login\n  git-stack submit --all\n  git-stack submit --next-on-clean feat/two feat/one",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			return a.cmdSubmit(submitAll, submitNextOnClean, branch)
		},
	}
	submitCmd.Flags().BoolVar(&submitAll, "all", false, "submit every tracked branch instead of only the current stack or the named branch's stack")
	submitCmd.Flags().StringVar(&submitNextOnClean, "next-on-clean", "", "when submit deletes the current merged branch, switch to this existing local branch instead of prompting")
	submitCmd.ValidArgsFunction = completeSingleBranchArg(false)
	_ = submitCmd.RegisterFlagCompletionFunc("next-on-clean", completeBranchRefs(false))
	root.AddCommand(submitCmd)

	var reparentOnto string
	var reparentPreserveLineage bool
	reparentCmd := &cobra.Command{
		Use:   "reparent [branch] --onto <new-parent>",
		Short: "Change the parent branch for a stack branch",
		Long: `Change the recorded parent for a tracked branch and rewrite its history onto the new base.

reparent requires a clean worktree. The target branch must already be tracked, and the new parent must exist locally or on origin. If [branch] is omitted, reparent targets the current branch. git-stack checks for invalid parent cycles before rewriting history.

Implementation-wise this is a rebase: git-stack switches to the target branch and runs git rebase --onto <new-parent> <old-parent>. If the branch already has PR metadata, git-stack also updates the PR base on GitHub. By default both Parent and LineageParent move to the new parent; use --preserve-lineage to keep the old lineage relationship for stack body/history context.`,
		Example: "  git-stack reparent --onto main\n  git-stack reparent feat-two --onto feat-base --preserve-lineage",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			return a.cmdReparent(branch, reparentOnto, reparentPreserveLineage)
		},
	}
	reparentCmd.Flags().StringVar(&reparentOnto, "onto", "", "new parent branch for the target; required")
	reparentCmd.Flags().BoolVar(&reparentPreserveLineage, "preserve-lineage", false, "keep the existing lineage parent instead of rewriting lineage to the new parent")
	reparentCmd.ValidArgsFunction = completeSingleBranchArg(false)
	_ = reparentCmd.RegisterFlagCompletionFunc("onto", completeBranchRefs(true))
	_ = reparentCmd.MarkFlagRequired("onto")
	root.AddCommand(reparentCmd)

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Diagnose local stack metadata health",
		Long: `Inspect persisted stack metadata and report structural problems.

check reads the saved stack state and validates trunk existence, parent references, parent ancestry, cycles, unrooted tracked branches, and any recorded restack operation. It also reports local branches that exist in git but are missing from stack state as informational output.

check does not mutate the repository. It exits non-zero when it finds errors and zero when the report contains only warnings or infos. Unlike most other commands, check requires initialized stack state and does not auto-bootstrap it.`,
		Example: "  git-stack check",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdCheck()
		},
	}
	root.AddCommand(checkCmd)

	var forwardNext string
	forwardCmd := &cobra.Command{
		Use:     "forward",
		Aliases: []string{"fw"},
		Short:   "Move forward after the current branch merges",
		Long: `Move the current stack forward after one or more branches in that stack have merged.

forward is a strict post-merge workflow. It requires a clean worktree, fetches origin with prune, then scans the current stack component for tracked branches whose PRs are merged, whose remote branches have already been deleted, and whose local commits are fully integrated into their PR base.

For eligible merged branches, stack cleans them from local state, reparents surviving children, restacks the surviving descendants, submits those updated branches, and restores you to an appropriate local branch. If the current branch is being cleaned and there are multiple surviving descendants, stack prompts for the next branch unless --next is provided.`,
		Example: "  git-stack forward\n  git-stack fw --next feat-two",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdForward(forwardNext)
		},
	}
	forwardCmd.Flags().StringVar(&forwardNext, "next", "", "when the current branch is deleted during forward, switch to this existing surviving branch instead of prompting")
	_ = forwardCmd.RegisterFlagCompletionFunc("next", completeBranchRefs(false))
	root.AddCommand(forwardCmd)

	var cleanYes bool
	var cleanAll bool
	var cleanIncludeSquash bool
	var cleanUntracked bool
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete merged local branches and reconcile stack state",
		Long: `Delete local branches that are already merged and reconcile tracked stack state.

clean requires a clean worktree, fetches origin with prune, builds a clean plan, prints that plan, and applies it after confirmation unless --yes is set. By default it only considers tracked branches in the current stack component and requires initialized tracked state. Use --all to consider every tracked branch.

	Tracked branches are eligible only when their remote branch is gone, a merged PR can be found for that branch head, the PR targeted trunk, and the branch is confirmed merged according to the configured merge-detection policy. Children of deleted tracked branches are reparented in stack state. With --untracked, clean also considers eligible untracked local branches globally. --include-squash relaxes merge detection so squash-integrated branches can be deleted when they are fully integrated into trunk.`,
		Example: "  git-stack clean\n  git-stack clean --yes\n  git-stack clean --all --yes\n  git-stack clean --yes --include-squash --untracked",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.cmdClean(cleanYes, cleanAll, cleanIncludeSquash, cleanUntracked)
		},
	}
	cleanCmd.Flags().BoolVar(&cleanYes, "yes", false, "apply the printed clean plan without an interactive confirmation prompt")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "consider all tracked branches instead of only the current stack component")
	cleanCmd.Flags().BoolVar(&cleanIncludeSquash, "include-squash", false, "allow deletion of branches that were integrated by squash or other non-merge-commit flows")
	cleanCmd.Flags().BoolVar(&cleanUntracked, "untracked", false, "also consider eligible untracked local branches outside persisted stack state")
	root.AddCommand(cleanCmd)
	a.addCompletionCmd(root)

	return root
}
