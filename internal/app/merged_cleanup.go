package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type mergedCleanupGitClient interface {
	CurrentBranch() (string, error)
	Run(args ...string) error
	DeleteLocalBranch(branch string) error
}

func switchAwayThenDeleteMergedBranch(git mergedCleanupGitClient, branch string, hasLocal bool, switchTarget string) error {
	current, err := git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to determine current branch before cleanup of %s: %w", branch, err)
	}
	if current == branch {
		switchTarget = strings.TrimSpace(switchTarget)
		if switchTarget == "" {
			return fmt.Errorf("no switch target available before cleanup of %s", branch)
		}
		if err := git.Run("switch", switchTarget); err != nil {
			return fmt.Errorf("failed to switch to %s before cleanup: %w", switchTarget, err)
		}
	}

	if !hasLocal {
		return nil
	}
	if err := git.DeleteLocalBranch(branch); err != nil {
		return fmt.Errorf("failed to delete local merged branch %s: %w", branch, err)
	}
	return nil
}

func cleanupMergedBranchState(out io.Writer, state *State, branch, replacementParent string) error {
	if state == nil {
		return fmt.Errorf("cleanup requires stack state for %s", branch)
	}
	if state.Branches[branch] == nil {
		return fmt.Errorf("cleanup requires tracked branch metadata for %s", branch)
	}
	archiveMergedBranch(state, branch)
	reparentChildrenAfterMergedDeletion(state, branch, replacementParent, out)
	delete(state.Branches, branch)
	pruneArchivedLineage(state)
	if _, err := fmt.Fprintf(out, "%s -> cleaned merged branch from local stack state\n", branch); err != nil {
		return fmt.Errorf("failed to write cleanup output for %s: %w", branch, err)
	}
	return nil
}

func archiveMergedBranch(state *State, branch string) {
	meta := state.Branches[branch]
	if meta == nil {
		return
	}
	if state.Archived == nil {
		state.Archived = map[string]*ArchivedRef{}
	}
	parent := meta.LineageParent
	if parent == "" {
		parent = meta.Parent
	}
	state.Archived[branch] = &ArchivedRef{Parent: parent, PR: meta.PR}
}

func pruneArchivedLineage(state *State) {
	if len(state.Archived) == 0 {
		return
	}
	keep := map[string]bool{}
	for _, meta := range state.Branches {
		lineageParent := meta.LineageParent
		if lineageParent == "" {
			lineageParent = meta.Parent
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
			cur = archived.Parent
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
	if replacementParent == "" {
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
		fmt.Fprintf(out, "%s -> reparented to %s after merged parent cleanup\n", name, replacementParent)
	}
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
