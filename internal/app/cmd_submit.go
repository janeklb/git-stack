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
		cleanupMergedBranch: func(state *State, branch string, nextOnCleanup string) (bool, error) {
			return a.cleanupMergedBranch(state, branch, nextOnCleanup, git)
		},
	}
}

func (a *App) cmdSubmit(all bool, nextOnCleanup, branch string) error {
	deps := a.defaultSubmitDeps()
	return a.cmdSubmitWithDeps(all, nextOnCleanup, branch, deps)
}

func (a *App) cmdSubmitWithDeps(all bool, nextOnCleanup, branch string, deps submitDeps) error {
	if err := deps.ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, persisted, err := deps.loadState()
	if err != nil {
		return err
	}
	if _, err := ensurePersistedState(repoRoot, state, persisted, a.stdout); err != nil {
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
		if strings.TrimSpace(nextOnCleanup) != "" {
			a.printf("submit: note: --next-on-cleanup was not used because submit did not clean up the current branch\n")
		}
		return nil
	}

	usedNextOnCleanup := false
	for _, branch := range queue {
		meta, ok := state.Branches[branch]
		if !ok {
			continue
		}
		var existingPR *GhPR
		if meta.PR != nil && meta.PR.Number > 0 {
			existing, err := deps.gh.View(meta.PR.Number)
			if err == nil && strings.EqualFold(existing.State, "MERGED") {
				meta.PR.URL = existing.URL
				if existing.BaseRefName != "" {
					meta.PR.Base = existing.BaseRefName
				}
				a.printf("%s -> PR #%d already merged, skipping\n", branch, existing.Number)
				used, err := deps.cleanupMergedBranch(state, branch, nextOnCleanup)
				if err != nil {
					return err
				}
				usedNextOnCleanup = usedNextOnCleanup || used
				continue
			}
			if err == nil {
				existingPR = existing
			}
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		if !deps.git.LocalBranchExists(branch) {
			a.printf("%s -> skipped: local branch no longer exists\n", branch)
			continue
		}
		hasCommits, err := deps.git.BranchHasCommitsSince(parent, branch)
		if err != nil {
			return fmt.Errorf("check submit range %s..%s: %w", parent, branch, err)
		}
		if !hasCommits {
			a.printf("%s -> skipped: no commits beyond %s\n", branch, parent)
			continue
		}
		if err := deps.git.PushBranch(branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
		pr, err := deps.ensurePR(branch, parent, meta.PR, existingPR)
		if err != nil {
			return fmt.Errorf("submit %s: %w", branch, err)
		}
		meta.PR = pr
		a.printf("%s -> PR #%d %s\n", branch, pr.Number, pr.URL)
	}
	if err := deps.syncCurrentStackBody(state, all, branch); err != nil {
		return err
	}
	if strings.TrimSpace(nextOnCleanup) != "" && !usedNextOnCleanup {
		a.printf("submit: note: --next-on-cleanup was not used because submit did not clean up the current branch\n")
	}

	if persisted {
		if err := deps.saveState(repoRoot, state); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) cleanupMergedBranch(state *State, branch string, nextOnCleanup string, git submitGitClient) (bool, error) {
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
		a.printf("%s -> merged and remote deleted, but integration check failed; keeping local branch\n", branch)
		return false, nil
	}
	if !integrated {
		a.printf("%s -> merged and remote deleted, but unmerged local changes detected; keeping local branch\n", branch)
		return false, nil
	}

	if currentErr == nil && current == branch {
		target, proceed, usedNextOnCleanup, err := chooseSubmitCleanupSwitchTarget(state, branch, nextOnCleanup, a.in, a.stdout, git)
		if err != nil {
			return usedNextOnCleanup, err
		}
		if !proceed {
			a.printf("%s -> keeping local merged branch\n", branch)
			return usedNextOnCleanup, nil
		}
		if err := switchAwayThenDeleteMergedBranch(git, branch, true, target); err != nil {
			a.printf("%s -> %v\n", branch, err)
			return usedNextOnCleanup, nil
		}
		if err := cleanupMergedBranchState(a.stdout, state, branch, base); err != nil {
			a.printf("%s -> %v\n", branch, err)
			return usedNextOnCleanup, nil
		}
		a.printf("%s -> deleted local merged branch\n", branch)
		return usedNextOnCleanup, nil
	}

	if err := switchAwayThenDeleteMergedBranch(git, branch, true, ""); err != nil {
		a.printf("%s -> %v\n", branch, err)
		return false, nil
	}
	if err := cleanupMergedBranchState(a.stdout, state, branch, base); err != nil {
		a.printf("%s -> %v\n", branch, err)
		return false, nil
	}
	a.printf("%s -> deleted local merged branch\n", branch)
	return false, nil
}

func chooseSubmitCleanupSwitchTarget(state *State, branch, nextOnCleanup string, in io.Reader, out io.Writer, git submitGitClient) (string, bool, bool, error) {
	if nextOnCleanup != "" {
		target, err := validateSubmitCleanupTarget(branch, nextOnCleanup, git)
		if err != nil {
			return "", false, true, err
		}
		fmt.Fprintf(out, "%s -> using --next-on-cleanup target %s before cleanup\n", branch, target)
		return target, true, true, nil
	}
	target, proceed := promptSwitchTargetForMergedBranchDeletion(state, branch, in, out)
	return target, proceed, false, nil
}

func validateSubmitCleanupTarget(branch, nextOnCleanup string, git submitGitClient) (string, error) {
	target := strings.TrimSpace(nextOnCleanup)
	if target == "" {
		return "", fmt.Errorf("submit --next-on-cleanup requires a branch name")
	}
	if target == branch {
		return "", fmt.Errorf("submit --next-on-cleanup cannot be the branch being cleaned: %s", target)
	}
	if !git.LocalBranchExists(target) {
		return "", fmt.Errorf("submit --next-on-cleanup branch does not exist locally: %s", target)
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
		fmt.Fprintf(out, "%s -> merged and remote deleted; switching to %s before cleanup\n", branch, target)
		return target, true
	}

	reader := bufio.NewReader(in)
	if len(children) == 1 {
		target := children[0]
		fmt.Fprintf(out, "%s -> merged and remote deleted. Switch to %s and delete this branch? [y/N]: ", branch, target)
		answer, err := readPromptLine(reader)
		if err != nil {
			fmt.Fprintf(out, "%s -> failed to read cleanup prompt\n", branch)
			return "", false
		}
		if answer != "y" && answer != "yes" {
			return "", false
		}
		return target, true
	}

	fmt.Fprintf(out, "%s -> merged and remote deleted. Choose branch to switch to before deleting it:\n", branch)
	for i, child := range children {
		fmt.Fprintf(out, "  %d) %s\n", i+1, child)
	}
	fmt.Fprintf(out, "  0) keep local branch\n")
	fmt.Fprintf(out, "selection [0-%d]: ", len(children))
	answer, err := readPromptLine(reader)
	if err != nil {
		fmt.Fprintf(out, "%s -> failed to read cleanup selection\n", branch)
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
