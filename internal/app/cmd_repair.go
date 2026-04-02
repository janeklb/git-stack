package app

func (a *App) cmdRepair() error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, oldState, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	branches, err := listLocalBranches()
	if err != nil {
		return err
	}

	newState := &State{
		Version:     stateVersion,
		Trunk:       oldState.Trunk,
		RestackMode: oldState.RestackMode,
		Naming:      oldState.Naming,
		Branches:    map[string]*BranchRef{},
		Archived:    map[string]*ArchivedRef{},
	}

	for _, branch := range branches {
		if branch == newState.Trunk {
			continue
		}
		parent, err := inferParent(branch, branches, newState.Trunk)
		if err != nil {
			return err
		}
		entry := &BranchRef{Parent: parent, LineageParent: parent}
		if oldMeta, ok := oldState.Branches[branch]; ok && oldMeta.PR != nil {
			entry.PR = oldMeta.PR
		}
		newState.Branches[branch] = entry
	}

	if err := saveState(repoRoot, newState); err != nil {
		return err
	}
	a.println("rebuilt stack metadata from git ancestry")
	return nil
}
