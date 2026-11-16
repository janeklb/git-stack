package app

import (
	"errors"
	"strings"
)

func (a *App) cmdInit(trunk, mode, template string, prefixIndex bool) error {
	if err := a.repo.ensureCleanWorktree(); err != nil {
		return err
	}
	if mode != "rebase" && mode != "merge" {
		return errors.New("--mode must be rebase or merge")
	}
	var err error
	detectedTrunk := strings.TrimSpace(trunk)
	if detectedTrunk == "" {
		detectedTrunk, err = a.repo.detectTrunk()
		if err != nil {
			return err
		}
	}

	branches := map[string]*BranchRef{}
	archived := map[string]*ArchivedRef{}
	nextIndex := 1
	if inferred, inferErr := a.inferStateDefaults(); inferErr == nil {
		if inferred.Naming.NextIndex > nextIndex {
			nextIndex = inferred.Naming.NextIndex
		}
	}
	if existing, loadErr := a.repo.metadataStore().loadState(); loadErr == nil {
		branches = existing.Branches
		archived = existing.Archived
		if existing.Naming.NextIndex > nextIndex {
			nextIndex = existing.Naming.NextIndex
		}
	}

	state := &State{
		Version:     stateVersion,
		Trunk:       detectedTrunk,
		RestackMode: mode,
		Naming: NamingConfig{
			Template:    template,
			PrefixIndex: prefixIndex,
			NextIndex:   nextIndex,
		},
		Branches: branches,
		Archived: archived,
	}
	if err := a.repo.metadataStore().saveState(state); err != nil {
		return err
	}
	a.println("note: git-stack init is a repair/reconfiguration command; normal mutating workflows should auto-bootstrap state when possible")
	a.printlnf("initialized stack state (trunk=%s, mode=%s)", detectedTrunk, mode)
	return nil
}
