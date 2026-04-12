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

type advanceCleanupCandidate struct {
	Branch   string
	Base     string
	HasLocal bool
	Children []string
}

type advanceDeps struct {
	git                     advanceGitClient
	gh                      advanceGHClient
	mergedCleanupIntegrated func(string, string, *GhPR) (bool, error)
	mergedBranchChildren    func(*State, string) []string
}

func defaultAdvanceDeps() advanceDeps {
	return advanceDeps{
		git:                     defaultGitClient{},
		gh:                      defaultGHClient{},
		mergedCleanupIntegrated: mergedCleanupIntegrated,
		mergedBranchChildren:    mergedBranchChildren,
	}
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

	deps := defaultAdvanceDeps()
	candidates, err := buildAdvanceCandidatesWithDeps(state, current, deps)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		a.println("advance: nothing to do")
		return nil
	}
	merged := map[string]bool{}
	for _, candidate := range candidates {
		merged[candidate.Branch] = true
	}
	target, err := chooseAdvanceTarget(a.in, a.stdout, state, current, merged, next, deps.git)
	if err != nil {
		return err
	}
	needsTrunkSync := false
	for _, candidate := range candidates {
		if candidate.Base == state.Trunk {
			needsTrunkSync = true
			break
		}
	}
	if needsTrunkSync {
		if err := syncLocalTrunkToFetchedRemote(state.Trunk); err != nil {
			return err
		}
	}

	// Clean merged slices first, then restack only the surviving roots that were
	// directly downstream of those merged branches.
	a.printlnf("advance: cleanup merged branches in current stack, switch to %s, restack, submit all", target)
	restackRoots := []string{}
	restackRootSet := map[string]bool{}
	rebaseBases := map[string]string{}
	for _, candidate := range candidates {
		mergedBranchHead, err := resolveBranchRef(candidate.Branch)
		if err != nil {
			return err
		}
		switchTarget := ""
		if candidate.Branch == current {
			switchTarget = target
		}
		if err := cleanupMergedBranchForAdvance(a.stdout, state, candidate, switchTarget, deps.git); err != nil {
			return err
		}
		for _, child := range candidate.Children {
			if merged[child] || restackRootSet[child] {
				continue
			}
			restackRootSet[child] = true
			restackRoots = append(restackRoots, child)
			rebaseBases[child] = mergedBranchHead
		}
	}
	restackQueue := advanceRestackQueue(state, restackRoots)
	if len(rebaseBases) == 0 {
		rebaseBases = nil
	}

	if err := saveState(repoRoot, state); err != nil {
		return err
	}

	if err := runRestackQueue(repoRoot, state, state.RestackMode, restackQueue, rebaseBases, a.stdout); err != nil {
		return err
	}
	if err := a.cmdSubmitWithDeps(false, "", "", submitDeps{
		git:                 defaultGitClient{},
		gh:                  defaultGHClient{},
		ensureCleanWorktree: ensureCleanWorktree,
		loadState: func() (string, *State, bool, error) {
			return repoRoot, state, true, nil
		},
		submitQueue: func(state *State, all bool, args []string) ([]string, error) {
			return advanceSubmitQueue(state, restackRoots), nil
		},
		ensurePR: ensurePR,
		syncCurrentStackBody: func(state *State, all bool, target string) error {
			return syncAdvanceStackBodies(state, restackRoots)
		},
		saveState:           saveState,
		cleanupMergedBranch: func(*State, string, string) (bool, error) { return false, nil },
	}); err != nil {
		return err
	}
	if err := restoreAdvanceTarget(current, target, merged, deps.git); err != nil {
		return err
	}

	a.println("advance completed")
	return nil
}

