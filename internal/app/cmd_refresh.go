package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type refreshCleanupCandidate struct {
	Branch   string
	Base     string
	HasLocal bool
	Children []string
}

type refreshPlan struct {
	Cleanup []refreshCleanupCandidate
}

type refreshPlanDeps struct {
	git                     refreshGitClient
	gh                      refreshGHClient
	mergedCleanupIntegrated func(string, string, *GhPR) (bool, error)
	mergedBranchChildren    func(*State, string) []string
}

func defaultRefreshPlanDeps() refreshPlanDeps {
	return refreshPlanDeps{
		git:                     defaultGitClient{},
		gh:                      defaultGHClient{},
		mergedCleanupIntegrated: mergedCleanupIntegrated,
		mergedBranchChildren:    mergedBranchChildren,
	}
}

func (a *App) cmdRefresh(restack bool, publish string, advance bool, next string) error {
	publish = strings.TrimSpace(strings.ToLower(publish))
	next = strings.TrimSpace(next)
	if advance {
		if restack {
			return errors.New("--advance cannot be combined with --restack")
		}
		if publish != "" {
			return errors.New("--advance cannot be combined with --publish")
		}
		return a.cmdRefreshAdvance(next)
	}
	if next != "" {
		return errors.New("--next requires --advance")
	}
	if publish != "" && publish != "current" && publish != "all" {
		return errors.New("--publish must be one of: current, all")
	}
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("refresh fetch failed: %w", err)
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}

	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	plan, err := buildRefreshPlan(state)
	if err != nil {
		return err
	}
	if len(plan.Cleanup) == 0 && !restack && publish == "" {
		a.println("refresh: nothing to do")
		return nil
	}

	current, _ := currentBranch()
	printRefreshPlan(a.stdout, plan, restack, publish, current)
	if !confirmRefreshApply(a.in, a.stdout) {
		a.println("refresh cancelled")
		return nil
	}

	deps := defaultRefreshPlanDeps()
	for _, candidate := range plan.Cleanup {
		cleanupMergedBranchForRefresh(a.stdout, state, candidate, deps.git)
	}

	if persisted {
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}

	if restack {
		if err := a.cmdRestack("", false, false); err != nil {
			return err
		}
	}

	if publish == "all" {
		if err := syncCurrentStackBodies(state, true, ""); err != nil {
			return err
		}
	} else if publish == "current" {
		if _, ok := state.Branches[current]; !ok {
			current = ""
		}
		if err := syncCurrentStackBodies(state, false, current); err != nil {
			return err
		}
	}

	a.println("refresh completed")
	return nil
}

func (a *App) cmdRefreshAdvance(next string) error {
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("refresh fetch failed: %w", err)
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}

	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	current, err := currentBranch()
	if err != nil {
		return err
	}

	deps := defaultRefreshPlanDeps()
	candidate, err := buildRefreshAdvanceCandidateWithDeps(state, current, deps)
	if err != nil {
		return err
	}
	target, err := chooseRefreshAdvanceTarget(state, candidate, next, deps.git)
	if err != nil {
		return err
	}

	fmt.Printf("refresh advance: cleanup %s, switch to %s, restack, submit all\n", candidate.Branch, target)
	if err := cleanupMergedBranchForRefreshAdvance(state, candidate, target, deps.git); err != nil {
		return err
	}

	if persisted {
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}

	if err := a.cmdRestack("", false, false); err != nil {
		return err
	}
	if err := a.cmdSubmit(true, ""); err != nil {
		return err
	}

	fmt.Println("refresh advance completed")
	return nil
}

func buildRefreshAdvanceCandidateWithDeps(state *State, current string, deps refreshPlanDeps) (refreshCleanupCandidate, error) {
	meta := state.Branches[current]
	if meta == nil {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance requires current branch to be tracked in stack state: %s", current)
	}
	if meta.PR == nil || meta.PR.Number <= 0 {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance requires current branch to have PR metadata: %s", current)
	}

	pr, err := deps.gh.View(meta.PR.Number)
	if err != nil {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance failed to load PR #%d for %s: %w", meta.PR.Number, current, err)
	}
	if !strings.EqualFold(pr.State, "MERGED") {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance requires current PR to be merged; %s is %s", current, strings.ToLower(pr.State))
	}

	remoteExists, err := deps.git.RemoteBranchExists(current)
	if err != nil {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance failed to check remote branch %s: %w", current, err)
	}
	if remoteExists {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance aborted: origin/%s still exists; delete the remote branch first", current)
	}

	base := state.Trunk
	if strings.TrimSpace(pr.BaseRefName) != "" {
		base = strings.TrimSpace(pr.BaseRefName)
	} else if strings.TrimSpace(meta.PR.Base) != "" {
		base = strings.TrimSpace(meta.PR.Base)
	} else if strings.TrimSpace(meta.Parent) != "" {
		base = strings.TrimSpace(meta.Parent)
	}

	integrated, err := deps.mergedCleanupIntegrated(current, base, pr)
	if err != nil {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance integration check failed for %s against %s: %w", current, base, err)
	}
	if !integrated {
		return refreshCleanupCandidate{}, fmt.Errorf("refresh --advance aborted: %s has local commits not fully integrated into %s", current, base)
	}

	return refreshCleanupCandidate{
		Branch:   current,
		Base:     base,
		HasLocal: deps.git.LocalBranchExists(current),
		Children: deps.mergedBranchChildren(state, current),
	}, nil
}

