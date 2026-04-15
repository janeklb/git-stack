package app

import (
	"fmt"
	"sort"
	"strings"
)

type stateTheme struct {
	useColor bool
}

func (a *App) cmdState(all bool, showDrift bool, noColor bool) error {
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	theme := stateTheme{useColor: !noColor && stdoutIsTTY(a.stdout)}

	current, err := currentBranch()
	if err != nil {
		return err
	}
	localBranches, err := listLocalBranches()
	if err != nil {
		return err
	}
	localBranchSet := map[string]bool{}
	for _, branch := range localBranches {
		localBranchSet[branch] = true
	}
	selected := map[string]bool{}
	if all || current == state.Trunk {
		for branch := range state.Branches {
			selected[branch] = true
		}
	} else {
		selected = branchesInCurrentStack(state, current)
	}

	for _, warning := range stateValidationWarnings(state, selected, localBranchSet) {
		a.println(theme.warning("WARN " + warning))
	}
	for _, archived := range stateArchivedLineageLines(state, selected, theme) {
		a.println(archived)
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
			line += fmt.Sprintf(" [%s]", theme.state(branchStateLabel(meta.PR)))
			if !localBranchSet[branch] {
				line += fmt.Sprintf(" [%s]", theme.warning("invalid: missing-local"))
			}
			if meta.PR != nil {
				line += fmt.Sprintf(" (PR #%d %s)", meta.PR.Number, theme.link(meta.PR.URL))
			}
			if showDrift && localBranchSet[branch] {
				if drift, reason := detectDrift(branch, meta.Parent); drift {
					line += fmt.Sprintf(" [%s]", theme.warning("drift: "+reason))
				}
			}
			a.println(line)
			walk(branch, nextPrefix)
		}
	}

	a.println(theme.trunk(state.Trunk))
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
		line += fmt.Sprintf(" state=%s]", theme.state(branchStateLabel(meta.PR)))
		a.println(line)
		walk(branch, "")
	}

	op, _ := loadOperation(repoRoot)
	if op != nil {
		a.printlnf("restack in progress: mode=%s index=%d/%d", op.Mode, op.Index, len(op.Queue))
	}
	return nil
}

func branchStateLabel(pr *PRMeta) string {
	if pr == nil || pr.Number <= 0 {
		return "local-only"
	}
	if pr.Updated {
		return "updated"
	}
	return "submitted"
}

func stateValidationWarnings(state *State, selected map[string]bool, localBranches map[string]bool) []string {
	warnings := []string{}
	if !localBranches[state.Trunk] {
		warnings = append(warnings, fmt.Sprintf("trunk missing locally: %s", state.Trunk))
	}
	for _, branch := range sortedStateBranchNames(state.Branches) {
		if !selected[branch] {
			continue
		}
		meta := state.Branches[branch]
		if meta == nil {
			warnings = append(warnings, fmt.Sprintf("tracked branch metadata missing: %s", branch))
			continue
		}
		if !localBranches[branch] {
			warnings = append(warnings, fmt.Sprintf("tracked branch missing locally: %s", branch))
		}
		if meta.Parent != "" && !localBranches[meta.Parent] {
			warnings = append(warnings, fmt.Sprintf("tracked branch parent missing locally: %s -> %s", branch, meta.Parent))
		}
	}
	return warnings
}

func sortedStateBranchNames(branches map[string]*BranchRef) []string {
	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stateArchivedLineageLines(state *State, selected map[string]bool, theme stateTheme) []string {
	ordered := orderedSelectedLineageBranches(state, selected)
	lines := []string{}
	for _, branch := range ordered {
		archived := state.Archived[branch]
		if archived == nil {
			continue
		}
		parent := archived.Parent
		if parent == "" {
			parent = state.Trunk
		}
		parentLabel := theme.branch(parent)
		if parent == state.Trunk {
			parentLabel = theme.trunk(parent)
		} else if state.Archived[parent] != nil {
			parentLabel = theme.warning(parent + " (archived)")
		}
		lines = append(lines, theme.warning(branch+" (archived)")+" -> "+parentLabel)
	}
	return lines
}

func (t stateTheme) branch(name string) string {
	return t.wrap(name, "1;36")
}

func (t stateTheme) trunk(name string) string {
	return t.wrap(name+" (trunk)", "1;35")
}

func (t stateTheme) state(name string) string {
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

func (t stateTheme) link(url string) string {
	return t.wrap(url, "34")
}

func (t stateTheme) warning(text string) string {
	return t.wrap(text, "31")
}

func (t stateTheme) wrap(text, code string) string {
	if !t.useColor {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
