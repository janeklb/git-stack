package app

import "fmt"

func validateReparentParent(state *State, target, newParent string) error {
	if newParent == target {
		return fmt.Errorf("invalid parent %q for %q: branch cannot parent itself", newParent, target)
	}
	if newParent == state.Trunk {
		return nil
	}
	if !isDescendantBranch(state, target, newParent) {
		return nil
	}
	return fmt.Errorf("invalid parent %q for %q: parent cannot be a descendant", newParent, target)
}

func isDescendantBranch(state *State, root, candidate string) bool {
	children := map[string][]string{}
	for name, meta := range state.Branches {
		if meta == nil {
			continue
		}
		children[meta.Parent] = append(children[meta.Parent], name)
	}

	seen := map[string]bool{}
	stack := []string{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range children[node] {
			if child == candidate {
				return true
			}
			if seen[child] {
				continue
			}
			seen[child] = true
			stack = append(stack, child)
		}
	}
	return false
}
