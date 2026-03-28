package app

import (
	"flag"
	"fmt"
	"os"
)

func (a *App) cmdSubmit(args []string) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	all := fs.Bool("all", false, "submit all stack branches")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	queue, err := submitQueue(state, *all, fs.Args())
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
