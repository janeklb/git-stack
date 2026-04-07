package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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

func (a *App) cmdRefresh(restack bool, publish string) error {
	publish = strings.TrimSpace(strings.ToLower(publish))
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
		if err := cleanupMergedBranchForRefresh(a.stdout, state, candidate, deps.git); err != nil {
			return err
		}
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

func (a *App) cmdAdvance(next string) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("advance fetch failed: %w", err)
	}

	repoRoot, state, err := loadStateFromRepo()
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
	target, err := chooseRefreshAdvanceTarget(a.in, a.stdout, state, candidate, next, deps.git)
	if err != nil {
		return err
	}
	if strings.TrimSpace(candidate.Base) == strings.TrimSpace(state.Trunk) {
		if err := syncLocalTrunkToFetchedRemote(state.Trunk); err != nil {
			return err
		}
	}

	a.printf("advance: cleanup %s, switch to %s, restack, submit all\n", candidate.Branch, target)
	if err := cleanupMergedBranchForRefreshAdvance(a.stdout, state, candidate, target, deps.git); err != nil {
		return err
	}
	restackQueue := advanceRestackQueue(state, candidate.Children)

	if err := saveState(repoRoot, state); err != nil {
		return err
	}

	if err := runRestackQueue(repoRoot, state, state.RestackMode, restackQueue, a.stdout); err != nil {
		return err
	}
	if err := a.cmdSubmitWithDeps(false, "", submitDeps{
		git:                 defaultGitClient{},
		gh:                  defaultGHClient{},
		ensureCleanWorktree: ensureCleanWorktree,
		loadState: func() (string, *State, bool, error) {
			return repoRoot, state, true, nil
		},
		submitQueue: func(state *State, all bool, args []string) ([]string, error) {
			return advanceSubmitQueue(state, candidate.Children), nil
		},
		ensurePR: ensurePR,
		syncCurrentStackBody: func(state *State, all bool, target string) error {
			return syncAdvanceStackBodies(state, candidate.Children)
		},
		saveState:           saveState,
		cleanupMergedBranch: func(*State, string) {},
	}); err != nil {
		return err
	}

	a.println("advance completed")
	return nil
}

func buildRefreshAdvanceCandidateWithDeps(state *State, current string, deps refreshPlanDeps) (refreshCleanupCandidate, error) {
	meta := state.Branches[current]
	if meta == nil {
		return refreshCleanupCandidate{}, fmt.Errorf("advance requires current branch to be tracked in stack state: %s", current)
	}
	if meta.PR == nil || meta.PR.Number <= 0 {
		return refreshCleanupCandidate{}, fmt.Errorf("advance requires current branch to have PR metadata: %s", current)
	}

	pr, err := deps.gh.View(meta.PR.Number)
	if err != nil {
		return refreshCleanupCandidate{}, fmt.Errorf("advance failed to load PR #%d for %s: %w", meta.PR.Number, current, err)
	}
	if !strings.EqualFold(pr.State, "MERGED") {
		return refreshCleanupCandidate{}, fmt.Errorf("advance requires current PR to be merged; %s is %s", current, strings.ToLower(pr.State))
	}

	remoteExists, err := deps.git.RemoteBranchExists(current)
	if err != nil {
		return refreshCleanupCandidate{}, fmt.Errorf("advance failed to check remote branch %s: %w", current, err)
	}
	if remoteExists {
		return refreshCleanupCandidate{}, fmt.Errorf("advance aborted: origin/%s still exists; delete the remote branch first", current)
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
		return refreshCleanupCandidate{}, fmt.Errorf("advance integration check failed for %s against %s: %w", current, base, err)
	}
	if !integrated {
		return refreshCleanupCandidate{}, fmt.Errorf("advance aborted: %s has local commits not fully integrated into %s", current, base)
	}

	return refreshCleanupCandidate{
		Branch:   current,
		Base:     base,
		HasLocal: deps.git.LocalBranchExists(current),
		Children: deps.mergedBranchChildren(state, current),
	}, nil
}

func chooseRefreshAdvanceTarget(in io.Reader, out io.Writer, state *State, candidate refreshCleanupCandidate, next string, git refreshGitClient) (string, error) {
	if next != "" {
		if next == candidate.Branch {
			return "", fmt.Errorf("advance --next cannot be the branch being cleaned: %s", next)
		}
		exists, err := advanceTargetExists(git, next)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("advance --next branch does not exist: %s", next)
		}
		return next, nil
	}

	if len(candidate.Children) == 0 {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		if target == candidate.Branch {
			return "", fmt.Errorf("advance could not choose a checkout target distinct from %s", candidate.Branch)
		}
		return target, nil
	}

	if len(candidate.Children) == 1 {
		target := candidate.Children[0]
		exists, err := advanceTargetExists(git, target)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("advance child branch does not exist: %s", target)
		}
		return target, nil
	}

	reader := bufio.NewReader(in)
	fmt.Fprintf(out, "%s -> select next branch to checkout after cleanup:\n", candidate.Branch)
	for i, child := range candidate.Children {
		fmt.Fprintf(out, "  %d) %s\n", i+1, child)
	}
	fmt.Fprintf(out, "selection [1-%d]: ", len(candidate.Children))
	answer, err := readPromptLine(reader)
	if err != nil {
		return "", fmt.Errorf("advance failed to read selection: %w", err)
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 1 || choice > len(candidate.Children) {
		return "", errors.New("advance invalid selection")
	}
	target := candidate.Children[choice-1]
	exists, err := advanceTargetExists(git, target)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("advance selected branch does not exist: %s", target)
	}
	return target, nil
}

