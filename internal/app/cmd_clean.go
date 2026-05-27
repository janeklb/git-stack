package app

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const cleanLookupConcurrency = 4

type pruneLocalCandidate struct {
	Branch   string
	PR       *GhPR
	Base     string
	HasLocal bool
	Stale    bool
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

type cleanPlanBranch struct {
	Branch   string
	HasLocal bool
}

type cleanLookupJob struct {
	Branch   string
	HasLocal bool
}

type cleanLookupResult struct {
	Branch     string
	HasLocal   bool
	Candidate  *pruneLocalCandidate
	PR         *GhPR
	SkipReason string
}

type pruneLocalScope struct {
	trackedBranches    map[string]bool
	trackedFromCurrent bool
	allTracked         bool
	includeUntracked   bool
}

func cleanDiscoveryBranches(state *State, branches []string, scope pruneLocalScope) []string {
	seen := map[string]bool{}
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
		seen[branch] = true
	}
	for branch := range state.Branches {
		if branch == "" || branch == state.Trunk || seen[branch] {
			continue
		}
		if scope.trackedBranches != nil && !scope.trackedBranches[branch] {
			continue
		}
		seen[branch] = true
	}
	selected := make([]string, 0, len(seen))
	for branch := range seen {
		selected = append(selected, branch)
	}
	sort.Strings(selected)
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

func cleanMergeEligible(git pruneGitClient, branch, base string, pr *GhPR) (bool, string) {
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

func (a *App) cmdClean(yes bool, all bool, untracked bool) error {
	repoRoot, state, persisted, err := loadStateFromRepoOrInfer()
	if err != nil {
		return err
	}
	if untracked {
		if _, err := ensurePersistedState(repoRoot, state, persisted, a.stdout); err != nil {
			return err
		}
	} else {
		if err := requirePersistedTrackedState(state, persisted, "clean"); err != nil {
			return err
		}
	}
	return a.runCleanCommand(repoRoot, state, yes, pruneLocalScope{trackedFromCurrent: true, allTracked: all, includeUntracked: untracked})
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
		if candidate.HasLocal && current == candidate.Branch {
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
		if candidate.HasLocal {
			if err := deleteLocalBranch(candidate.Branch); err != nil {
				a.printlnf("%s -> failed to delete local branch: %v", candidate.Branch, err)
				continue
			}
		}

		if _, tracked := state.Branches[candidate.Branch]; tracked {
			if err := pruneTrackedBranchFromState(repoRoot, state, candidate, a.stdout); err != nil {
				return err
			}
		}

		if candidate.PR != nil {
			if candidate.Stale {
				a.printlnf("%s -> pruned stale tracked branch from stack state (closed PR #%d)", candidate.Branch, candidate.PR.Number)
			} else if candidate.HasLocal {
				a.printlnf("%s -> deleted local branch (merged PR #%d)", candidate.Branch, candidate.PR.Number)
			} else {
				a.printlnf("%s -> pruned tracked branch from stack state (merged PR #%d)", candidate.Branch, candidate.PR.Number)
			}
		} else {
			a.printlnf("%s -> pruned missing tracked branch from stack state", candidate.Branch)
		}
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
	branches, err := deps.git.ListLocalBranches()
	if err != nil {
		return nil, err
	}
	discovered := cleanPlanBranches(state, branches, scope)
	plan := &pruneLocalPlan{}
	jobs := cleanPrepassLookupJobs(discovered, deps.git, plan)
	results := cleanResolveLookupJobs(state, deps.gh, jobs)
	cleanAssembleLookupResults(state, deps.git, results, plan)

	sort.Slice(plan.Delete, func(i, j int) bool {
		return plan.Delete[i].Branch < plan.Delete[j].Branch
	})
	sort.Slice(plan.Skip, func(i, j int) bool {
		return plan.Skip[i].Branch < plan.Skip[j].Branch
	})
	return plan, nil
}

func cleanPlanBranches(state *State, branches []string, scope pruneLocalScope) []cleanPlanBranch {
	localBranches := map[string]bool{}
	for _, branch := range branches {
		localBranches[branch] = true
	}
	discovered := cleanDiscoveryBranches(state, branches, scope)
	planned := make([]cleanPlanBranch, 0, len(discovered))
	for _, branch := range discovered {
		planned = append(planned, cleanPlanBranch{Branch: branch, HasLocal: localBranches[branch]})
	}
	return planned
}

func cleanPrepassLookupJobs(branches []cleanPlanBranch, git pruneGitClient, plan *pruneLocalPlan) []cleanLookupJob {
	jobs := make([]cleanLookupJob, 0, len(branches))
	for _, branch := range branches {
		remoteExists, remoteErr := git.RemoteBranchExists(branch.Branch)
		if remoteErr != nil {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch.Branch, Reason: "remote check failed"})
			continue
		}
		if remoteExists {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: branch.Branch, Reason: "remote branch still exists"})
			continue
		}
		jobs = append(jobs, cleanLookupJob{Branch: branch.Branch, HasLocal: branch.HasLocal})
	}
	return jobs
}

