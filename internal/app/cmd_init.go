package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func (a *App) cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	trunk := fs.String("trunk", "", "trunk branch")
	mode := fs.String("mode", defaultRestackMode, "restack mode: rebase or merge")
	template := fs.String("template", "{slug}", "branch naming template")
	prefixIndex := fs.Bool("prefix-index", false, "prefix generated name with incrementing index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	if *mode != "rebase" && *mode != "merge" {
		return errors.New("--mode must be rebase or merge")
	}
	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}
	detectedTrunk := strings.TrimSpace(*trunk)
	if detectedTrunk == "" {
		detectedTrunk, err = detectTrunk()
		if err != nil {
			return err
		}
	}

	branches := map[string]*BranchRef{}
	if inferred, inferErr := inferState(repoRoot); inferErr == nil {
		branches = inferred.Branches
	}
	if existing, loadErr := loadState(repoRoot); loadErr == nil {
		branches = existing.Branches
	}

	state := &State{
		Version:     stateVersion,
		Trunk:       detectedTrunk,
		RestackMode: *mode,
		Naming: NamingConfig{
			Template:    *template,
			PrefixIndex: *prefixIndex,
			NextIndex:   1,
		},
		Branches: branches,
	}
	if err := saveState(repoRoot, state); err != nil {
		return err
	}
	fmt.Printf("initialized stack state (trunk=%s, mode=%s)\n", detectedTrunk, *mode)
	return nil
}
