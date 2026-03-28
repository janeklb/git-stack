package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (a *App) cmdReparent(args []string) error {
	fs := flag.NewFlagSet("reparent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	parent := fs.String("parent", "", "new parent branch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: stack reparent <branch> --parent <new-parent>")
	}
	if strings.TrimSpace(*parent) == "" {
		return errors.New("--parent is required")
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}

	target := fs.Arg(0)
	newParent := strings.TrimSpace(*parent)
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	meta, ok := state.Branches[target]
	if !ok {
		return fmt.Errorf("branch not tracked in stack: %s", target)
	}
	if !branchExists(newParent) {
		return fmt.Errorf("new parent does not exist: %s", newParent)
	}
	oldParent := meta.Parent
	if oldParent == "" {
		oldParent = state.Trunk
	}
	if oldParent == newParent {
		fmt.Println("branch already has requested parent")
		return nil
	}

	if err := gitRun("switch", target); err != nil {
		return err
	}
	if err := gitRun("rebase", "--onto", newParent, oldParent); err != nil {
		return fmt.Errorf("rebase during reparent failed: %w", err)
	}

	meta.Parent = newParent
	if err := saveState(repoRoot, state); err != nil {
		return err
	}

	if meta.PR != nil {
		if err := ghRun("pr", "edit", strconv.Itoa(meta.PR.Number), "--base", newParent); err != nil {
			return fmt.Errorf("updated branch parent but failed to update PR base: %w", err)
		}
		meta.PR.Base = newParent
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}

	fmt.Printf("reparented %s: %s -> %s\n", target, oldParent, newParent)
	return nil
}
