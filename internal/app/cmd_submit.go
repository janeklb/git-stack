package app

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (a *App) defaultSubmitDeps() submitDeps {
	git := defaultGitClient{}
	return submitDeps{
		git:                  git,
		gh:                   defaultGHClient{},
		ensureCleanWorktree:  ensureCleanWorktree,
		loadState:            loadStateFromRepoOrInfer,
		submitQueue:          submitQueue,
		ensurePR:             ensurePR,
		syncCurrentStackBody: syncCurrentStackBodies,
		saveState:            saveState,
		cleanMergedBranch: func(state *State, branch string, nextOnClean string) (bool, error) {
			return a.cleanMergedBranch(state, branch, nextOnClean, git)
		},
	}
}

func (a *App) cmdSubmit(all bool, nextOnClean, branch string) error {
	deps := a.defaultSubmitDeps()
	return a.cmdSubmitWithDeps(all, nextOnClean, branch, deps)
}

func (a *App) cmdSubmitWithDeps(all bool, nextOnClean, branch string, deps submitDeps) error {
	nextOnClean = strings.TrimSpace(nextOnClean)
	if err := deps.ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, persisted, err := deps.loadState()
	if err != nil {
		return err
	}
	if err := requirePersistedTrackedState(state, persisted, "submit"); err != nil {
		return err
	}

	args := []string{}
	if branch != "" {
		args = append(args, branch)
	}
	queue, err := deps.submitQueue(state, all, args)
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		a.println("nothing to submit")
		if nextOnClean != "" {
			a.println("submit: note: --next-on-clean was not used because submit did not clean the current branch")
		}
		return nil
	}

	usedNextOnClean := false
	for _, branch := range queue {
		meta, ok := state.Branches[branch]
		if !ok {
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		existingPR, mergedPR, err := resolveSubmitBranchPR(state, branch, parent, deps)
		if err != nil {
			return fmt.Errorf("submit %s: %w", branch, err)
		}
		if mergedPR != nil {
			a.printlnf("%s -> PR #%d already merged, skipping", branch, mergedPR.Number)
			used, err := deps.cleanMergedBranch(state, branch, nextOnClean)
			if err != nil {
				return err
			}
			usedNextOnClean = usedNextOnClean || used
			continue
		}
		if !deps.git.LocalBranchExists(branch) {
			a.printlnf("%s -> skipped: local branch no longer exists", branch)
			continue
		}
		hasCommits, err := deps.git.BranchHasCommitsSince(parent, branch)
		if err != nil {
			return fmt.Errorf("check submit range %s..%s: %w", parent, branch, err)
		}
		if !hasCommits {
			a.printlnf("%s -> skipped: no commits beyond %s", branch, parent)
			continue
		}
		if err := deps.git.PushBranch(branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
		pr, err := deps.ensurePR(repoRoot, state.Trunk, branch, parent, meta.PR, existingPR)
		if err != nil {
			return fmt.Errorf("submit %s: %w", branch, err)
		}
		meta.PR = pr
		a.printlnf("%s -> PR #%d %s", branch, pr.Number, pr.URL)
	}
	if err := deps.syncCurrentStackBody(state, all, branch); err != nil {
		return err
	}
	if nextOnClean != "" && !usedNextOnClean {
		a.println("submit: note: --next-on-clean was not used because submit did not clean the current branch")
	}

	if persisted {
		if err := deps.saveState(repoRoot, state); err != nil {
			return err
		}
	}
	return nil
}

func resolveSubmitBranchPR(state *State, branch, parent string, deps submitDeps) (*GhPR, *GhPR, error) {
	meta := state.Branches[branch]
	if meta == nil {
		return nil, nil, nil
	}
	if meta.PR != nil && meta.PR.Number > 0 {
		existing, err := deps.gh.View(meta.PR.Number)
		if err == nil {
			meta.PR = trackedPRMetaFromPR(state, branch, existing, true)
			if strings.EqualFold(existing.State, "MERGED") {
				return nil, existing, nil
			}
			return existing, nil, nil
		}
	}

	pr, repaired, err := repairTrackedPRMetadata(state, branch, deps.gh)
	if err != nil {
		return nil, nil, err
	}
	if !repaired || pr == nil {
		return nil, nil, nil
	}
	if strings.EqualFold(pr.State, "MERGED") {
		if err := ensureSubmitMergedRepairSafe(branch, state, parent, pr, deps.git); err != nil {
			return nil, nil, err
		}
		return nil, pr, nil
	}
	return pr, nil, nil
}

func ensureSubmitMergedRepairSafe(branch string, state *State, parent string, pr *GhPR, git submitGitClient) error {
	remoteExists, err := git.RemoteBranchExists(branch)
	if err != nil || remoteExists {
		return err
	}

	base := parent
	if pr != nil && pr.BaseRefName != "" {
		base = pr.BaseRefName
	}
	if base == "" {
		base = state.Trunk
	}
	if !git.LocalBranchExists(branch) {
		return nil
	}
	integrated, err := git.BranchFullyIntegrated(branch, base)
	if err != nil {
		return fmt.Errorf("merged PR metadata was found, but integration check against %s failed: %w", base, err)
	}
	if !integrated {
		return fmt.Errorf("merged PR metadata was found, but local commits are not fully integrated into %s", base)
	}
	return nil
}

func (a *App) cleanMergedBranch(state *State, branch string, nextOnClean string, git submitGitClient) (bool, error) {
	remoteExists, remoteErr := git.RemoteBranchExists(branch)
	if remoteErr != nil {
		return false, nil
	}
	if remoteExists {
		return false, nil
	}

	current, currentErr := git.CurrentBranch()
	base := state.Trunk
	if meta := state.Branches[branch]; meta != nil {
		if meta.PR != nil && meta.PR.Base != "" {
			base = meta.PR.Base
		} else if meta.Parent != "" {
			base = meta.Parent
		}
	}

	integrated, integratedErr := git.BranchFullyIntegrated(branch, base)
	if integratedErr != nil {
		a.printlnf("%s -> merged and remote deleted, but integration check failed; keeping local branch", branch)
		return false, nil
	}
	if !integrated {
		a.printlnf("%s -> merged and remote deleted, but unmerged local changes detected; keeping local branch", branch)
		return false, nil
	}

	if currentErr == nil && current == branch {
		target, proceed, usedNextOnClean, err := chooseSubmitCleanSwitchTarget(state, branch, nextOnClean, a.in, a.stdout, git)
		if err != nil {
			return usedNextOnClean, err
		}
		if !proceed {
			a.printlnf("%s -> keeping local merged branch", branch)
			return usedNextOnClean, nil
		}
		if err := switchAwayThenDeleteMergedBranch(git, branch, true, target); err != nil {
			a.printlnf("%s -> %v", branch, err)
			return usedNextOnClean, nil
		}
		if err := cleanMergedBranchState(a.stdout, state, branch, base); err != nil {
			a.printlnf("%s -> %v", branch, err)
			return usedNextOnClean, nil
		}
		a.printlnf("%s -> deleted local merged branch", branch)
		return usedNextOnClean, nil
	}

	if err := switchAwayThenDeleteMergedBranch(git, branch, true, ""); err != nil {
		a.printlnf("%s -> %v", branch, err)
		return false, nil
	}
	if err := cleanMergedBranchState(a.stdout, state, branch, base); err != nil {
		a.printlnf("%s -> %v", branch, err)
		return false, nil
	}
	a.printlnf("%s -> deleted local merged branch", branch)
	return false, nil
}

func chooseSubmitCleanSwitchTarget(state *State, branch, nextOnClean string, in io.Reader, out io.Writer, git submitGitClient) (string, bool, bool, error) {
	if nextOnClean != "" {
		target, err := validateSubmitCleanTarget(branch, nextOnClean, git)
		if err != nil {
			return "", false, true, err
		}
		fmt.Fprintf(out, "%s -> using --next-on-clean target %s before clean\n", branch, target)
		return target, true, true, nil
	}
	target, proceed := promptSwitchTargetForMergedBranchDeletion(state, branch, in, out)
	return target, proceed, false, nil
}

func validateSubmitCleanTarget(branch, nextOnClean string, git submitGitClient) (string, error) {
	target := nextOnClean
	if target == "" {
		return "", fmt.Errorf("submit --next-on-clean requires a branch name")
	}
	if target == branch {
		return "", fmt.Errorf("submit --next-on-clean cannot be the branch being cleaned: %s", target)
	}
	if !git.LocalBranchExists(target) {
		return "", fmt.Errorf("submit --next-on-clean branch does not exist locally: %s", target)
	}
	return target, nil
}

func promptSwitchTargetForMergedBranchDeletion(state *State, branch string, in io.Reader, out io.Writer) (string, bool) {
	children := mergedBranchChildren(state, branch)
	if len(children) == 0 {
		target := state.Trunk
		if target == "" {
			target = "main"
		}
		fmt.Fprintf(out, "%s -> merged and remote deleted; switching to %s before clean\n", branch, target)
		return target, true
	}

	reader := bufio.NewReader(in)
	if len(children) == 1 {
		target := children[0]
		fmt.Fprintf(out, "%s -> merged and remote deleted. Switch to %s and delete this branch? [y/N]: ", branch, target)
		answer, err := readPromptLine(reader)
		if err != nil {
			fmt.Fprintf(out, "%s -> failed to read clean prompt\n", branch)
			return "", false
		}
		if answer != "y" && answer != "yes" {
			return "", false
		}
		return target, true
	}

	fmt.Fprintf(out, "%s -> merged and remote deleted. Choose branch to switch to before deleting it:\n", branch)
	for i, child := range children {
		fmt.Fprintln(out, fmt.Sprintf("  %d) %s", i+1, child))
	}
	fmt.Fprintln(out, "  0) keep local branch")
	fmt.Fprintf(out, "selection [0-%d]: ", len(children))
	answer, err := readPromptLine(reader)
	if err != nil {
		fmt.Fprintf(out, "%s -> failed to read clean selection\n", branch)
		return "", false
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 0 || choice > len(children) {
		fmt.Fprintf(out, "%s -> invalid selection, keeping local branch\n", branch)
		return "", false
	}
	if choice == 0 {
		return "", false
	}
	return children[choice-1], true
}
