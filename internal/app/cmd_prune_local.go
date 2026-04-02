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

func defaultPruneLocalPlanDeps() pruneLocalPlanDeps {
	return pruneLocalPlanDeps{
		git: defaultGitClient{},
		gh:  defaultGHClient{},
	}
}

func (a *App) cmdPruneLocal(yes bool) error {
	if err := ensureCleanWorktree(); err != nil {
		return err
	}
	_, state, err := loadStateFromRepo()
	if err != nil {
		return err
	}
	if err := gitRun("fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("prune-local fetch failed: %w", err)
	}

	plan, err := buildPruneLocalPlan(state)
	if err != nil {
		return err
	}
	if len(plan.Delete) == 0 {
		a.println("prune-local: nothing to do")
		return nil
	}

	printPruneLocalPlan(a.stdout, plan)
	if !yes && !confirmPruneLocalApply(a.in, a.stdout) {
		a.println("prune-local cancelled")
		return nil
	}

	current, _ := currentBranch()
	for _, candidate := range plan.Delete {
		if current == candidate.Branch {
			target := state.Trunk
			if strings.TrimSpace(target) == "" {
				target = "main"
			}
			if err := gitRun("switch", target); err != nil {
				a.printf("%s -> failed to switch to %s before deletion: %v\n", candidate.Branch, target, err)
				continue
			}
			current = target
		}
		if err := deleteLocalBranch(candidate.Branch); err != nil {
			a.printf("%s -> failed to delete local branch: %v\n", candidate.Branch, err)
			continue
		}
		a.printf("%s -> deleted local branch (merged PR #%d)\n", candidate.Branch, candidate.PR.Number)
	}

	a.println("prune-local completed")
	return nil
}

func buildPruneLocalPlan(state *State) (*pruneLocalPlan, error) {
	return buildPruneLocalPlanWithDeps(state, defaultPruneLocalPlanDeps())
}

func buildPruneLocalPlanWithDeps(state *State, deps pruneLocalPlanDeps) (*pruneLocalPlan, error) {
	branches, err := deps.git.ListLocalBranches()
	if err != nil {
		return nil, err
	}
	plan := &pruneLocalPlan{}
	for _, branch := range branches {
		if branch == "" || branch == state.Trunk {
			continue
		}
		if _, tracked := state.Branches[branch]; tracked {
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

func printPruneLocalPlan(out io.Writer, plan *pruneLocalPlan) {
	fmt.Fprintln(out, "prune-local plan:")
	for _, candidate := range plan.Delete {
		fmt.Fprintf(out, "- delete: %s (PR #%d %s)\n", candidate.Branch, candidate.PR.Number, candidate.PR.URL)
	}
	for _, skipped := range plan.Skip {
		fmt.Fprintf(out, "- skip: %s (%s)\n", skipped.Branch, skipped.Reason)
	}
}

func confirmPruneLocalApply(in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)
	fmt.Fprint(out, "apply prune-local plan? [y/N]: ")
	answer, err := readPromptLine(reader)
	if err != nil {
		return false
	}
	return answer == "y" || answer == "yes"
}
