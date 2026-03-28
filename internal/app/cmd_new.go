package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func (a *App) cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	parent := fs.String("parent", "", "parent branch")
	template := fs.String("template", "", "override naming template")
	prefixIndex := fs.Bool("prefix-index", false, "prefix generated name with incrementing index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, err := loadStateFromRepo()
	if err != nil {
		return err
	}

	name := ""
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	} else {
		name = fmt.Sprintf("change-%d", time.Now().Unix())
	}

	parentBranch := strings.TrimSpace(*parent)
	if parentBranch == "" {
		parentBranch, err = defaultParent(state)
		if err != nil {
			return err
		}
	}

	if !branchExists(parentBranch) {
		return fmt.Errorf("parent branch does not exist: %s", parentBranch)
	}

	slug := slugify(name)
	if slug == "" {
		return errors.New("branch name cannot be empty")
	}

	chosenTemplate := state.Naming.Template
	if *template != "" {
		chosenTemplate = *template
	}
	usePrefixIndex := state.Naming.PrefixIndex || *prefixIndex
	branchName := applyTemplate(chosenTemplate, slug, state.Naming.NextIndex, usePrefixIndex)

	if branchExists(branchName) {
		return fmt.Errorf("branch already exists: %s", branchName)
	}

	if err := gitRun("switch", parentBranch); err != nil {
		return err
	}
	if err := gitRun("switch", "-c", branchName); err != nil {
		return err
	}

	state.Branches[branchName] = &BranchRef{Parent: parentBranch}
	state.Naming.NextIndex++
	if err := saveState(repoRoot, state); err != nil {
		return err
	}
	fmt.Printf("created %s (parent=%s)\n", branchName, parentBranch)
	return nil
}
