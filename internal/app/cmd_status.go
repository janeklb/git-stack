package app

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

func (a *App) cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	fmt.Printf("trunk: %s\n", state.Trunk)
	fmt.Printf("restack mode: %s\n", state.RestackMode)
	if len(state.Branches) == 0 {
		fmt.Println("(no stacked branches)")
		return nil
	}

	children := map[string][]string{}
	for branch, meta := range state.Branches {
		children[meta.Parent] = append(children[meta.Parent], branch)
	}
	for k := range children {
		sort.Strings(children[k])
	}

	var walk func(parent, indent string)
	walk = func(parent, indent string) {
		for _, branch := range children[parent] {
			meta := state.Branches[branch]
			line := indent + "- " + branch
			if meta.PR != nil {
				line += fmt.Sprintf(" (PR #%d %s)", meta.PR.Number, meta.PR.URL)
			}
			if drift, reason := detectDrift(branch, meta.Parent); drift {
				line += fmt.Sprintf(" [drift: %s]", reason)
			}
			fmt.Println(line)
			walk(branch, indent+"  ")
		}
	}

	fmt.Printf("- %s\n", state.Trunk)
	walk(state.Trunk, "  ")

	op, _ := loadOperation(repoRoot)
	if op != nil {
		fmt.Printf("restack in progress: mode=%s index=%d/%d\n", op.Mode, op.Index, len(op.Queue))
	}
	return nil
}
