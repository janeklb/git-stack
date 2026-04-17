package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (a *App) cmdReparent(target, newParent string, preserveLineage bool) error {
	if strings.TrimSpace(target) == "" {
		current, err := currentBranch()
		if err != nil {
			return err
		}
		target = current
	}
	if strings.TrimSpace(newParent) == "" {
		return errors.New("--onto is required")
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}

	target = strings.TrimSpace(target)
	newParent = strings.TrimSpace(newParent)
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	if err := requirePersistedTrackedState(state, persisted, "reparent"); err != nil {
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

	if err := gitRunQuiet("switch", target); err != nil {
		return err
	}
	if err := gitRun("rebase", "--onto", newParent, oldParent); err != nil {
		return fmt.Errorf("rebase during reparent failed: %w", err)
	}

	meta.Parent = newParent
	if !preserveLineage {
		meta.LineageParent = newParent
	}
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

	a.printlnf("reparented %s: %s -> %s", target, oldParent, newParent)
	return nil
}
