package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (a *App) cmdReparent(target, newParent string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("usage: stack reparent <branch> --parent <new-parent>")
	}
	if strings.TrimSpace(newParent) == "" {
		return errors.New("--parent is required")
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}

	target = strings.TrimSpace(target)
	newParent = strings.TrimSpace(newParent)
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	meta, ok := state.Branches[target]
	if !ok {
		return fmt.Errorf("branch not tracked in stack: %s", target)
	}
	if !branchExists(newParent) {
		return fmt.Errorf("new parent does not exist: %s", newParent)
	}
	oldParent := meta.Parent
	if oldParent == "" {
		oldParent = state.Trunk
	}
	if oldParent == newParent {
		a.println("branch already has requested parent")
		return nil
	}
	if err := validateReparentParent(state, target, newParent); err != nil {
		return err
	}

	if err := gitRun("switch", target); err != nil {
		return err
	}
	if err := gitRun("rebase", "--onto", newParent, oldParent); err != nil {
		return fmt.Errorf("rebase during reparent failed: %w", err)
	}

	meta.Parent = newParent
	meta.LineageParent = newParent
	if err := saveState(repoRoot, state); err != nil {
		return err
	}

	if meta.PR != nil {
		if err := ghRun("pr", "edit", strconv.Itoa(meta.PR.Number), "--base", newParent); err != nil {
			return fmt.Errorf("updated branch parent but failed to update PR base: %w", err)
		}
		meta.PR.Base = newParent
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}

	a.printf("reparented %s: %s -> %s\n", target, oldParent, newParent)
	return nil
}

func validateReparentParent(state *State, target, newParent string) error {
	if newParent == target {
		return fmt.Errorf("invalid parent %q for %q: branch cannot parent itself", newParent, target)
	}
	if newParent == state.Trunk {
		return nil
	}
	if !isDescendantBranch(state, target, newParent) {
		return nil
	}
	return fmt.Errorf("invalid parent %q for %q: parent cannot be a descendant", newParent, target)
}

func isDescendantBranch(state *State, root, candidate string) bool {
	children := map[string][]string{}
	for name, meta := range state.Branches {
		if meta == nil {
			continue
		}
		children[meta.Parent] = append(children[meta.Parent], name)
	}

	seen := map[string]bool{}
	stack := []string{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range children[node] {
			if child == candidate {
				return true
			}
			if seen[child] {
				continue
			}
			seen[child] = true
			stack = append(stack, child)
		}
	}
	return false
}
