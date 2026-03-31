package app

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type submitDeps struct {
	git                  submitGitClient
	gh                   submitGHClient
	ensureCleanWorktree  func() error
	loadState            func() (string, *State, bool, error)
	submitQueue          func(*State, bool, []string) ([]string, error)
	ensurePR             func(string, string, *PRMeta) (*PRMeta, error)
	syncCurrentStackBody func(*State, bool, string) error
	saveState            func(string, *State) error
	cleanupMergedBranch  func(*State, string)
}

func (a *App) defaultSubmitDeps() submitDeps {
	git := defaultGitBoundary{}
	return submitDeps{
		git:                  git,
		gh:                   defaultGHBoundary{},
		ensureCleanWorktree:  ensureCleanWorktree,
		loadState:            loadStateFromRepoOrInfer,
		submitQueue:          submitQueue,
		ensurePR:             ensurePR,
		syncCurrentStackBody: syncCurrentStackBodies,
		saveState:            saveState,
		cleanupMergedBranch: func(state *State, branch string) {
			a.cleanupMergedBranch(state, branch, git, a.in, a.stdout)
		},
	}
}

func (a *App) cmdSubmit(all bool, branch string) error {
	deps := a.defaultSubmitDeps()
	return a.cmdSubmitWithDeps(all, branch, deps)
}