func advanceTargetExists(git refreshGitClient, branch string) (bool, error) {
	if git.LocalBranchExists(branch) {
		return true, nil
	}
	remoteExists, err := git.RemoteBranchExists(branch)
	if err != nil {
		return false, fmt.Errorf("advance failed to verify branch %s: %w", branch, err)
	}
	return remoteExists, nil
}

func advanceSubmitQueue(state *State, roots []string) []string {
	if len(roots) == 0 {
		return nil
	}
	return advanceDescendantQueue(state, roots)
}

func advanceRestackQueue(state *State, roots []string) []string {
	return advanceDescendantQueue(state, roots)
}

func advanceDescendantQueue(state *State, roots []string) []string {
	if len(roots) == 0 {
		return nil
	}
	children := map[string][]string{}
	for branch, meta := range state.Branches {
		if meta == nil {
			continue
		}
		children[meta.Parent] = append(children[meta.Parent], branch)
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	selected := []string{}
	seen := map[string]bool{}
	orderedRoots := append([]string{}, roots...)
	sort.Strings(orderedRoots)
	var visit func(string)
	visit = func(branch string) {
		if seen[branch] {
			return
		}
		seen[branch] = true
		selected = append(selected, branch)
		for _, child := range children[branch] {
			visit(child)
		}
	}
	for _, root := range orderedRoots {
		visit(root)
	}
	return selected
}

func syncLocalTrunkToFetchedRemote(trunk string) error {
	trunk = strings.TrimSpace(trunk)
	if trunk == "" {
		return nil
	}
	remoteRef := "origin/" + trunk
	if gitRunQuiet("show-ref", "--verify", "--quiet", "refs/remotes/origin/"+trunk) != nil {
		return nil
	}
	if gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+trunk) != nil {
		return nil
	}
	localCommit, err := resolveBranchRef(trunk)
	if err != nil {
		return err
	}
	remoteCommit, err := resolveBranchRef(remoteRef)
	if err != nil {
		return err
	}
	if strings.TrimSpace(localCommit) == strings.TrimSpace(remoteCommit) {
		return nil
	}
	canFastForward, err := commitIsAncestor(localCommit, remoteCommit)
	if err != nil {
		return err
	}
	if !canFastForward {
		return fmt.Errorf("local trunk %s has diverged from fetched %s; update it before advancing", trunk, remoteRef)
	}
	current, err := currentBranch()
	if err != nil {
		return err
	}
	if current == trunk {
		return gitRun("merge", "--ff-only", remoteRef)
	}
	if err := gitRunQuiet("switch", trunk); err != nil {
		return err
	}
	if err := gitRun("merge", "--ff-only", remoteRef); err != nil {
		_ = gitRunQuiet("switch", current)
		return err
	}
	return gitRunQuiet("switch", current)
}

func syncAdvanceStackBodies(state *State, roots []string) error {
	selected := map[string]bool{}
	for _, branch := range advanceSubmitQueue(state, roots) {
		selected[branch] = true
	}
	ordered := orderedSelectedLineageBranches(state, selected)
	snapshots := fetchStackBodySyncSnapshots(state, ordered)

	lines := []StackPRLine{}
	updates := []stackBodyUpdate{}
	for _, snapshot := range snapshots {
		if snapshot.hasLine {
			lines = append(lines, snapshot.line)
		}
		if snapshot.hasUpdate {
			updates = append(updates, snapshot.update)
		}
	}
	if len(lines) == 0 || len(updates) == 0 {
		return nil
	}
	return applyStackBodyUpdates(lines, updates)
}

func cleanupMergedBranchForRefreshAdvance(out io.Writer, state *State, candidate refreshCleanupCandidate, switchTarget string, git refreshGitClient) error {
	if err := switchAwayThenDeleteMergedBranch(git, candidate.Branch, candidate.HasLocal, switchTarget); err != nil {
		return err
	}
	cleanupMergedBranchState(out, state, candidate.Branch, candidate.Base)
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

func cleanupMergedBranchForRefresh(out io.Writer, state *State, candidate refreshCleanupCandidate, git refreshGitClient) error {
	target := state.Trunk
	if strings.TrimSpace(target) == "" {
		target = "main"
	}
	if err := switchAwayThenDeleteMergedBranch(git, candidate.Branch, candidate.HasLocal, target); err != nil {
		return fmt.Errorf("refresh cleanup failed for %s: %w", candidate.Branch, err)
	}
	if err := cleanupMergedBranchState(out, state, candidate.Branch, candidate.Base); err != nil {
		return fmt.Errorf("refresh cleanup failed for %s: %w", candidate.Branch, err)
	}
	return nil
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
