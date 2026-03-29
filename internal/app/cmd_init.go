package app

import (
	"errors"
	"fmt"
	"strings"
)

func (a *App) cmdInit(trunk, mode, template string, prefixIndex bool) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	if mode != "rebase" && mode != "merge" {
		return errors.New("--mode must be rebase or merge")
	}
	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}
	detectedTrunk := strings.TrimSpace(trunk)
	if detectedTrunk == "" {
		detectedTrunk, err = detectTrunk()
		if err != nil {
			return err
		}
	}

	branches := map[string]*BranchRef{}
	archived := map[string]*ArchivedRef{}
	nextIndex := 1
	if inferred, inferErr := inferState(repoRoot); inferErr == nil {
		branches = inferred.Branches
		archived = inferred.Archived
		if inferred.Naming.NextIndex > nextIndex {
			nextIndex = inferred.Naming.NextIndex
		}
	}
	if existing, loadErr := loadState(repoRoot); loadErr == nil {
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
	if err := saveState(repoRoot, state); err != nil {
		return err
	}
	fmt.Printf("initialized stack state (trunk=%s, mode=%s)\n", detectedTrunk, mode)
	return nil
}
