package app

import (
	"errors"
	"strings"
)

func (a *App) cmdRestack(mode string, cont, abort bool) error {
	if cont && abort {
		return errors.New("--continue and --abort are mutually exclusive")
	}
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	if abort {
		return abortRestack(repoRoot, a.stdout)
	}
	if cont {
		return continueRestack(repoRoot, state, a.stdout)
	}

	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	if _, err := ensurePersistedState(repoRoot, state, persisted, a.stdout); err != nil {
		return err
	}
	op, _ := loadOperation(repoRoot)
	if op != nil {
		active, err := restackGitOperationInProgress(op.Mode)
		if err != nil {
			return err
		}
		if active {
			return errors.New("a restack operation is already in progress; use stack restack --continue or --abort")
		}
		return runRestack(repoRoot, state, op, false, a.stdout)
	}

	chosenMode := state.RestackMode
	if strings.TrimSpace(mode) != "" {
		chosenMode = strings.TrimSpace(mode)
	}
	return runRestackQueue(repoRoot, state, chosenMode, topoOrder(state), nil, a.stdout)
}
