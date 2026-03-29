package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

func (a *App) cmdSubmit(all bool, branch string) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	args := []string{}
	if branch != "" {
		args = append(args, branch)
	}
	queue, err := submitQueue(state, all, args)
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		fmt.Println("nothing to submit")
		return nil
	}

	for _, branch := range queue {
		meta, ok := state.Branches[branch]
		if !ok {
			continue
		}
		if meta.PR != nil && meta.PR.Number > 0 {
			existing, err := ghView(meta.PR.Number)
			if err == nil && strings.EqualFold(existing.State, "MERGED") {
				meta.PR.URL = existing.URL
				if existing.BaseRefName != "" {
					meta.PR.Base = existing.BaseRefName
				}
				fmt.Printf("%s -> PR #%d already merged, skipping\n", branch, existing.Number)
				a.cleanupMergedBranch(state, branch)
				continue
			}
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		if err := pushBranch(branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
		pr, err := ensurePR(branch, parent, meta.PR)
		if err != nil {
			return fmt.Errorf("submit %s: %w", branch, err)
		}
		meta.PR = pr
		fmt.Printf("%s -> PR #%d %s\n", branch, pr.Number, pr.URL)
	}
	if err := syncCurrentStackBodies(state, all, branch); err != nil {
		return err
	}

	if persisted {
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) cleanupMergedBranch(state *State, branch string) {
	remoteExists, remoteErr := remoteBranchExists(branch)
	if remoteErr != nil {
		return
	}
	if remoteExists {
		return
	}

	current, currentErr := currentBranch()
	base := state.Trunk
	if meta := state.Branches[branch]; meta != nil {
		if meta.PR != nil && strings.TrimSpace(meta.PR.Base) != "" {
			base = meta.PR.Base
		} else if strings.TrimSpace(meta.Parent) != "" {
			base = meta.Parent
		}
	}

	integrated, integratedErr := branchFullyIntegrated(branch, base)
	if integratedErr != nil {
		fmt.Printf("%s -> merged and remote deleted, but integration check failed; keeping local branch\n", branch)
		return
	}
	if !integrated {
		fmt.Printf("%s -> merged and remote deleted, but unmerged local changes detected; keeping local branch\n", branch)
		return
	}

	if currentErr == nil && current == branch {
		target, proceed := promptSwitchTargetForMergedBranchDeletion(state, branch)
		if !proceed {
			fmt.Printf("%s -> keeping local merged branch\n", branch)
			return
		}
		if err := gitRun("switch", target); err != nil {
			fmt.Printf("%s -> failed to switch to %s before deletion: %v\n", branch, target, err)
			return
		}
	}

	if err := deleteLocalBranch(branch); err != nil {
		fmt.Printf("%s -> failed to delete local merged branch: %v\n", branch, err)
		return
	}
	archiveMergedBranch(state, branch)
	delete(state.Branches, branch)
	pruneArchivedLineage(state)
	fmt.Printf("%s -> deleted local merged branch\n", branch)
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

func promptSwitchTargetForMergedBranchDeletion(state *State, branch string) (string, bool) {
	children := mergedBranchChildren(state, branch)
	if len(children) == 0 {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		fmt.Printf("%s -> merged and remote deleted; switching to %s before cleanup\n", branch, target)
		return target, true
	}

	reader := bufio.NewReader(os.Stdin)
	if len(children) == 1 {
		target := children[0]
		fmt.Printf("%s -> merged and remote deleted. Switch to %s and delete this branch? [y/N]: ", branch, target)
		answer, err := readPromptLine(reader)
		if err != nil {
			fmt.Printf("%s -> failed to read cleanup prompt\n", branch)
			return "", false
		}
		if answer != "y" && answer != "yes" {
			return "", false
		}
		return target, true
	}

	fmt.Printf("%s -> merged and remote deleted. Choose branch to switch to before deleting it:\n", branch)
	for i, child := range children {
		fmt.Printf("  %d) %s\n", i+1, child)
	}
	fmt.Printf("  0) keep local branch\n")
	fmt.Printf("selection [0-%d]: ", len(children))
	answer, err := readPromptLine(reader)
	if err != nil {
		fmt.Printf("%s -> failed to read cleanup selection\n", branch)
		return "", false
	}
	choice, err := strconv.Atoi(answer)
	if err != nil || choice < 0 || choice > len(children) {
		fmt.Printf("%s -> invalid selection, keeping local branch\n", branch)
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

func readPromptLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(line)), nil
}
