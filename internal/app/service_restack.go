package app

import (
	"errors"
	"fmt"
	"io"
)

func (a *App) runRestackQueue(state *State, mode string, queue []string, rebaseBases map[string]string, out io.Writer) error {
	chosenMode := mode
	if chosenMode != "rebase" && chosenMode != "merge" {
		return errors.New("restack mode must be rebase or merge")
	}

	if len(queue) == 0 {
		fmt.Fprintln(out, "nothing to restack")
		return nil
	}
	original, err := a.repo.CurrentBranch()
	if err != nil {
		return err
	}
	originalHeads := map[string]string{}
	for _, branch := range queue {
		head, headErr := a.repo.resolveBranchRef(branch)
		if headErr != nil {
			return headErr
		}
		originalHeads[branch] = head
	}
	op := &RestackOperation{
		Type:           "restack",
		Mode:           chosenMode,
		OriginalBranch: original,
		Queue:          queue,
		Index:          0,
		OriginalHeads:  originalHeads,
		RebaseBases:    mergedRebaseBases(state, queue, rebaseBases),
	}
	if err := a.repo.metadataStore().saveOperation(op); err != nil {
		return err
	}
	return a.runRestack(state, op, false, out)
}

func (a *App) restackGitOperationInProgress(mode string) (bool, error) {
	if mode == "merge" {
		return a.repo.mergeInProgress()
	}
	return a.repo.rebaseInProgress()
}

func (a *App) runRestack(state *State, op *RestackOperation, fromContinue bool, out io.Writer) error {
	if fromContinue {
		continueEnv := []string{"GIT_EDITOR=true"}
		contArgs := []string{op.Mode, "--continue"}
		if op.Mode == "merge" {
			continueEnv = append(continueEnv, "GIT_MERGE_AUTOEDIT=no")
			contArgs = []string{"merge", "--continue"}
		}
		if err := a.repo.gitRunWithEnv(continueEnv, contArgs...); err != nil {
			return fmt.Errorf("failed to continue %s: %w", op.Mode, err)
		}
		active, err := a.restackGitOperationInProgress(op.Mode)
		if err != nil {
			return err
		}
		completedBranch := ""
		if !active && op.Index < len(op.Queue) {
			completedBranch = op.Queue[op.Index]
		}
		completed, err := a.recordRestackContinueProgress(op, !active)
		if err != nil {
			return err
		}
		if completed && clearPendingRebaseBase(state, completedBranch) {
			if err := a.repo.metadataStore().saveState(state); err != nil {
				return err
			}
		}
		if !completed {
			fmt.Fprintf(out, "%s still in progress on %s; resolve remaining steps then run git-stack restack --continue again\n", op.Mode, op.Queue[op.Index])
			return nil
		}
	}

	for op.Index < len(op.Queue) {
		branch := op.Queue[op.Index]
		meta := state.Branches[branch]
		if meta == nil {
			op.Index++
			_ = a.repo.metadataStore().saveOperation(op)
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}

		if err := a.repo.gitRunQuiet("switch", branch); err != nil {
			return err
		}

		oldBase := ""
		if op.RebaseBases != nil {
			oldBase = op.RebaseBases[branch]
		}
		if oldBase == "" && op.OriginalHeads != nil {
			oldBase = op.OriginalHeads[parent]
		}

		if err := a.restackBranch(op.Mode, parent, oldBase); err != nil {
			if saveErr := a.repo.metadataStore().saveOperation(op); saveErr != nil {
				return saveErr
			}
			return fmt.Errorf("%s %s onto %s stopped for conflicts; resolve then run git-stack restack --continue or --abort", op.Mode, branch, parent)
		}
		if clearPendingRebaseBase(state, branch) {
			if err := a.repo.metadataStore().saveState(state); err != nil {
				return err
			}
		}

		op.Index++
		if err := a.repo.metadataStore().saveOperation(op); err != nil {
			return err
		}
	}

	if err := a.repo.gitRunQuiet("switch", op.OriginalBranch); err != nil {
		return err
	}
	if err := a.repo.metadataStore().removeOperation(); err != nil {
		return err
	}
	fmt.Fprintln(out, "restack completed")
	return nil
}

func (a *App) recordRestackContinueProgress(op *RestackOperation, completed bool) (bool, error) {
	if completed {
		op.Index++
	}
	if err := a.repo.metadataStore().saveOperation(op); err != nil {
		return false, err
	}
	return completed, nil
}

func (a *App) continueRestack(state *State, out io.Writer) error {
	op, err := a.repo.metadataStore().loadOperation()
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New("no restack operation in progress")
	}
	active, err := a.restackGitOperationInProgress(op.Mode)
	if err != nil {
		return err
	}
	if !active {
		if op.Index < len(op.Queue) {
			current, curErr := a.repo.CurrentBranch()
			if curErr == nil && current == op.Queue[op.Index] {
				op.Index++
				if err := a.repo.metadataStore().saveOperation(op); err != nil {
					return err
				}
			}
		}
		return a.runRestack(state, op, false, out)
	}
	return a.runRestack(state, op, true, out)
}

func (a *App) abortRestack(out io.Writer) error {
	op, err := a.repo.metadataStore().loadOperation()
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New("no restack operation in progress")
	}
	if op.Mode == "merge" {
		_ = a.repo.Run("merge", "--abort")
	} else {
		_ = a.repo.Run("rebase", "--abort")
	}
	if op.OriginalBranch != "" {
		_ = a.repo.gitRunQuiet("switch", op.OriginalBranch)
	}
	if err := a.repo.metadataStore().removeOperation(); err != nil {
		return err
	}
	fmt.Fprintln(out, "restack aborted")
	return nil
}

func (a *App) restackBranch(mode, parent, oldBase string) error {
	if mode == "merge" {
		return a.repo.Run("merge", "--no-edit", "--no-ff", parent)
	}
	if oldBase != "" {
		return a.repo.Run("rebase", "--onto", parent, oldBase)
	}
	return a.repo.Run("rebase", parent)
}

func mergedRebaseBases(state *State, queue []string, explicit map[string]string) map[string]string {
	merged := map[string]string{}
	for _, branch := range queue {
		meta := state.Branches[branch]
		if meta == nil || meta.PendingRebaseBase == "" {
			continue
		}
		merged[branch] = meta.PendingRebaseBase
	}
	for branch, base := range explicit {
		if branch == "" || base == "" || merged[branch] != "" {
			continue
		}
		merged[branch] = base
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func clearPendingRebaseBase(state *State, branch string) bool {
	if state == nil {
		return false
	}
	meta := state.Branches[branch]
	if meta == nil || meta.PendingRebaseBase == "" {
		return false
	}
	meta.PendingRebaseBase = ""
	return true
}