func chooseRefreshAdvanceTarget(state *State, candidate refreshCleanupCandidate, next string, git refreshGitClient) (string, error) {
	if next != "" {
		if next == candidate.Branch {
			return "", fmt.Errorf("refresh --advance --next cannot be the branch being cleaned: %s", next)
		}
		exists, err := refreshTargetExists(git, next)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("refresh --advance --next branch does not exist: %s", next)
		}
		return next, nil
	}

	if len(candidate.Children) == 0 {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		if target == candidate.Branch {
			return "", fmt.Errorf("refresh --advance could not choose a checkout target distinct from %s", candidate.Branch)
		}
		return target, nil
	}

	if len(candidate.Children) == 1 {
		target := candidate.Children[0]
		exists, err := refreshTargetExists(git, target)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("refresh --advance child branch does not exist: %s", target)
		}
		return target, nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s -> select next branch to checkout after cleanup:\n", candidate.Branch)
	for i, child := range candidate.Children {
		fmt.Printf("  %d) %s\n", i+1, child)
	}
	fmt.Printf("selection [1-%d]: ", len(candidate.Children))
	answer, err := readPromptLine(reader)
	if err != nil {
		return "", fmt.Errorf("refresh --advance failed to read selection: %w", err)
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 1 || choice > len(candidate.Children) {
		return "", errors.New("refresh --advance invalid selection")
	}
	target := candidate.Children[choice-1]
	exists, err := refreshTargetExists(git, target)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("refresh --advance selected branch does not exist: %s", target)
	}
	return target, nil
}

func refreshTargetExists(git refreshGitClient, branch string) (bool, error) {
	if git.LocalBranchExists(branch) {
		return true, nil
	}
	remoteExists, err := git.RemoteBranchExists(branch)
	if err != nil {
		return false, fmt.Errorf("refresh --advance failed to verify branch %s: %w", branch, err)
	}
	return remoteExists, nil
}

func cleanupMergedBranchForRefreshAdvance(state *State, candidate refreshCleanupCandidate, switchTarget string, git refreshGitClient) error {
	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}
	if current == candidate.Branch {
		if err := git.Run("switch", switchTarget); err != nil {
			return fmt.Errorf("failed to switch to %s before cleanup: %w", switchTarget, err)
		}
	}

	if candidate.HasLocal {
		if err := git.DeleteLocalBranch(candidate.Branch); err != nil {
			return fmt.Errorf("failed to delete local merged branch %s: %w", candidate.Branch, err)
		}
	}

	archiveMergedBranch(state, candidate.Branch)
	reparentChildrenAfterCleanup(os.Stdout, state, candidate.Branch, candidate.Base)
	delete(state.Branches, candidate.Branch)
	pruneArchivedLineage(state)
	fmt.Printf("%s -> cleaned merged branch from local stack state\n", candidate.Branch)
	return nil
}

func buildRefreshPlan(state *State) (*refreshPlan, error) {
	return buildRefreshPlanWithDeps(state, defaultRefreshPlanDeps())
}

func buildRefreshPlanWithDeps(state *State, deps refreshPlanDeps) (*refreshPlan, error) {
	plan := &refreshPlan{Cleanup: []refreshCleanupCandidate{}}
	branches := topoOrder(state)
	for _, branch := range branches {
		meta := state.Branches[branch]
		if meta == nil || meta.PR == nil || meta.PR.Number <= 0 {
			continue
		}
		pr, err := deps.gh.View(meta.PR.Number)
		if err != nil || !strings.EqualFold(pr.State, "MERGED") {
			continue
		}

		remoteExists, remoteErr := deps.git.RemoteBranchExists(branch)
		if remoteErr != nil || remoteExists {
			continue
		}

		base := state.Trunk
		if strings.TrimSpace(meta.PR.Base) != "" {
			base = meta.PR.Base
		} else if strings.TrimSpace(meta.Parent) != "" {
			base = meta.Parent
		}

		hasLocal := deps.git.LocalBranchExists(branch)
		if hasLocal {
			integrated, err := deps.mergedCleanupIntegrated(branch, base, pr)
			if err != nil || !integrated {
				continue
			}
		}

		plan.Cleanup = append(plan.Cleanup, refreshCleanupCandidate{
			Branch:   branch,
			Base:     base,
			HasLocal: hasLocal,
			Children: deps.mergedBranchChildren(state, branch),
		})
	}

	sort.Slice(plan.Cleanup, func(i, j int) bool {
		return plan.Cleanup[i].Branch < plan.Cleanup[j].Branch
	})
	return plan, nil
}

