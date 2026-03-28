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
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
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
			state.Branches[cur] = &BranchRef{Parent: inferredParent}
		}
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
	if !persisted {
		fmt.Printf("initialized stack state (trunk=%s, mode=%s)\n", state.Trunk, state.RestackMode)
	}
	if err := saveState(repoRoot, state); err != nil {
		return err
	}
	fmt.Printf("created %s (parent=%s)\n", branchName, parentBranch)
	return nil
}
