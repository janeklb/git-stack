package app

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type statusTheme struct {
	useColor bool
}

func (a *App) cmdStatus(all bool, showDrift bool, noColor bool) error {
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	theme := statusTheme{useColor: !noColor && stdoutIsTTY()}

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

	var walk func(parent, prefix string)
	walk = func(parent, prefix string) {
		nodes := []string{}
		for _, branch := range children[parent] {
			if !selected[branch] {
				continue
			}
			if printed[branch] {
				continue
			}
			nodes = append(nodes, branch)
		}
		for i, branch := range nodes {
			printed[branch] = true
			last := i == len(nodes)-1
			meta := state.Branches[branch]
			connector := "├─ "
			nextPrefix := prefix + "│  "
			if last {
				connector = "└─ "
				nextPrefix = prefix + "   "
			}
			line := prefix + connector + theme.branch(branch)
			line += fmt.Sprintf(" [%s]", theme.state(branchPRState(meta.PR)))
			if meta.PR != nil {
				line += fmt.Sprintf(" (PR #%d %s)", meta.PR.Number, theme.link(meta.PR.URL))
			}
			if showDrift {
				if drift, reason := detectDrift(branch, meta.Parent); drift {
					line += fmt.Sprintf(" [%s]", theme.warning("drift: "+reason))
				}
			}
			fmt.Println(line)
			walk(branch, nextPrefix)
		}
	}

	fmt.Println(theme.trunk(state.Trunk))
	walk(state.Trunk, "")

	unrooted := []string{}
	for branch := range state.Branches {
		if selected[branch] && !printed[branch] {
			unrooted = append(unrooted, branch)
		}
	}
	sort.Strings(unrooted)
	for _, branch := range unrooted {
		meta := state.Branches[branch]
		line := theme.warning("? "+branch) + " [unrooted"
		if meta.Parent != "" {
			line += fmt.Sprintf(" parent=%s", meta.Parent)
		}
		line += fmt.Sprintf(" state=%s]", theme.state(branchPRState(meta.PR)))
		fmt.Println(line)
		walk(branch, "")
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

func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (t statusTheme) branch(name string) string {
	return t.wrap(name, "1;36")
}

func (t statusTheme) trunk(name string) string {
	return t.wrap(name+" (trunk)", "1;35")
}

func (t statusTheme) state(name string) string {
	color := "36"
	switch strings.ToLower(name) {
	case "local-only":
		color = "90"
	case "submitted":
		color = "33"
	case "updated":
		color = "32"
	}
	return t.wrap(name, color)
}

func (t statusTheme) link(url string) string {
	return t.wrap(url, "34")
}

func (t statusTheme) warning(text string) string {
	return t.wrap(text, "31")
}

func (t statusTheme) wrap(text, code string) string {
	if !t.useColor {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