func printRefreshPlan(out io.Writer, plan *refreshPlan, restack bool, publish, current string) {
	fmt.Fprintln(out, "refresh plan:")
	if len(plan.Cleanup) == 0 {
		fmt.Fprintln(out, "- cleanup: none")
	} else {
		for _, candidate := range plan.Cleanup {
			kind := "state-only"
			if candidate.HasLocal {
				kind = "delete-local+state"
			}
			fmt.Fprintf(out, "- cleanup: %s (%s)\n", candidate.Branch, kind)
			if len(candidate.Children) > 0 {
				fmt.Fprintf(out, "  reparent children -> %s: %s\n", candidate.Base, strings.Join(candidate.Children, ", "))
			}
		}
	}
	if restack {
		fmt.Fprintln(out, "- restack: enabled")
	} else {
		fmt.Fprintln(out, "- restack: disabled")
	}
	if publish == "all" {
		fmt.Fprintln(out, "- publish: all tracked branches")
	} else if publish == "current" {
		if strings.TrimSpace(current) == "" {
			fmt.Fprintln(out, "- publish: current stack (auto) ")
		} else {
			fmt.Fprintf(out, "- publish: current stack from %s\n", current)
		}
	} else {
		fmt.Fprintln(out, "- publish: disabled")
	}
}

func confirmRefreshApply(in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)
	fmt.Fprint(out, "apply refresh plan? [y/N]: ")
	answer, err := readPromptLine(reader)
	if err != nil {
		return false
	}
	return answer == "y" || answer == "yes"
}

func cleanupMergedBranchForRefresh(out io.Writer, state *State, candidate refreshCleanupCandidate, git refreshGitClient) {
	current, err := git.CurrentBranch()
	if err == nil && current == candidate.Branch {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		if switchErr := git.Run("switch", target); switchErr != nil {
			fmt.Fprintf(out, "%s -> failed to switch to %s before cleanup: %v\n", candidate.Branch, target, switchErr)
			return
		}
	}

	if candidate.HasLocal {
		if err := git.DeleteLocalBranch(candidate.Branch); err != nil {
			fmt.Fprintf(out, "%s -> failed to delete local merged branch: %v\n", candidate.Branch, err)
			return
		}
	}

	archiveMergedBranch(state, candidate.Branch)
	reparentChildrenAfterCleanup(out, state, candidate.Branch, candidate.Base)
	delete(state.Branches, candidate.Branch)
	pruneArchivedLineage(state)
	fmt.Fprintf(out, "%s -> cleaned merged branch from local stack state\n", candidate.Branch)
}

func reparentChildrenAfterCleanup(out io.Writer, state *State, removedBranch, replacementParent string) {
	if strings.TrimSpace(replacementParent) == "" {
		replacementParent = state.Trunk
	}
	for name, meta := range state.Branches {
		if name == removedBranch || meta == nil {
			continue
		}
		if meta.Parent != removedBranch {
			continue
		}
		meta.Parent = replacementParent
		if meta.PR != nil {
			meta.PR.Base = replacementParent
		}
		fmt.Fprintf(out, "%s -> reparented to %s after merged parent cleanup\n", name, replacementParent)
	}
}

func mergedCleanupIntegrated(branch, base string, pr *GhPR) (bool, error) {
	integrated, err := branchFullyIntegrated(branch, base)
	if err != nil {
		return false, err
	}
	if integrated {
		return true, nil
	}
	if pr == nil {
		return false, nil
	}

	mergeCommit := ""
	if pr.MergeCommit != nil {
		mergeCommit = strings.TrimSpace(pr.MergeCommit.OID)
	}
	headCommit := strings.TrimSpace(pr.HeadRefOID)
	if headCommit == "" {
		return false, nil
	}

	localAtOrBehindHead, err := branchAtOrBehindCommit(branch, headCommit)
	if err != nil {
		return false, err
	}
	if !localAtOrBehindHead {
		return false, nil
	}
	if mergeCommit == "" {
		return false, nil
	}

	contains, err := baseContainsCommit(base, mergeCommit)
	if err != nil {
		return false, err
	}
	return contains, nil
}