// Advance should look across the whole current stack component, not just the
// checked-out branch, and clean merged slices from the top down.
func buildAdvanceCandidatesWithDeps(state *State, current string, deps advanceDeps) ([]advanceCleanupCandidate, error) {
	selected := branchesInCurrentStack(state, current)
	order := topoOrderSelected(state, selected)
	candidates := []advanceCleanupCandidate{}
	for _, branch := range order {
		candidate, eligible, err := detectAdvanceCandidateWithDeps(state, branch, deps)
		if err != nil {
			return nil, err
		}
		if eligible {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func buildAdvanceCandidateWithDeps(state *State, current string, deps advanceDeps) (advanceCleanupCandidate, error) {
	candidate, eligible, err := detectAdvanceCandidateWithDeps(state, current, deps)
	if err != nil {
		return advanceCleanupCandidate{}, err
	}
	if !eligible {
		meta := state.Branches[current]
		if meta == nil {
			return advanceCleanupCandidate{}, fmt.Errorf("advance requires current branch to be tracked in stack state: %s", current)
		}
		if meta.PR == nil || meta.PR.Number <= 0 {
			return advanceCleanupCandidate{}, fmt.Errorf("advance requires current branch to have PR metadata: %s", current)
		}
		pr, err := deps.gh.View(meta.PR.Number)
		if err != nil {
			return advanceCleanupCandidate{}, fmt.Errorf("advance failed to load PR #%d for %s: %w", meta.PR.Number, current, err)
		}
		return advanceCleanupCandidate{}, fmt.Errorf("advance requires current PR to be merged; %s is %s", current, strings.ToLower(pr.State))
	}
	return candidate, nil
}

func detectAdvanceCandidateWithDeps(state *State, current string, deps advanceDeps) (advanceCleanupCandidate, bool, error) {
	meta := state.Branches[current]
	if meta == nil {
		return advanceCleanupCandidate{}, false, nil
	}
	if meta.PR == nil || meta.PR.Number <= 0 {
		return advanceCleanupCandidate{}, false, nil
	}

	pr, err := deps.gh.View(meta.PR.Number)
	if err != nil {
		return advanceCleanupCandidate{}, false, fmt.Errorf("advance failed to load PR #%d for %s: %w", meta.PR.Number, current, err)
	}
	if !strings.EqualFold(pr.State, "MERGED") {
		return advanceCleanupCandidate{}, false, nil
	}

	remoteExists, err := deps.git.RemoteBranchExists(current)
	if err != nil {
		return advanceCleanupCandidate{}, false, fmt.Errorf("advance failed to check remote branch %s: %w", current, err)
	}
	if remoteExists {
		return advanceCleanupCandidate{}, false, fmt.Errorf("advance aborted: origin/%s still exists; delete the remote branch first", current)
	}

	base := state.Trunk
	if pr.BaseRefName != "" {
		base = pr.BaseRefName
	} else if meta.PR.Base != "" {
		base = meta.PR.Base
	} else if meta.Parent != "" {
		base = meta.Parent
	}

	integrated, err := deps.mergedCleanupIntegrated(current, base, pr)
	if err != nil {
		return advanceCleanupCandidate{}, false, fmt.Errorf("advance integration check failed for %s against %s: %w", current, base, err)
	}
	if !integrated {
		return advanceCleanupCandidate{}, false, fmt.Errorf("advance aborted: %s has local commits not fully integrated into %s", current, base)
	}

	return advanceCleanupCandidate{
		Branch:   current,
		Base:     base,
		HasLocal: deps.git.LocalBranchExists(current),
		Children: deps.mergedBranchChildren(state, current),
	}, true, nil
}

func chooseAdvanceTarget(in io.Reader, out io.Writer, state *State, current string, merged map[string]bool, next string, git advanceGitClient) (string, error) {
	if !merged[current] {
		return current, nil
	}
	if next != "" {
		if next == current {
			return "", fmt.Errorf("advance --next cannot be the branch being cleaned: %s", next)
		}
		if merged[next] {
			return "", fmt.Errorf("advance --next cannot be another merged branch being cleaned: %s", next)
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
	options := advanceCleanupTargets(state, current, merged)

	if len(options) == 0 {
		target := state.Trunk
		if target == "" {
			target = "main"
		}
		if target == current {
			return "", fmt.Errorf("advance could not choose a checkout target distinct from %s", current)
		}
		return target, nil
	}

	if len(options) == 1 {
		target := options[0]
		exists, err := advanceTargetExists(git, target)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("advance next branch does not exist: %s", target)
		}
		return target, nil
	}

	reader := bufio.NewReader(in)
	fmt.Fprintf(out, "%s -> select next branch to checkout after cleanup:\n", current)
	for i, child := range options {
		fmt.Fprintf(out, "  %d) %s\n", i+1, child)
	}
	fmt.Fprintf(out, "selection [1-%d]: ", len(options))
	answer, err := readPromptLine(reader)
	if err != nil {
		return "", fmt.Errorf("advance failed to read selection: %w", err)
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 1 || choice > len(options) {
		return "", errors.New("advance invalid selection")
	}
	target := options[choice-1]
	exists, err := advanceTargetExists(git, target)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("advance selected branch does not exist: %s", target)
	}
	return target, nil
}

// When the current branch is itself being deleted, pick the first surviving
// descendants beneath it as candidate checkout targets.
func advanceCleanupTargets(state *State, branch string, merged map[string]bool) []string {
	children := map[string][]string{}
	for name, meta := range state.Branches {
		if meta == nil {
			continue
		}
		children[meta.Parent] = append(children[meta.Parent], name)
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	targets := map[string]bool{}
	var visit func(string)
	visit = func(node string) {
		if node == "" {
			return
		}
		if !merged[node] {
			targets[node] = true
			return
		}
		for _, child := range children[node] {
			visit(child)
		}
	}
	for _, child := range children[branch] {
		visit(child)
	}
	ordered := make([]string, 0, len(targets))
	for target := range targets {
		ordered = append(ordered, target)
	}
	sort.Strings(ordered)
	return ordered
}

func advanceTargetExists(git advanceGitClient, branch string) (bool, error) {
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

func advanceRebaseBases(roots []string, oldBase string) map[string]string {
	oldBase = strings.TrimSpace(oldBase)
	if len(roots) == 0 || oldBase == "" {
		return nil
	}
	bases := map[string]string{}
	for _, branch := range roots {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		bases[branch] = oldBase
	}
	if len(bases) == 0 {
		return nil
	}
	return bases
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

func cleanupMergedBranchForAdvance(out io.Writer, state *State, candidate advanceCleanupCandidate, switchTarget string, git advanceGitClient) error {
	if err := switchAwayThenDeleteMergedBranch(git, candidate.Branch, candidate.HasLocal, switchTarget); err != nil {
		return err
	}
	if err := cleanupMergedBranchState(out, state, candidate.Branch, candidate.Base); err != nil {
		return err
	}
	return nil
}

// Preserve the user's location when their starting branch survived the advance;
// otherwise stay on the fallback branch chosen during cleanup.
func restoreAdvanceTarget(original, fallback string, merged map[string]bool, git advanceGitClient) error {
	target := fallback
	if !merged[original] {
		target = original
	}
	if strings.TrimSpace(target) == "" {
		return nil
	}
	current, err := git.CurrentBranch()
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}
	return git.Run("switch", target)
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
		mergeCommit = pr.MergeCommit.OID
	}
	headCommit := pr.HeadRefOID
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
