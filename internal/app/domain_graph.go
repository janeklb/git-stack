package app

import (
	"fmt"
	"sort"
)

func defaultParent(state *State) (string, error) {
	cur, err := currentBranch()
	if err != nil {
		return "", err
	}
	if _, ok := state.Branches[cur]; ok {
		return cur, nil
	}
	return state.Trunk, nil
}

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
	err := gitRun("merge-base", "--is-ancestor", parent, branch)
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
	candidates := []candidate{}
	for _, b := range allBranches {
		if b == branch {
			continue
		}
		if err := gitRun("merge-base", "--is-ancestor", b, branch); err == nil {
			ts, err := branchTimestamp(b)
			if err != nil {
				return "", err
			}
			candidates = append(candidates, candidate{name: b, ts: ts})
		}
	}
	if branchExists(trunk) {
		if err := gitRun("merge-base", "--is-ancestor", trunk, branch); err == nil {
			ts, err := branchTimestamp(trunk)
			if err != nil {
				return "", err
			}
			candidates = append(candidates, candidate{name: trunk, ts: ts})
		}
	}
	if len(candidates) == 0 {
		return trunk, nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ts > candidates[j].ts })
	return candidates[0].name, nil
}
