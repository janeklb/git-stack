package app

import (
	"errors"
	"fmt"
	"io"
)

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
		op.Index++
		if err := saveOperation(repoRoot, op); err != nil {
			return err
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

		if err := gitRun("switch", branch); err != nil {
			return err
		}

		if err := restackBranch(op.Mode, parent); err != nil {
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

	if err := gitRun("switch", op.OriginalBranch); err != nil {
		return err
	}
	if err := removeOperation(repoRoot); err != nil {
		return err
	}
	fmt.Fprintln(out, "restack completed")
	return nil
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
		_ = gitRun("switch", op.OriginalBranch)
	}
	if err := removeOperation(repoRoot); err != nil {
		return err
	}
	fmt.Fprintln(out, "restack aborted")
	return nil
}

func restackBranch(mode, parent string) error {
	if mode == "merge" {
		return gitRun("merge", "--no-edit", "--no-ff", parent)
	}
	return gitRun("rebase", parent)
}
