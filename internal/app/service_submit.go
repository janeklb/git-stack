package app

import (
	"sort"
	"strings"
)

func ensurePR(branch, parent string, existing *PRMeta) (*PRMeta, error) {
	latestTitle, summary, err := branchSummary(parent, branch)
	if err != nil {
		return nil, err
	}
	defaultBody := composeBody(summary, "")

	if existing != nil && existing.Number > 0 {
		pr, err := ghView(existing.Number)
		if err == nil && strings.EqualFold(pr.State, "OPEN") {
			if err := ghEdit(existing.Number, parent, pr.Body); err != nil {
				return nil, err
			}
			return &PRMeta{Number: existing.Number, URL: pr.URL, Base: parent, Updated: true}, nil
		}
	}

	if open, err := ghFindByHead(branch); err == nil && open != nil {
		if err := ghEdit(open.Number, parent, open.Body); err != nil {
			return nil, err
		}
		return &PRMeta{Number: open.Number, URL: open.URL, Base: parent, Updated: true}, nil
	}

	number, url, err := ghCreate(branch, parent, latestTitle, defaultBody)
	if err != nil {
		return nil, err
	}
	return &PRMeta{Number: number, URL: url, Base: parent, Updated: false}, nil
}

func syncCurrentStackBodies(state *State, all bool, target string) error {
	selected, err := stackSelection(state, all, target)
	if err != nil {
		return err
	}
	ordered := orderedSelectedBranches(state, selected)

	lines := []StackPRLine{}
	type editablePR struct {
		branch string
		number int
		base   string
		body   string
	}
	editable := []editablePR{}

	for _, branch := range ordered {
		meta := state.Branches[branch]
		if meta == nil || meta.PR == nil || meta.PR.Number <= 0 {
			continue
		}
		pr, err := ghView(meta.PR.Number)
		if err != nil {
			continue
		}
		lines = append(lines, StackPRLine{
			Branch: branch,
			Number: pr.Number,
			Title:  pr.Title,
			URL:    pr.URL,
			State:  pr.State,
		})
		if strings.EqualFold(pr.State, "OPEN") {
			base := meta.Parent
			if base == "" {
				base = state.Trunk
			}
			editable = append(editable, editablePR{branch: branch, number: pr.Number, base: base, body: pr.Body})
		}
	}

	if len(lines) == 0 || len(editable) == 0 {
		return nil
	}

	for _, pr := range editable {
		managed := managedStackBlock(pr.branch, lines)
		body := upsertManagedBlock(pr.body, managed)
		if err := ghEdit(pr.number, pr.base, body); err != nil {
			return err
		}
	}
	return nil
}

func stackSelection(state *State, all bool, target string) (map[string]bool, error) {
	selected := map[string]bool{}
	if all {
		for branch := range state.Branches {
			selected[branch] = true
		}
		return selected, nil
	}
	if target == "" {
		current, err := currentBranch()
		if err != nil {
			return nil, err
		}
		target = current
	}
	return branchesInCurrentStack(state, target), nil
}

func orderedSelectedBranches(state *State, selected map[string]bool) []string {
	children := map[string][]string{}
	for branch, meta := range state.Branches {
		if !selected[branch] {
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		children[parent] = append(children[parent], branch)
	}
	for parent := range children {
		sort.Strings(children[parent])
	}

	ordered := []string{}
	seen := map[string]bool{}
	var visit func(parent string)
	visit = func(parent string) {
		for _, child := range children[parent] {
			if seen[child] {
				continue
			}
			seen[child] = true
			ordered = append(ordered, child)
			visit(child)
		}
	}

	visit(state.Trunk)
	for branch := range selected {
		if seen[branch] {
			continue
		}
		ordered = append(ordered, branch)
	}
	return ordered
}
