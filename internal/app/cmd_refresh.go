package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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

type refreshPlanDeps struct {
	git                     refreshGitBoundary
	gh                      refreshGHBoundary
	mergedCleanupIntegrated func(string, string, *GhPR) (bool, error)
	mergedBranchChildren    func(*State, string) []string
}

func defaultRefreshPlanDeps() refreshPlanDeps {
	return refreshPlanDeps{
		git:                     defaultGitBoundary{},
		gh:                      defaultGHBoundary{},
		mergedCleanupIntegrated: mergedCleanupIntegrated,
		mergedBranchChildren:    mergedBranchChildren,
	}
}

func (a *App) cmdRefresh(restack bool, publish string) error {
	publish = strings.TrimSpace(strings.ToLower(publish))
	if publish != "" && publish != "current" && publish != "all" {
		return errors.New("--publish must be one of: current, all")
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
	if len(plan.Cleanup) == 0 && !restack && publish == "" {
		a.println("refresh: nothing to do")
		return nil
	}

	current, _ := currentBranch()
	printRefreshPlan(a.stdout, plan, restack, publish, current)
	if !confirmRefreshApply(a.in, a.stdout) {
		a.println("refresh cancelled")
		return nil
	}

	deps := defaultRefreshPlanDeps()
	for _, candidate := range plan.Cleanup {
		cleanupMergedBranchForRefresh(a.stdout, state, candidate, deps.git)
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

	if publish == "all" {
		if err := syncCurrentStackBodies(state, true, ""); err != nil {
			return err
		}
	} else if publish == "current" {
		if _, ok := state.Branches[current]; !ok {
			current = ""
		}
		if err := syncCurrentStackBodies(state, false, current); err != nil {
			return err
		}
	}

	a.println("refresh completed")
	return nil
}

func buildRefreshPlan(state *State) (*refreshPlan, error) {
	return buildRefreshPlanWithDeps(state, defaultRefreshPlanDeps())
}

func buildRefreshPlanWithDeps(state *State, deps refreshPlanDeps) (*refreshPlan, error) {
	plan := &refreshPlan{Cleanup: []refreshCleanupCandidate{}}
	branches := topoOrder(state)
	for _, branch := range branches {
		meta := state.Branches[branch]
		if meta == nil || meta.PR == nil || meta.PR.Number <= 0 {
			continue
		}
		pr, err := deps.gh.View(meta.PR.Number)
		if err != nil || !strings.EqualFold(pr.State, "MERGED") {
			continue
		}

		remoteExists, remoteErr := deps.git.RemoteBranchExists(branch)
		if remoteErr != nil || remoteExists {
			continue
		}

		base := state.Trunk
		if strings.TrimSpace(meta.PR.Base) != "" {
			base = meta.PR.Base
		} else if strings.TrimSpace(meta.Parent) != "" {
			base = meta.Parent
		}

		hasLocal := deps.git.LocalBranchExists(branch)
		if hasLocal {
			integrated, err := deps.mergedCleanupIntegrated(branch, base, pr)
			if err != nil || !integrated {
				continue
			}
		}

		plan.Cleanup = append(plan.Cleanup, refreshCleanupCandidate{
			Branch:   branch,
			Base:     base,
			HasLocal: hasLocal,
			Children: deps.mergedBranchChildren(state, branch),
		})
	}

	sort.Slice(plan.Cleanup, func(i, j int) bool {
		return plan.Cleanup[i].Branch < plan.Cleanup[j].Branch
	})
	return plan, nil
}

func printRefreshPlan(out io.Writer, plan *refreshPlan, restack bool, publish, current string) {
	_, _ = fmt.Fprintln(out, "refresh plan:")
	if len(plan.Cleanup) == 0 {
		_, _ = fmt.Fprintln(out, "- cleanup: none")
	} else {
		for _, candidate := range plan.Cleanup {
			kind := "state-only"
			if candidate.HasLocal {
				kind = "delete-local+state"
			}
			_, _ = fmt.Fprintf(out, "- cleanup: %s (%s)\n", candidate.Branch, kind)
			if len(candidate.Children) > 0 {
				_, _ = fmt.Fprintf(out, "  reparent children -> %s: %s\n", candidate.Base, strings.Join(candidate.Children, ", "))
			}
		}
	}
	if restack {
		_, _ = fmt.Fprintln(out, "- restack: enabled")
	} else {
		_, _ = fmt.Fprintln(out, "- restack: disabled")
	}
	if publish == "all" {
		_, _ = fmt.Fprintln(out, "- publish: all tracked branches")
	} else if publish == "current" {
		if strings.TrimSpace(current) == "" {
			_, _ = fmt.Fprintln(out, "- publish: current stack (auto) ")
		} else {
			_, _ = fmt.Fprintf(out, "- publish: current stack from %s\n", current)
		}
	} else {
		_, _ = fmt.Fprintln(out, "- publish: disabled")
	}
}

func confirmRefreshApply(in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)
	_, _ = fmt.Fprint(out, "apply refresh plan? [y/N]: ")
	answer, err := readPromptLine(reader)
	if err != nil {
		return false
	}
	return answer == "y" || answer == "yes"
}

func cleanupMergedBranchForRefresh(out io.Writer, state *State, candidate refreshCleanupCandidate, git refreshGitBoundary) {
	current, err := git.CurrentBranch()
	if err == nil && current == candidate.Branch {
		target := state.Trunk
		if strings.TrimSpace(target) == "" {
			target = "main"
		}
		if switchErr := git.Run("switch", target); switchErr != nil {
			_, _ = fmt.Fprintf(out, "%s -> failed to switch to %s before cleanup: %v\n", candidate.Branch, target, switchErr)
			return
		}
	}

	if candidate.HasLocal {
		if err := git.DeleteLocalBranch(candidate.Branch); err != nil {
			_, _ = fmt.Fprintf(out, "%s -> failed to delete local merged branch: %v\n", candidate.Branch, err)
			return
		}
	}

	archiveMergedBranch(state, candidate.Branch)
	reparentChildrenAfterCleanup(out, state, candidate.Branch, candidate.Base)
	delete(state.Branches, candidate.Branch)
	pruneArchivedLineage(state)
	_, _ = fmt.Fprintf(out, "%s -> cleaned merged branch from local stack state\n", candidate.Branch)
}

func reparentChildrenAfterCleanup(out io.Writer, state *State, removedBranch, replacementParent string) {
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
		_, _ = fmt.Fprintf(out, "%s -> reparented to %s after merged parent cleanup\n", name, replacementParent)
	}
}

func mergedCleanupIntegrated(branch, base string, pr *GhPR) (bool, error) {
	integrated, err := branchFullyIntegrated(branch, base)
	if err != nil {
		return false, err
	}
	if integrated {
		return true, nil
	}
	if pr == nil {
		return false, nil
	}

	mergeCommit := ""
	if pr.MergeCommit != nil {
		mergeCommit = strings.TrimSpace(pr.MergeCommit.OID)
	}
	headCommit := strings.TrimSpace(pr.HeadRefOID)
	if headCommit == "" {
		return false, nil
	}

	localAtOrBehindHead, err := branchAtOrBehindCommit(branch, headCommit)
	if err != nil {
		return false, err
	}
	if !localAtOrBehindHead {
		return false, nil
	}
	if mergeCommit == "" {
		return false, nil
	}

	contains, err := baseContainsCommit(base, mergeCommit)
	if err != nil {
		return false, err
	}
	return contains, nil
}