func cleanResolveLookupJobs(state *State, gh pruneGHClient, jobs []cleanLookupJob) []cleanLookupResult {
	if len(jobs) == 0 {
		return nil
	}

	jobCh := make(chan cleanLookupJob)
	resultCh := make(chan cleanLookupResult, len(jobs))
	workerCount := cleanLookupConcurrency
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for job := range jobCh {
				resultCh <- cleanResolveLookupJob(state, gh, job)
			}
		}()
	}

	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)
	wg.Wait()
	close(resultCh)

	results := make([]cleanLookupResult, 0, len(jobs))
	for result := range resultCh {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Branch < results[j].Branch
	})
	return results
}

func cleanResolveLookupJob(state *State, gh pruneGHClient, job cleanLookupJob) cleanLookupResult {
	if !job.HasLocal {
		candidate, ok, err := buildMissingTrackedBranchCandidate(state, gh, job.Branch)
		if err != nil {
			return cleanLookupResult{Branch: job.Branch, SkipReason: "merged PR lookup failed"}
		}
		if !ok {
			return cleanLookupResult{Branch: job.Branch, SkipReason: "no merged PR found"}
		}
		return cleanLookupResult{Branch: job.Branch, Candidate: &candidate}
	}

	pr, err := cleanResolveLocalBranchPR(state, gh, job.Branch)
	if err != nil {
		return cleanLookupResult{Branch: job.Branch, HasLocal: true, SkipReason: "merged PR lookup failed"}
	}
	if pr == nil {
		return cleanLookupResult{Branch: job.Branch, HasLocal: true, SkipReason: "no merged PR found"}
	}
	return cleanLookupResult{Branch: job.Branch, HasLocal: true, PR: pr}
}

func cleanAssembleLookupResults(state *State, git pruneGitClient, results []cleanLookupResult, plan *pruneLocalPlan) {
	for _, result := range results {
		if result.SkipReason != "" {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: result.Branch, Reason: result.SkipReason})
			continue
		}
		if !result.HasLocal {
			plan.Delete = append(plan.Delete, *result.Candidate)
			continue
		}

		base := result.PR.BaseRefName
		if base == "" {
			base = state.Trunk
		}
		if base != state.Trunk {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: result.Branch, Reason: "merged into non-trunk base"})
			continue
		}

		eligible, reason := cleanMergeEligible(git, result.Branch, base, result.PR)
		if !eligible {
			plan.Skip = append(plan.Skip, pruneLocalSkip{Branch: result.Branch, Reason: reason})
			continue
		}

		plan.Delete = append(plan.Delete, pruneLocalCandidate{Branch: result.Branch, PR: result.PR, Base: base, HasLocal: true})
	}
}

func cleanResolveLocalBranchPR(state *State, gh pruneGHClient, branch string) (*GhPR, error) {
	meta := state.Branches[branch]
	if meta != nil && meta.PR != nil && meta.PR.Number > 0 {
		pr, err := gh.View(meta.PR.Number)
		if err != nil {
			return nil, err
		}
		if pr != nil && strings.EqualFold(pr.State, "MERGED") {
			return pr, nil
		}
	}
	return gh.FindMergedByHead(branch)
}

func buildMissingTrackedBranchCandidate(state *State, gh pruneGHClient, branch string) (pruneLocalCandidate, bool, error) {
	meta := state.Branches[branch]
	if meta == nil {
		return pruneLocalCandidate{}, false, nil
	}
	base := meta.Parent
	if base == "" {
		base = state.Trunk
	}
	pr, err := cleanTrackedMergedPR(state, gh, branch)
	if err != nil {
		return pruneLocalCandidate{}, false, err
	}
	if pr != nil {
		if pr.BaseRefName != "" {
			base = pr.BaseRefName
		} else if meta.PR != nil && meta.PR.Base != "" {
			base = meta.PR.Base
		}
		return pruneLocalCandidate{Branch: branch, PR: pr, Base: base, Stale: !strings.EqualFold(pr.State, "MERGED")}, true, nil
	}
	if meta.PR == nil || meta.PR.Number <= 0 {
		return pruneLocalCandidate{Branch: branch, Base: base}, true, nil
	}
	return pruneLocalCandidate{}, false, nil
}

func cleanTrackedMergedPR(state *State, gh pruneGHClient, branch string) (*GhPR, error) {
	meta := state.Branches[branch]
	if meta == nil || meta.PR == nil || meta.PR.Number <= 0 {
		return nil, nil
	}
	pr, err := gh.View(meta.PR.Number)
	if err != nil {
		return nil, err
	}
	if pr == nil || !strings.EqualFold(pr.State, "MERGED") {
		if strings.EqualFold(pr.State, "CLOSED") {
			return pr, nil
		}
		return nil, nil
	}
	return pr, nil
}

func printCleanPlan(out io.Writer, plan *pruneLocalPlan) {
	fmt.Fprintln(out, "clean plan:")
	for _, candidate := range plan.Delete {
		if candidate.Stale && candidate.PR != nil {
			fmt.Fprintf(out, "- delete: %s (stale tracked state; closed PR #%d %s)\n", candidate.Branch, candidate.PR.Number, candidate.PR.URL)
			continue
		}
		if candidate.PR == nil {
			fmt.Fprintf(out, "- delete: %s (stale tracked state)\n", candidate.Branch)
			continue
		}
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
