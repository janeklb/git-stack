package app

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

type pruneLocalCandidate struct {
	Branch string
	PR     *GhPR
	Base   string
}

type pruneLocalSkip struct {
	Branch string
	Reason string
}

type pruneLocalPlan struct {
	Delete []pruneLocalCandidate
	Skip   []pruneLocalSkip
}

type pruneLocalPlanDeps struct {
	git pruneGitClient
	gh  pruneGHClient
}

type pruneLocalScope struct {
	trackedBranches  map[string]bool
	includeUntracked bool
}

func defaultPruneLocalPlanDeps() pruneLocalPlanDeps {
	return pruneLocalPlanDeps{
		git: defaultGitClient{},
		gh:  defaultGHClient{},
	}
}

func allTrackedBranches(state *State) map[string]bool {
	tracked := map[string]bool{}
	for branch := range state.Branches {
		tracked[branch] = true
	}
	return tracked
}

func (a *App) cmdCleanup(yes bool, untracked bool) error {
	return a.runCleanupCommand("cleanup", yes, pruneLocalScope{includeUntracked: untracked})
}

func (a *App) cmdPruneLocal(yes bool) error {
	return a.runCleanupCommand("prune-local", yes, pruneLocalScope{includeUntracked: true})
}

func (a *App) runCleanupCommand(commandName string, yes bool, scope pruneLocalScope) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	repoRoot, state, err := loadStateFromRepo()
	if err != nil {
		return err
	}
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("%s fetch failed: %w", commandName, err)
	}
	if scope.trackedBranches == nil {
		scope.trackedBranches = allTrackedBranches(state)
	}

	plan, err := buildPruneLocalPlan(state, scope)
	if err != nil {
		return err
	}
	if len(plan.Delete) == 0 {
		a.printf("%s: nothing to do\n", commandName)
		return nil
	}

	printCleanupPlan(a.stdout, commandName, plan)
	if !yes && !confirmCleanupApply(a.in, a.stdout, commandName) {
		a.printf("%s cancelled\n", commandName)
		return nil
	}

	current, _ := currentBranch()
	for _, candidate := range plan.Delete {
		if current == candidate.Branch {
			target := state.Trunk
			if strings.TrimSpace(target) == "" {
				target = "main"
			}
			if err := gitRunQuiet("switch", target); err != nil {
				a.printf("%s -> failed to switch to %s before deletion: %v\n", candidate.Branch, target, err)
				continue
			}
			current = target
		}
		if err := deleteLocalBranch(candidate.Branch); err != nil {
			a.printf("%s -> failed to delete local branch: %v\n", candidate.Branch, err)
			continue
		}

		if _, tracked := state.Branches[candidate.Branch]; tracked {
			if err := pruneTrackedBranchFromState(repoRoot, state, candidate, a.stdout); err != nil {
				return err
			}
		}

		a.printf("%s -> deleted local branch (merged PR #%d)\n", candidate.Branch, candidate.PR.Number)
	}

	a.printf("%s completed\n", commandName)
	return nil
}

func pruneTrackedBranchFromState(repoRoot string, state *State, candidate pruneLocalCandidate, out io.Writer) error {
	archiveMergedBranch(state, candidate.Branch)
	reparentChildrenAfterMergedDeletion(state, candidate.Branch, candidate.Base, out)
	delete(state.Branches, candidate.Branch)
	pruneArchivedLineage(state)
	if err := saveState(repoRoot, state); err != nil {
		return fmt.Errorf("%s -> deleted locally but failed to update stack state: %w", candidate.Branch, err)
	}
	return nil
}

func buildPruneLocalPlan(state *State, scope pruneLocalScope) (*pruneLocalPlan, error) {
	return buildPruneLocalPlanWithDeps(state, defaultPruneLocalPlanDeps(), scope)
}

func buildPruneLocalPlanWithDeps(state *State, deps pruneLocalPlanDeps, scope pruneLocalScope) (*pruneLocalPlan, error) {
	branches, err := deps.git.ListLocalBranches()
	if err != nil {
		return nil, err
	}
	plan := &pruneLocalPlan{}
	for _, branch := range branches {
		if branch == "" || branch == state.Trunk {
			continue
		}

		_, tracked := state.Branches[branch]
		if tracked {
			if scope.trackedBranches != nil && !scope.trackedBranches[branch] {
				continue
			}
		} else if !scope.includeUntracked {
			continue
		}

		remoteExists, remoteErr := deps.git.RemoteBranchExists(branch)
		if remoteErr != nil {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "remote check failed"})
			continue
		}
		if remoteExists {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "remote branch still exists"})
			continue
		}

		pr, prErr := deps.gh.FindMergedByHead(branch)
		if prErr != nil {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "merged PR lookup failed"})
			continue
		}
		if pr == nil {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "no merged PR found"})
			continue
		}

		base := strings.TrimSpace(pr.BaseRefName)
		if base == "" {
			base = state.Trunk
		}
		if base != state.Trunk {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "merged into non-trunk base"})
			continue
		}

		head := strings.TrimSpace(pr.HeadRefOID)
		if head == "" {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "missing PR head commit"})
			continue
		}
		atOrBehind, headErr := deps.git.BranchAtOrBehindCommit(branch, head)
		if headErr != nil {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "head ancestry check failed"})
			continue
		}
		if !atOrBehind {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "local commits ahead of PR head"})
			continue
		}

		mergeCommit := ""
		if pr.MergeCommit != nil {
			mergeCommit = strings.TrimSpace(pr.MergeCommit.OID)
		}
		if mergeCommit == "" {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "missing merge commit"})
			continue
		}
		contains, containsErr := deps.git.BaseContainsCommit(base, mergeCommit)
		if containsErr != nil {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "merge containment check failed"})
			continue
		}
		if !contains {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "merge commit not in trunk"})
			continue
		}

		plan.Delete = append(plan.Delete, pruneLocalCandidate{Branch: branch, PR: pr, Base: base})
	}

	sort.Slice(plan.Delete, func(i, j int) bool {
		return plan.Delete[i].Branch < plan.Delete[j].Branch
	})
	sort.Slice(plan.Skip, func(i, j int) bool {
		return plan.Skip[i].Branch < plan.Skip[j].Branch
	})
	return plan, nil
}

func printCleanupPlan(out io.Writer, commandName string, plan *pruneLocalPlan) {
	fmt.Fprintf(out, "%s plan:\n", commandName)
	for _, candidate := range plan.Delete {
		fmt.Fprintf(out, "- delete: %s (PR #%d %s)\n", candidate.Branch, candidate.PR.Number, candidate.PR.URL)
	}
	for _, skipped := range plan.Skip {
		fmt.Fprintf(out, "- skip: %s (%s)\n", skipped.Branch, skipped.Reason)
	}
}

func confirmCleanupApply(in io.Reader, out io.Writer, commandName string) bool {
	reader := bufio.NewReader(in)
	fmt.Fprintf(out, "apply %s plan? [y/N]: ", commandName)
	answer, err := readPromptLine(reader)
	if err != nil {
		return false
	}
	return answer == "y" || answer == "yes"
}
