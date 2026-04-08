package app

import (
	"errors"
	"fmt"
	"io"
)

func runRestackQueue(repoRoot string, state *State, mode string, queue []string, rebaseBases map[string]string, out io.Writer) error {
	chosenMode := mode
	if chosenMode != "rebase" && chosenMode != "merge" {
		return errors.New("restack mode must be rebase or merge")
	}

	if len(queue) == 0 {
		fmt.Fprintln(out, "nothing to restack")
		return nil
	}
	original, err := currentBranch()
	if err != nil {
		return err
	}
	originalHeads := map[string]string{}
	for _, branch := range queue {
		head, headErr := resolveBranchRef(branch)
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
		RebaseBases:    rebaseBases,
	}
	if err := saveOperation(repoRoot, op); err != nil {
		return err
	}
	return runRestack(repoRoot, state, op, false, out)
}

func restackGitOperationInProgress(mode string) (bool, error) {
	if mode == "merge" {
		return mergeInProgress()
	}
	return rebaseInProgress()
}

func runRestack(repoRoot string, state *State, op *RestackOperation, fromContinue bool, out io.Writer) error {
	if fromContinue {
		contArgs := []string{op.Mode, "--continue"}
		if op.Mode == "merge" {
			contArgs = []string{"merge", "--continue"}
		}
		if err := gitRun(contArgs...); err != nil {
			return fmt.Errorf("failed to continue %s: %w", op.Mode, err)
		}
		active, err := restackGitOperationInProgress(op.Mode)
		if err != nil {
			return err
		}
		completed, err := recordRestackContinueProgress(repoRoot, op, !active)
		if err != nil {
			return err
		}
		if !completed {
			fmt.Fprintf(out, "%s still in progress on %s; resolve remaining steps then run stack restack --continue again\n", op.Mode, op.Queue[op.Index])
			return nil
		}
	}

	for op.Index < len(op.Queue) {
		branch := op.Queue[op.Index]
		meta := state.Branches[branch]
		if meta == nil {
			op.Index++
			_ = saveOperation(repoRoot, op)
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}

		if err := gitRunQuiet("switch", branch); err != nil {
			return err
		}

		oldBase := ""
		if op.RebaseBases != nil {
			oldBase = op.RebaseBases[branch]
		}
		if oldBase == "" && op.OriginalHeads != nil {
			oldBase = op.OriginalHeads[parent]
		}

		if err := restackBranch(op.Mode, parent, oldBase); err != nil {
			if saveErr := saveOperation(repoRoot, op); saveErr != nil {
				return saveErr
			}
			return fmt.Errorf("%s %s onto %s stopped for conflicts; resolve then run stack restack --continue or --abort", op.Mode, branch, parent)
		}

		op.Index++
		if err := saveOperation(repoRoot, op); err != nil {
			return err
		}
	}

	if err := gitRunQuiet("switch", op.OriginalBranch); err != nil {
		return err
	}
	if err := removeOperation(repoRoot); err != nil {
		return err
	}
	fmt.Fprintln(out, "restack completed")
	return nil
}

func recordRestackContinueProgress(repoRoot string, op *RestackOperation, completed bool) (bool, error) {
	if completed {
		op.Index++
	}
	if err := saveOperation(repoRoot, op); err != nil {
		return false, err
	}
	return completed, nil
}

func continueRestack(repoRoot string, state *State, out io.Writer) error {
	op, err := loadOperation(repoRoot)
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New("no restack operation in progress")
	}
	active, err := restackGitOperationInProgress(op.Mode)
	if err != nil {
		return err
	}
	if !active {
		if op.Index < len(op.Queue) {
			current, curErr := currentBranch()
			if curErr == nil && current == op.Queue[op.Index] {
				op.Index++
				if err := saveOperation(repoRoot, op); err != nil {
					return err
				}
			}
		}
		return runRestack(repoRoot, state, op, false, out)
	}
	return runRestack(repoRoot, state, op, true, out)
}

func abortRestack(repoRoot string, out io.Writer) error {
	op, err := loadOperation(repoRoot)
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New("no restack operation in progress")
	}
	if op.Mode == "merge" {
		_ = gitRun("merge", "--abort")
	} else {
		_ = gitRun("rebase", "--abort")
	}
	if op.OriginalBranch != "" {
		_ = gitRunQuiet("switch", op.OriginalBranch)
	}
	if err := removeOperation(repoRoot); err != nil {
		return err
	}
	fmt.Fprintln(out, "restack aborted")
	return nil
}

func restackBranch(mode, parent, oldBase string) error {
	if mode == "merge" {
		return gitRun("merge", "--no-edit", "--no-ff", parent)
	}
	if oldBase != "" {
		return gitRun("rebase", "--onto", parent, oldBase)
	}
	return gitRun("rebase", parent)
}