func (a *App) cmdSubmitWithDeps(all bool, branch string, deps submitDeps) error {
	if err := deps.ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, persisted, err := deps.loadState()
	if err != nil {
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
		return nil
	}

	for _, branch := range queue {
		meta, ok := state.Branches[branch]
		if !ok {
			continue
		}
		if meta.PR != nil && meta.PR.Number > 0 {
			existing, err := deps.gh.View(meta.PR.Number)
			if err == nil && strings.EqualFold(existing.State, "MERGED") {
				meta.PR.URL = existing.URL
				if existing.BaseRefName != "" {
					meta.PR.Base = existing.BaseRefName
				}
				a.printf("%s -> PR #%d already merged, skipping\n", branch, existing.Number)
				deps.cleanupMergedBranch(state, branch)
				continue
			}
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		if err := deps.git.PushBranch(branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
		pr, err := deps.ensurePR(branch, parent, meta.PR)
		if err != nil {
			return fmt.Errorf("submit %s: %w", branch, err)
		}
		meta.PR = pr
		a.printf("%s -> PR #%d %s\n", branch, pr.Number, pr.URL)
	}
	if err := deps.syncCurrentStackBody(state, all, branch); err != nil {
		return err
	}

	if persisted {
		if err := deps.saveState(repoRoot, state); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) cleanupMergedBranch(state *State, branch string, git submitGitClient, in io.Reader, out io.Writer) {
	remoteExists, remoteErr := git.RemoteBranchExists(branch)
	if remoteErr != nil {
		return
	}
	if remoteExists {
		return
	}

	current, currentErr := git.CurrentBranch()
	base := state.Trunk
	if meta := state.Branches[branch]; meta != nil {
		if meta.PR != nil && strings.TrimSpace(meta.PR.Base) != "" {
			base = meta.PR.Base
		} else if strings.TrimSpace(meta.Parent) != "" {
			base = meta.Parent
		}
	}

	integrated, integratedErr := git.BranchFullyIntegrated(branch, base)
	if integratedErr != nil {
		_, _ = fmt.Fprintf(out, "%s -> merged and remote deleted, but integration check failed; keeping local branch\n", branch)
		return
	}
	if !integrated {
		_, _ = fmt.Fprintf(out, "%s -> merged and remote deleted, but unmerged local changes detected; keeping local branch\n", branch)
		return
	}

	if currentErr == nil && current == branch {
		target, proceed := promptSwitchTargetForMergedBranchDeletion(state, branch, in, out)
		if !proceed {
			_, _ = fmt.Fprintf(out, "%s -> keeping local merged branch\n", branch)
			return
		}
		if err := git.Run("switch", target); err != nil {
			_, _ = fmt.Fprintf(out, "%s -> failed to switch to %s before deletion: %v\n", branch, target, err)
			return
		}
	}

	if err := git.DeleteLocalBranch(branch); err != nil {
		_, _ = fmt.Fprintf(out, "%s -> failed to delete local merged branch: %v\n", branch, err)
		return
	}
	archiveMergedBranch(state, branch)
	reparentChildrenAfterMergedDeletion(state, branch, base, out)
	delete(state.Branches, branch)
	pruneArchivedLineage(state)
	_, _ = fmt.Fprintf(out, "%s -> deleted local merged branch\n", branch)
}

func archiveMergedBranch(state *State, branch string) {
	meta := state.Branches[branch]
	if meta == nil {
		return
	}
	if state.Archived == nil {
		state.Archived = map[string]*ArchivedRef{}
	}
	parent := strings.TrimSpace(meta.LineageParent)
	if parent == "" {
		parent = strings.TrimSpace(meta.Parent)
	}
	state.Archived[branch] = &ArchivedRef{Parent: parent, PR: meta.PR}
}

func pruneArchivedLineage(state *State) {
	if len(state.Archived) == 0 {
		return
	}
	keep := map[string]bool{}
	for _, meta := range state.Branches {
		lineageParent := strings.TrimSpace(meta.LineageParent)
		if lineageParent == "" {
			lineageParent = strings.TrimSpace(meta.Parent)
		}
		cur := lineageParent
		seen := map[string]bool{}
		for cur != "" && cur != state.Trunk {
			if seen[cur] {
				break
			}
			seen[cur] = true
			archived, ok := state.Archived[cur]
			if !ok || archived == nil {
				break
			}
			keep[cur] = true
			cur = strings.TrimSpace(archived.Parent)
		}
	}
	for branch := range state.Archived {
		if keep[branch] {
			continue
		}
		delete(state.Archived, branch)
	}
}

func reparentChildrenAfterMergedDeletion(state *State, deletedBranch, replacementParent string, out io.Writer) {
	if strings.TrimSpace(replacementParent) == "" {
		replacementParent = state.Trunk
	}
	for name, meta := range state.Branches {
		if name == deletedBranch || meta == nil {
			continue
		}
		if meta.Parent != deletedBranch {
			continue
		}
		meta.Parent = replacementParent
		if meta.PR != nil {
			meta.PR.Base = replacementParent
		}
		_, _ = fmt.Fprintf(out, "%s -> reparented to %s after merged parent cleanup\n", name, replacementParent)
	}
}

func promptSwitchTargetForMergedBranchDeletion(state *State, branch string, in io.Reader, out io.Writer) (string, bool) {
	children := mergedBranchChildren(state, branch)
	if len(children) == 0 {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		_, _ = fmt.Fprintf(out, "%s -> merged and remote deleted; switching to %s before cleanup\n", branch, target)
		return target, true
	}

	reader := bufio.NewReader(in)
	if len(children) == 1 {
		target := children[0]
		_, _ = fmt.Fprintf(out, "%s -> merged and remote deleted. Switch to %s and delete this branch? [y/N]: ", branch, target)
		answer, err := readPromptLine(reader)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s -> failed to read cleanup prompt\n", branch)
			return "", false
		}
		if answer != "y" && answer != "yes" {
			return "", false
		}
		return target, true
	}

	_, _ = fmt.Fprintf(out, "%s -> merged and remote deleted. Choose branch to switch to before deleting it:\n", branch)
	for i, child := range children {
		_, _ = fmt.Fprintf(out, "  %d) %s\n", i+1, child)
	}
	_, _ = fmt.Fprintf(out, "  0) keep local branch\n")
	_, _ = fmt.Fprintf(out, "selection [0-%d]: ", len(children))
	answer, err := readPromptLine(reader)
	if err != nil {
		_, _ = fmt.Fprintf(out, "%s -> failed to read cleanup selection\n", branch)
		return "", false
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 0 || choice > len(children) {
		_, _ = fmt.Fprintf(out, "%s -> invalid selection, keeping local branch\n", branch)
		return "", false
	}
	if choice == 0 {
		return "", false
	}
	return children[choice-1], true
}

func mergedBranchChildren(state *State, branch string) []string {
	children := []string{}
	for name, meta := range state.Branches {
		if name == branch || meta == nil {
			continue
		}
		if meta.Parent == branch {
			children = append(children, name)
		}
	}
	sort.Strings(children)
	return children
}
