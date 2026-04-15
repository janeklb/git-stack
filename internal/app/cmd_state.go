package app

import (
	"fmt"
	"sort"
	"strings"
)

type stateTheme struct {
	useColor bool
	current  string
}

type stateStatusKind int

const (
	stateStatusNormal stateStatusKind = iota
	stateStatusAlert
	stateStatusMerged
)

type stateStatusItem struct {
	text string
	kind stateStatusKind
}

func (a *App) cmdState(all bool, showDrift bool, noColor bool) error {
	repoRoot, state, _, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	current, err := currentBranch()
	if err != nil {
		return err
	}
	theme := stateTheme{useColor: !noColor && stdoutIsTTY(a.stdout), current: current}
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

	for _, archived := range stateArchivedLineageLines(state, selected, theme) {
		a.println(archived)
	}
	trunkPrefix := stateArchivedTreePrefix(state, selected)

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
			line += stateStatusBox(theme, stateBranchStatusItems(branch, meta, localBranchSet, showDrift))
			if meta.PR != nil {
				line += fmt.Sprintf(" (PR #%d %s)", meta.PR.Number, theme.link(meta.PR.URL))
			}
			a.println(line)
			walk(branch, nextPrefix)
		}
	}

	trunkLine := trunkPrefix + theme.trunk(state.Trunk)
	if !localBranchSet[state.Trunk] {
		trunkLine += stateStatusBox(theme, []stateStatusItem{{text: "invalid", kind: stateStatusAlert}})
	}
	a.println(trunkLine)
	walk(state.Trunk, strings.Repeat("   ", len(stateArchivedLineageChainForSelection(state, selected))))

	unrooted := []string{}
	for branch := range state.Branches {
		if selected[branch] && !printed[branch] {
			unrooted = append(unrooted, branch)
		}
	}
	sort.Strings(unrooted)
	for _, branch := range unrooted {
		meta := state.Branches[branch]
		items := []stateStatusItem{{text: "unrooted", kind: stateStatusAlert}}
		items = append(items, stateBranchStatusItems(branch, meta, localBranchSet, showDrift)...)
		line := theme.warning("? ") + theme.branch(branch) + stateStatusBox(theme, items)
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

func stateBranchStatusItems(branch string, meta *BranchRef, localBranches map[string]bool, showDrift bool) []stateStatusItem {
	pr := (*PRMeta)(nil)
	if meta != nil {
		pr = meta.PR
	}
	items := []stateStatusItem{{text: branchStateLabel(pr), kind: stateStatusNormal}}
	if stateBranchMissingLocally(branch, meta, localBranches) {
		items = append(items, stateStatusItem{text: "invalid", kind: stateStatusAlert})
	}
	if meta != nil && meta.Parent != "" && !localBranches[meta.Parent] {
		items = append(items, stateStatusItem{text: "missing-parent=" + meta.Parent, kind: stateStatusAlert})
	}
	if showDrift && meta != nil && localBranches[branch] {
		if drift, reason := detectDrift(branch, meta.Parent); drift {
			switch reason {
			case "parent-not-ancestor":
				items = append(items, stateStatusItem{text: "drifted-from-ancestor", kind: stateStatusAlert})
			case "parent-missing":
				// Already represented as missing-parent=<name> above.
			}
		}
	}
	return items
}

func sortedStateBranchNames(branches map[string]*BranchRef) []string {
	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stateBranchInvalid(branch string, meta *BranchRef, localBranches map[string]bool) bool {
	if meta == nil {
		return true
	}
	if !localBranches[meta.Parent] {
		return true
	}
	return stateBranchMissingLocally(branch, meta, localBranches)
}

func stateBranchMissingLocally(branch string, meta *BranchRef, localBranches map[string]bool) bool {
	if meta == nil {
		return true
	}
	if !localBranches[branch] {
		return true
	}
	return false
}

func stateArchivedLineageLines(state *State, selected map[string]bool, theme stateTheme) []string {
	chain := stateArchivedLineageChainForSelection(state, selected)
	lines := []string{}
	for i, branch := range chain {
		prefix := strings.Repeat("   ", i)
		if i > 0 {
			prefix = strings.Repeat("   ", i-1) + "└─ "
		}
		lines = append(lines, prefix+theme.branch(branch)+stateStatusBox(theme, []stateStatusItem{{text: "merged", kind: stateStatusMerged}}))
	}
	return lines
}

func stateArchivedTreePrefix(state *State, selected map[string]bool) string {
	chain := stateArchivedLineageChainForSelection(state, selected)
	if len(chain) == 0 {
		return ""
	}
	return strings.Repeat("   ", len(chain)-1) + "└─ "
}

func stateArchivedLineageChainForSelection(state *State, selected map[string]bool) []string {
	best := []string{}
	for _, branch := range sortedStateBranchNames(state.Branches) {
		if !selected[branch] {
			continue
		}
		chain := stateArchivedLineageChain(state, branch)
		if len(chain) > len(best) {
			best = chain
		}
	}
	return best
}

func stateArchivedLineageChain(state *State, branch string) []string {
	chain := []string{}
	seen := map[string]bool{}
	cur := lineageParent(state, branch)
	for cur != "" && cur != state.Trunk {
		if seen[cur] {
			break
		}
		seen[cur] = true
		if state.Archived[cur] == nil {
			break
		}
		chain = append(chain, cur)
		cur = lineageParent(state, cur)
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func (t stateTheme) branch(name string) string {
	if name == t.current {
		return t.wrap(name, "1;36")
	}
	return name
}

func (t stateTheme) trunk(name string) string {
	if name == t.current {
		return t.wrap(name+" (trunk)", "1;36")
	}
	return t.wrap(name+" (trunk)", "1")
}

func (t stateTheme) state(name string) string {
	color := "36"
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(name, ",", 2)[0]))
	switch base {
	case "local-only":
		color = "90"
	case "submitted":
		color = "33"
	case "updated":
		color = "32"
	case "invalid":
		color = "90"
	}
	return t.wrap(name, color)
}

func stateStatusBox(t stateTheme, items []stateStatusItem) string {
	if len(items) == 0 {
		return ""
	}
	return " [" + renderStateStatusItems(t, items) + "]"
}

func renderStateStatusItems(t stateTheme, items []stateStatusItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch item.kind {
		case stateStatusAlert:
			parts = append(parts, t.warning(item.text))
		case stateStatusMerged:
			parts = append(parts, t.archived(item.text))
		default:
			parts = append(parts, t.state(item.text))
		}
	}
	return strings.Join(parts, ", ")
}

func (t stateTheme) link(url string) string {
	return t.wrap(url, "34")
}

func (t stateTheme) warning(text string) string {
	return t.wrap(text, "31")
}

func (t stateTheme) archived(text string) string {
	return t.wrap(text, "90")
}

func (t stateTheme) wrap(text, code string) string {
	if !t.useColor {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
