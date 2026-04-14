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
	trackedBranches    map[string]bool
	trackedFromCurrent bool
	allTracked         bool
	mergeDetection     string
	includeUntracked   bool
}

func cleanDiscoveryBranches(state *State, branches []string, scope pruneLocalScope) []string {
	selected := []string{}
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
		selected = append(selected, branch)
	}
	return selected
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

func cleanTrackedScope(state *State, current string, all bool) map[string]bool {
	if all {
		return allTrackedBranches(state)
	}
	return branchesInCurrentStack(state, current)
}

func cleanMergeDetectionPolicy(state *State, includeSquash bool) string {
	if includeSquash {
		return "include-squash"
	}
	policy := state.Clean.MergeDetection
	if policy == "" {
		return cleanMergeDetectionStrict
	}
	return policy
}

func cleanMergeEligible(git pruneGitClient, branch, base string, pr *GhPR, policy string) (bool, string) {
	if strings.TrimSpace(policy) == "" {
		policy = cleanMergeDetectionStrict
	}
	head := pr.HeadRefOID
	if head == "" {
		return false, "missing PR head commit"
	}
	atOrBehind, headErr := git.BranchAtOrBehindCommit(branch, head)
	if headErr != nil {
		return false, "head ancestry check failed"
	}
	if !atOrBehind {
		return false, "local commits ahead of PR head"
	}

	mergeCommit := ""
	if pr.MergeCommit != nil {
		mergeCommit = pr.MergeCommit.OID
	}
	if mergeCommit != "" {
		contains, containsErr := git.BaseContainsCommit(base, mergeCommit)
		if containsErr != nil {
			return false, "merge containment check failed"
		}
		if contains {
			return true, ""
		}
		if policy == cleanMergeDetectionStrict {
			return false, "merge commit not in trunk"
		}
	} else if policy == cleanMergeDetectionStrict {
		return false, "missing merge commit"
	}

	if policy != "include-squash" {
		return false, "unsupported merge detection policy"
	}
	integrated, integratedErr := git.BranchFullyIntegrated(branch, base)
	if integratedErr != nil {
		return false, "integration check failed"
	}
	if !integrated {
		return false, "branch not fully integrated into trunk"
	}
	return true, ""
}

func (a *App) cmdClean(yes bool, all bool, includeSquash bool, untracked bool) error {
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	if _, err := ensurePersistedState(repoRoot, state, persisted, a.stdout); err != nil {
		return err
	}
	return a.runCleanCommand(repoRoot, state, yes, pruneLocalScope{trackedFromCurrent: true, allTracked: all, mergeDetection: cleanMergeDetectionPolicy(state, includeSquash), includeUntracked: untracked})
}

func (a *App) runCleanCommand(repoRoot string, state *State, yes bool, scope pruneLocalScope) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("clean fetch failed: %w", err)
	}
	if scope.trackedBranches == nil {
		if scope.trackedFromCurrent {
			current, err := currentBranch()
			if err != nil {
				return err
			}
			scope.trackedBranches = cleanTrackedScope(state, current, scope.allTracked)
		} else {
			scope.trackedBranches = allTrackedBranches(state)
		}
	}

	plan, err := buildPruneLocalPlan(state, scope)
	if err != nil {
		return err
	}
	if len(plan.Delete) == 0 {
		a.println("clean: nothing to do")
		return nil
	}

	printCleanPlan(a.stdout, plan)
	if !yes && !confirmCleanApply(a.in, a.stdout) {
		a.println("clean cancelled")
		return nil
	}

	current, _ := currentBranch()
	for _, candidate := range plan.Delete {
		if current == candidate.Branch {
			target := state.Trunk
			if target == "" {
				target = "main"
			}
			if err := gitRunQuiet("switch", target); err != nil {
				a.printlnf("%s -> failed to switch to %s before deletion: %v", candidate.Branch, target, err)
				continue
			}
			current = target
		}
		if err := deleteLocalBranch(candidate.Branch); err != nil {
			a.printlnf("%s -> failed to delete local branch: %v", candidate.Branch, err)
			continue
		}

		if _, tracked := state.Branches[candidate.Branch]; tracked {
			if err := pruneTrackedBranchFromState(repoRoot, state, candidate, a.stdout); err != nil {
				return err
			}
		}

		a.printlnf("%s -> deleted local branch (merged PR #%d)", candidate.Branch, candidate.PR.Number)
	}

	a.println("clean completed")
	return nil
}

func pruneTrackedBranchFromState(repoRoot string, state *State, candidate pruneLocalCandidate, out io.Writer) error {
	if err := cleanMergedBranchState(out, state, candidate.Branch, candidate.Base); err != nil {
		return err
	}
	if err := saveState(repoRoot, state); err != nil {
		return fmt.Errorf("%s -> deleted locally but failed to update stack state: %w", candidate.Branch, err)
	}
	return nil
}

func buildPruneLocalPlan(state *State, scope pruneLocalScope) (*pruneLocalPlan, error) {
	return buildPruneLocalPlanWithDeps(state, defaultPruneLocalPlanDeps(), scope)
}

func buildPruneLocalPlanWithDeps(state *State, deps pruneLocalPlanDeps, scope pruneLocalScope) (*pruneLocalPlan, error) {
	if strings.TrimSpace(scope.mergeDetection) == "" {
		scope.mergeDetection = cleanMergeDetectionStrict
	}
	branches, err := deps.git.ListLocalBranches()
	if err != nil {
		return nil, err
	}
	plan := &pruneLocalPlan{}
	for _, branch := range cleanDiscoveryBranches(state, branches, scope) {
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

		base := pr.BaseRefName
		if base == "" {
			base = state.Trunk
		}
		if base != state.Trunk {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: "merged into non-trunk base"})
			continue
		}

		eligible, reason := cleanMergeEligible(deps.git, branch, base, pr, scope.mergeDetection)
		if !eligible {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch, Reason: reason})
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

func printCleanPlan(out io.Writer, plan *pruneLocalPlan) {
	fmt.Fprintln(out, "clean plan:")
	for _, candidate := range plan.Delete {
		fmt.Fprintf(out, "- delete: %s (PR #%d %s)\n", candidate.Branch, candidate.PR.Number, candidate.PR.URL)
	}
	for _, skipped := range plan.Skip {
		fmt.Fprintf(out, "- skip: %s (%s)\n", skipped.Branch, skipped.Reason)
	}
}

func confirmCleanApply(in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)
	fmt.Fprint(out, "apply clean plan? [y/N]: ")
	answer, err := readPromptLine(reader)
	if err != nil {
		return false
	}
	return answer == "y" || answer == "yes"
}
