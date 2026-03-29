package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

type refreshCleanupCandidate struct {
	Branch   string
	Base     string
	HasLocal bool
	Children []string
}

type refreshPlan struct {
	Cleanup []refreshCleanupCandidate
}

func (a *App) cmdRefresh(restack, publish, publishAll bool) error {
	if publish && publishAll {
		return errors.New("--publish and --publish-all are mutually exclusive")
	}
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("refresh fetch failed: %w", err)
	}
	if err := ensureCleanWorktree(); err != nil {
		return err
	}

	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}

	plan, err := buildRefreshPlan(state)
	if err != nil {
		return err
	}

	current, _ := currentBranch()
	printRefreshPlan(plan, restack, publish, publishAll, current)
	if !confirmRefreshApply() {
		fmt.Println("refresh cancelled")
		return nil
	}

	for _, candidate := range plan.Cleanup {
		cleanupMergedBranchForRefresh(state, candidate)
	}

	if persisted {
		if err := saveState(repoRoot, state); err != nil {
			return err
		}
	}

	if restack {
		if err := a.cmdRestack("", false, false); err != nil {
			return err
		}
	}

	if publishAll {
		if err := syncCurrentStackBodies(state, true, ""); err != nil {
			return err
		}
	} else if publish {
		if _, ok := state.Branches[current]; !ok {
			current = ""
		}
		if err := syncCurrentStackBodies(state, false, current); err != nil {
			return err
		}
	}

	fmt.Println("refresh completed")
	return nil
}

func buildRefreshPlan(state *State) (*refreshPlan, error) {
	plan := &refreshPlan{Cleanup: []refreshCleanupCandidate{}}
	branches := topoOrder(state)
	for _, branch := range branches {
		meta := state.Branches[branch]
		if meta == nil || meta.PR == nil || meta.PR.Number <= 0 {
			continue
		}
		pr, err := ghView(meta.PR.Number)
		if err != nil || !strings.EqualFold(pr.State, "MERGED") {
			continue
		}

		remoteExists, remoteErr := remoteBranchExists(branch)
		if remoteErr != nil || remoteExists {
			continue
		}

		base := state.Trunk
		if strings.TrimSpace(meta.PR.Base) != "" {
			base = meta.PR.Base
		} else if strings.TrimSpace(meta.Parent) != "" {
			base = meta.Parent
		}

		hasLocal := localBranchExists(branch)
		if hasLocal {
			integrated, err := branchFullyIntegrated(branch, base)
			if err != nil || !integrated {
				continue
			}
		}

		plan.Cleanup = append(plan.Cleanup, refreshCleanupCandidate{
			Branch:   branch,
			Base:     base,
			HasLocal: hasLocal,
			Children: mergedBranchChildren(state, branch),
		})
	}

	sort.Slice(plan.Cleanup, func(i, j int) bool {
		return plan.Cleanup[i].Branch < plan.Cleanup[j].Branch
	})
	return plan, nil
}

func printRefreshPlan(plan *refreshPlan, restack, publish, publishAll bool, current string) {
	fmt.Println("refresh plan:")
	if len(plan.Cleanup) == 0 {
		fmt.Println("- cleanup: none")
	} else {
		for _, candidate := range plan.Cleanup {
			kind := "state-only"
			if candidate.HasLocal {
				kind = "delete-local+state"
			}
			fmt.Printf("- cleanup: %s (%s)\n", candidate.Branch, kind)
			if len(candidate.Children) > 0 {
				fmt.Printf("  reparent children -> %s: %s\n", candidate.Base, strings.Join(candidate.Children, ", "))
			}
		}
	}
	if restack {
		fmt.Println("- restack: enabled")
	} else {
		fmt.Println("- restack: disabled")
	}
	if publishAll {
		fmt.Println("- publish: all tracked branches")
	} else if publish {
		if strings.TrimSpace(current) == "" {
			fmt.Println("- publish: current stack (auto) ")
		} else {
			fmt.Printf("- publish: current stack from %s\n", current)
		}
	} else {
		fmt.Println("- publish: disabled")
	}
}

func confirmRefreshApply() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("apply refresh plan? [y/N]: ")
	answer, err := readPromptLine(reader)
	if err != nil {
		return false
	}
	return answer == "y" || answer == "yes"
}

func cleanupMergedBranchForRefresh(state *State, candidate refreshCleanupCandidate) {
	current, err := currentBranch()
	if err == nil && current == candidate.Branch {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		if switchErr := gitRun("switch", target); switchErr != nil {
			fmt.Printf("%s -> failed to switch to %s before cleanup: %v\n", candidate.Branch, target, switchErr)
			return
		}
	}

	if candidate.HasLocal {
		if err := deleteLocalBranch(candidate.Branch); err != nil {
			fmt.Printf("%s -> failed to delete local merged branch: %v\n", candidate.Branch, err)
			return
		}
	}

	archiveMergedBranch(state, candidate.Branch)
	reparentChildrenAfterCleanup(state, candidate.Branch, candidate.Base)
	delete(state.Branches, candidate.Branch)
	pruneArchivedLineage(state)
	fmt.Printf("%s -> cleaned merged branch from local stack state\n", candidate.Branch)
}

func reparentChildrenAfterCleanup(state *State, removedBranch, replacementParent string) {
	if strings.TrimSpace(replacementParent) == "" {
		replacementParent = state.Trunk
	}
	for name, meta := range state.Branches {
		if name == removedBranch || meta == nil {
			continue
		}
		if meta.Parent != removedBranch {
			continue
		}
		meta.Parent = replacementParent
		if meta.PR != nil {
			meta.PR.Base = replacementParent
		}
		fmt.Printf("%s -> reparented to %s after merged parent cleanup\n", name, replacementParent)
	}
}
