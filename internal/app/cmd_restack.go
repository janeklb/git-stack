package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func (a *App) cmdRestack(args []string) error {
	fs := flag.NewFlagSet("restack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	mode := fs.String("mode", "", "restack mode override")
	cont := fs.Bool("continue", false, "continue restack after conflicts")
	abort := fs.Bool("abort", false, "abort in-progress restack")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cont && *abort {
		return errors.New("--continue and --abort are mutually exclusive")
	}
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	if *abort {
		return abortRestack(repoRoot)
	}
	if *cont {
		return continueRestack(repoRoot, state)
	}

	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	op, _ := loadOperation(repoRoot)
	if op != nil {
		return errors.New("a restack operation is already in progress; use stack restack --continue or --abort")
	}

	chosenMode := state.RestackMode
	if strings.TrimSpace(*mode) != "" {
		chosenMode = strings.TrimSpace(*mode)
	}
	if chosenMode != "rebase" && chosenMode != "merge" {
		return errors.New("restack mode must be rebase or merge")
	}

	queue := topoOrder(state)
	if len(queue) == 0 {
		fmt.Println("nothing to restack")
		return nil
	}
	original, err := currentBranch()
	if err != nil {
		return err
	}
	op = &RestackOperation{
		Type:           "restack",
		Mode:           chosenMode,
		OriginalBranch: original,
		Queue:          queue,
		Index:          0,
	}
	if err := saveOperation(repoRoot, op); err != nil {
		return err
	}
	return runRestack(repoRoot, state, op, false)
}
