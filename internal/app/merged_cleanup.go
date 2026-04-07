package app

import (
	"fmt"
	"io"
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
		return err
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

func cleanupMergedBranchState(out io.Writer, state *State, branch, replacementParent string) {
	archiveMergedBranch(state, branch)
	reparentChildrenAfterMergedDeletion(state, branch, replacementParent, out)
	delete(state.Branches, branch)
	pruneArchivedLineage(state)
	fmt.Fprintf(out, "%s -> cleaned merged branch from local stack state\n", branch)
}
