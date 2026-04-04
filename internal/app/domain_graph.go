package app

import (
	"fmt"
	"sort"
	"strings"
)

func submitQueue(state *State, all bool, args []string) ([]string, error) {
	if all {
		return topoOrder(state), nil
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

	order := []string{}
	seen := map[string]bool{}
	cur := target
	for {
		meta, ok := state.Branches[cur]
		if !ok {
			break
		}
		if seen[cur] {
			break
		}
		seen[cur] = true
		order = append(order, cur)
		if meta.Parent == "" || meta.Parent == state.Trunk {
			break
		}
		cur = meta.Parent
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order, nil
}

func topoOrder(state *State) []string {
	visited := map[string]bool{}
	order := []string{}
	children := map[string][]string{}
	for name, meta := range state.Branches {
		p := meta.Parent
		if p == "" {
			p = state.Trunk
		}
		children[p] = append(children[p], name)
	}
	for key := range children {
		sort.Strings(children[key])
	}
	var visit func(node string)
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
	visit(state.Trunk)

	for name := range state.Branches {
		if visited[name] {
			continue
		}
		order = append(order, name)
	}
	return order
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
	branchHead, err := gitOutput("rev-parse", branch)
	if err != nil {
		return "", err
	}
	branchHead = strings.TrimSpace(branchHead)

	candidates := []candidate{}
	for _, b := range allBranches {
		if b == branch {
			continue
		}
		candidateHead, err := gitOutput("rev-parse", b)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(candidateHead) == branchHead {
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
		trunkHead, err := gitOutput("rev-parse", trunk)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(trunkHead) != branchHead {
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

	children := map[string][]string{}
	for branch, meta := range state.Branches {
		children[meta.Parent] = append(children[meta.Parent], branch)
	}

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
