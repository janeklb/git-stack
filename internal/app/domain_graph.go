package app

import (
	"fmt"
	"sort"
)

func submitQueue(state *State, all bool, args []string) ([]string, error) {
	if all {
		return topoOrder(state)
	}
	target := ""
	if len(args) > 0 {
		target = args[0]
	} else {
		cur, err := currentBranch()
		if err != nil {
			return nil, err
		}
		target = cur
	}
	if _, ok := state.Branches[target]; !ok {
		return nil, fmt.Errorf("branch not tracked in stack: %s (use --all to submit everything)", target)
	}

	selected := branchesInCurrentStack(state, target)
	return topoOrderSelected(state, selected)
}

func topoOrder(state *State) ([]string, error) {
	return topoOrderSelected(state, nil)
}

type graphError struct {
	Branch string
	Reason string
}

func (e *graphError) Error() string {
	return fmt.Sprintf("invalid stack graph at %q: %s", e.Branch, e.Reason)
}

// An untracked parent is an external root, not a malformed tracked graph.
func topoOrderSelected(state *State, selected map[string]bool) ([]string, error) {
	included := map[string]bool{}
	for name := range state.Branches {
		if selected == nil || selected[name] {
			included[name] = true
		}
	}
	for name, include := range selected {
		if include {
			included[name] = true
		}
	}
	names := make([]string, 0, len(included))
	for name := range included {
		names = append(names, name)
	}
	sort.Strings(names)
	roots := []string{}
	for _, name := range names {
		meta := state.Branches[name]
		if meta == nil {
			return nil, &graphError{Branch: name, Reason: "missing metadata"}
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		if parent == name {
			return nil, &graphError{Branch: name, Reason: "branch cannot parent itself"}
		}
		if !included[parent] {
			roots = append(roots, name)
		}
	}
	children := branchChildrenIndex(state, included)
	visited := map[string]bool{}
	order := make([]string, 0, len(names))
	var walk func(string)
	walk = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		order = append(order, name)
		for _, child := range children[name] {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	for _, name := range names {
		if !visited[name] {
			return nil, &graphError{Branch: name, Reason: "parent cycle"}
		}
	}
	return order, nil
}

func branchChildrenIndex(state *State, selected map[string]bool) map[string][]string {
	children := map[string][]string{}
	for name, meta := range state.Branches {
		if selected != nil && !selected[name] {
			continue
		}
		if meta == nil {
			continue
		}
		parent := meta.Parent
		if parent == "" {
			parent = state.Trunk
		}
		children[parent] = append(children[parent], name)
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	return children
}

func traverseChildren(children map[string][]string, root string) ([]string, map[string]bool) {
	visited := map[string]bool{}
	order := []string{}
	var visit func(string)
	visit = func(node string) {
		for _, child := range children[node] {
			if visited[child] {
				continue
			}
			visited[child] = true
			order = append(order, child)
			visit(child)
		}
	}
	visit(root)
	return order, visited
}

func detectDrift(branch, parent string) (bool, string) {
	if !branchExists(parent) {
		return true, "parent-missing"
	}
	err := gitRunQuiet("merge-base", "--is-ancestor", parent, branch)
	if err != nil {
		return true, "parent-not-ancestor"
	}
	return false, ""
}

func inferParent(branch string, allBranches []string, trunk string) (string, error) {
	type candidate struct {
		name string
		ts   int64
	}
	branchHead, err := resolveBranchRef(branch)
	if err != nil {
		return "", err
	}

	candidates := []candidate{}
	for _, b := range allBranches {
		if b == branch {
			continue
		}
		candidateHead, err := resolveBranchRef(b)
		if err != nil {
			return "", err
		}
		if candidateHead == branchHead {
			continue
		}
		// Skip branches already merged into trunk - they shouldn't be inferred as stack parents
		if err := gitRunQuiet("merge-base", "--is-ancestor", b, trunk); err == nil {
			continue
		}
		if err := gitRunQuiet("merge-base", "--is-ancestor", b, branch); err == nil {
			ts, err := branchTimestamp(b)
			if err != nil {
				return "", err
			}
			candidates = append(candidates, candidate{name: b, ts: ts})
		}
	}
	if branchExists(trunk) {
		trunkHead, err := resolveBranchRef(trunk)
		if err != nil {
			return "", err
		}
		if trunkHead != branchHead {
			if err := gitRunQuiet("merge-base", "--is-ancestor", trunk, branch); err == nil {
				ts, err := branchTimestamp(trunk)
				if err != nil {
					return "", err
				}
				candidates = append(candidates, candidate{name: trunk, ts: ts})
			}
		}
	}
	if len(candidates) == 0 {
		return trunk, nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ts > candidates[j].ts })
	return candidates[0].name, nil
}

func branchesInCurrentStack(state *State, current string) map[string]bool {
	selected := map[string]bool{}
	if current == "" || current == state.Trunk {
		for branch := range state.Branches {
			selected[branch] = true
		}
		return selected
	}
	if _, ok := state.Branches[current]; !ok {
		return selected
	}

	children := branchChildrenIndex(state, nil)
	root := currentStackRoot(state, current)
	return collectDescendants(children, root)
}

func currentStackRoot(state *State, current string) string {
	root := current
	seen := map[string]bool{}
	for {
		if seen[root] {
			break
		}
		seen[root] = true
		meta := state.Branches[root]
		if meta == nil {
			break
		}
		parent := meta.Parent
		if parent == "" || parent == state.Trunk {
			break
		}
		if _, ok := state.Branches[parent]; !ok {
			break
		}
		root = parent
	}
	return root
}

func collectDescendants(children map[string][]string, root string) map[string]bool {
	selected := map[string]bool{}
	stack := []string{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if selected[node] {
			continue
		}
		selected[node] = true
		for _, child := range children[node] {
			stack = append(stack, child)
		}
	}
	return selected
}
