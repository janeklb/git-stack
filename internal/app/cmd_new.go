package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (a *App) cmdNew(name, parent, template string, prefixIndex bool) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	if !hasHeadCommit() {
		return errors.New("repository has no commits yet; create an initial commit first (for example: git commit --allow-empty -m \"initial commit\")")
	}
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("change-%d", time.Now().Unix())
	}

	parentBranch := strings.TrimSpace(parent)
	if parentBranch == "" {
		parentBranch, err = currentBranch()
		if err != nil {
			return err
		}
	}

	if !branchExists(parentBranch) {
		return fmt.Errorf("parent branch does not exist: %s", parentBranch)
	}
	cur, err := currentBranch()
	if err != nil {
		return err
	}
	if parentBranch == cur && cur != state.Trunk {
		if _, tracked := state.Branches[cur]; !tracked {
			branches, err := listLocalBranches()
			if err != nil {
				return err
			}
			inferredParent, err := inferParent(cur, branches, state.Trunk)
			if err != nil {
				return err
			}
			state.Branches[cur] = &BranchRef{Parent: inferredParent, LineageParent: inferredParent}
		}
	}

	slug := slugify(name)
	if slug == "" {
		return errors.New("branch name cannot be empty")
	}

	chosenTemplate := state.Naming.Template
	if template != "" {
		chosenTemplate = template
	}
	usePrefixIndex := state.Naming.PrefixIndex || prefixIndex
	branchName := applyTemplate(chosenTemplate, slug, state.Naming.NextIndex, usePrefixIndex)

	if branchExists(branchName) {
		return fmt.Errorf("branch already exists: %s", branchName)
	}

	if cur != parentBranch {
		if err := gitRunQuiet("switch", parentBranch); err != nil {
			return err
		}
	}
	if err := gitRunQuiet("switch", "-c", branchName); err != nil {
		return err
	}

	state.Branches[branchName] = &BranchRef{Parent: parentBranch, LineageParent: parentBranch}
	state.Naming.NextIndex++
	if !persisted {
		a.printf("initialized stack state (trunk=%s, mode=%s)\n", state.Trunk, state.RestackMode)
	}
	if err := saveState(repoRoot, state); err != nil {
		return err
	}
	a.printf("created %s (parent=%s)\n", branchName, parentBranch)
	return nil
}
