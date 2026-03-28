package app

import (
	"fmt"
	"sort"
)

func (a *App) cmdStatus(all bool, showDrift bool) error {
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	current, err := currentBranch()
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	if all || current == state.Trunk {
		for branch := range state.Branches {
			selected[branch] = true
		}
	} else {
		selected = branchesInCurrentStack(state, current)
	}

	children := map[string][]string{}
	for branch, meta := range state.Branches {
		children[meta.Parent] = append(children[meta.Parent], branch)
	}
	for k := range children {
		sort.Strings(children[k])
	}
	printed := map[string]bool{}

	var walk func(parent, indent string)
	walk = func(parent, indent string) {
		for _, branch := range children[parent] {
			if !selected[branch] {
				continue
			}
			if printed[branch] {
				continue
			}
			printed[branch] = true
			meta := state.Branches[branch]
			line := indent + "- " + branch
			line += fmt.Sprintf(" [%s]", branchPRState(meta.PR))
			if meta.PR != nil {
				line += fmt.Sprintf(" (PR #%d %s)", meta.PR.Number, meta.PR.URL)
			}
			if showDrift {
				if drift, reason := detectDrift(branch, meta.Parent); drift {
					line += fmt.Sprintf(" [drift: %s]", reason)
				}
			}
			fmt.Println(line)
			walk(branch, indent+"  ")
		}
	}

	fmt.Printf("- %s\n", state.Trunk)
	walk(state.Trunk, "  ")

	unrooted := []string{}
	for branch := range state.Branches {
		if selected[branch] && !printed[branch] {
			unrooted = append(unrooted, branch)
		}
	}
	sort.Strings(unrooted)
	for _, branch := range unrooted {
		meta := state.Branches[branch]
		line := "- " + branch + " [unrooted"
		if meta.Parent != "" {
			line += fmt.Sprintf(" parent=%s", meta.Parent)
		}
		line += fmt.Sprintf(" state=%s]", branchPRState(meta.PR))
		fmt.Println(line)
		walk(branch, "  ")
	}

	op, _ := loadOperation(repoRoot)
	if op != nil {
		fmt.Printf("restack in progress: mode=%s index=%d/%d\n", op.Mode, op.Index, len(op.Queue))
	}
	return nil
}

func branchPRState(pr *PRMeta) string {
	if pr == nil || pr.Number <= 0 {
		return "local-only"
	}
	if pr.Updated {
		return "updated"
	}
	return "submitted"
}
