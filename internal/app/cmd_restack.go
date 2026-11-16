package app

import (
	"errors"
	"strings"
)

func (a *App) cmdRestack(mode string, cont, abort bool) error {
	if cont && abort {
		return errors.New("--continue and --abort are mutually exclusive")
	}
	state, persisted, err := a.loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	if abort {
		return a.abortRestack(a.stdout)
	}
	if cont {
		return a.continueRestack(state, a.stdout)
	}

	if err := a.repo.ensureCleanWorktree(); err != nil {
		return err
	}
	if err := requirePersistedTrackedState(state, persisted, "restack"); err != nil {
		return err
	}
	op, _ := a.repo.metadataStore().loadOperation()
	if op != nil {
		active, err := a.restackGitOperationInProgress(op.Mode)
		if err != nil {
			return err
		}
		if active {
			return errors.New("a restack operation is already in progress; use git-stack restack --continue or --abort")
		}
		return a.runRestack(state, op, false, a.stdout)
	}

	chosenMode := state.RestackMode
	if strings.TrimSpace(mode) != "" {
		chosenMode = strings.TrimSpace(mode)
	}
	queue, err := topoOrder(state)
	if err != nil {
		return err
	}
	return a.runRestackQueue(state, chosenMode, queue, nil, a.stdout)
}
