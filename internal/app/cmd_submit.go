package app

import (
	"fmt"
)

func (a *App) cmdSubmit(all bool, branch string) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	args := []string{}
	if branch != "" {
		args = append(args, branch)
	}
	queue, err := submitQueue(state, all, args)
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		fmt.Println("nothing to submit")
		return nil
	}

	for _, branch := range queue {
		meta, ok := state.Branches[branch]
		if !ok {
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		if err := pushBranch(branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
		pr, err := ensurePR(branch, parent, meta.PR)
		if err != nil {
			return fmt.Errorf("submit %s: %w", branch, err)
		}
		meta.PR = pr
		fmt.Printf("%s -> PR #%d %s\n", branch, pr.Number, pr.URL)
	}

	if persisted {
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}
	return nil
}
