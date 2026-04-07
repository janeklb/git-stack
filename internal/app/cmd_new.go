package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func adoptExistingBranch(state *State, branch, parent string) error {
	branch = strings.TrimSpace(branch)
	parent = strings.TrimSpace(parent)
	if branch == "" {
		return errors.New("branch name cannot be empty")
	}
	if parent == "" {
		return errors.New("parent branch cannot be empty")
	}
	state.Branches[branch] = &BranchRef{Parent: parent, LineageParent: parent}
	return nil
}

func (a *App) cmdNew(name, parent, template string, prefixIndex bool, adopt bool) error {
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
	if name == "" && !adopt {
		name = fmt.Sprintf("change-%d", time.Now().Unix())
	}

	parentBranch := strings.TrimSpace(parent)
	parentProvided := parentBranch != ""
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
	if adopt {
		branchName := cur
		if strings.TrimSpace(name) != "" {
			if strings.TrimSpace(name) != strings.TrimSpace(cur) {
				return fmt.Errorf("adopt expects the current branch or no explicit name, got: %s", name)
			}
			branchName = strings.TrimSpace(name)
		}
		if !parentProvided {
			branches, err := listLocalBranches()
			if err != nil {
				return err
			}
			parentBranch, err = inferParent(branchName, branches, state.Trunk)
			if err != nil {
				return err
			}
		}
		if _, tracked := state.Branches[branchName]; tracked {
			return fmt.Errorf("branch already tracked in stack: %s", branchName)
		}
		if err := adoptExistingBranch(state, branchName, parentBranch); err != nil {
			return err
		}
		if !persisted {
			a.printf("initialized stack state (trunk=%s, mode=%s)\n", state.Trunk, state.RestackMode)
		}
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
		a.printf("adopted %s (parent=%s)\n", branchName, parentBranch)
		return nil
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
