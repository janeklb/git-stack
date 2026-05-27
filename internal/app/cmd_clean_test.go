package app

import (
	"errors"
	"testing"
)

type fakePruneGit struct {
	listLocalBranchesFn  func() ([]string, error)
	remoteBranchExistsFn func(string) (bool, error)
	branchAtOrBehindFn   func(string, string) (bool, error)
	baseContainsCommitFn func(string, string) (bool, error)
	branchIntegratedFn   func(string, string) (bool, error)
}

func (f fakePruneGit) ListLocalBranches() ([]string, error) {
	return f.listLocalBranchesFn()
}

func (f fakePruneGit) RemoteBranchExists(branch string) (bool, error) {
	return f.remoteBranchExistsFn(branch)
}

func (f fakePruneGit) BranchAtOrBehindCommit(branch, commit string) (bool, error) {
	return f.branchAtOrBehindFn(branch, commit)
}

func (f fakePruneGit) BaseContainsCommit(base, commit string) (bool, error) {
	return f.baseContainsCommitFn(base, commit)
}

func (f fakePruneGit) BranchFullyIntegrated(branch, base string) (bool, error) {
	if f.branchIntegratedFn == nil {
		return false, nil
	}
	return f.branchIntegratedFn(branch, base)
}

type fakePruneGH struct {
	findMergedByHeadFn func(string) (*GhPR, error)
	viewFn             func(int) (*GhPR, error)
}

func (f fakePruneGH) FindMergedByHead(branch string) (*GhPR, error) {
	return f.findMergedByHeadFn(branch)
}

func (f fakePruneGH) View(number int) (*GhPR, error) {
	if f.viewFn == nil {
		return nil, nil
	}
	return f.viewFn(number)
}

func TestBuildPruneLocalPlanSelectsEligibleBranchesAndSkipsOthers(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn: func() ([]string, error) {
				return []string{"main", "tracked", "old-clean", "remote", "ahead", "nopr", "wrong-base"}, nil
			},
			remoteBranchExistsFn: func(branch string) (bool, error) {
				return branch == "remote", nil
			},
			branchAtOrBehindFn: func(branch, commit string) (bool, error) {
				if branch == "ahead" {
					return false, nil
				}
				return true, nil
			},
			baseContainsCommitFn: func(base, commit string) (bool, error) {
				return true, nil
			},
		},
		gh: fakePruneGH{findMergedByHeadFn: func(branch string) (*GhPR, error) {
			switch branch {
			case "tracked":
				return &GhPR{Number: 10, URL: "https://example.invalid/pr/10", BaseRefName: "main", HeadRefOID: "h0", MergeCommit: &GhCommit{OID: "m0"}}, nil
			case "old-clean":
				return &GhPR{Number: 11, URL: "https://example.invalid/pr/11", BaseRefName: "main", HeadRefOID: "h1", MergeCommit: &GhCommit{OID: "m1"}}, nil
			case "ahead":
				return &GhPR{Number: 12, URL: "https://example.invalid/pr/12", BaseRefName: "main", HeadRefOID: "h2", MergeCommit: &GhCommit{OID: "m2"}}, nil
			case "wrong-base":
				return &GhPR{Number: 13, URL: "https://example.invalid/pr/13", BaseRefName: "release", HeadRefOID: "h3", MergeCommit: &GhCommit{OID: "m3"}}, nil
			default:
				return nil, nil
			}
		}},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked": {Parent: "main"},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{includeUntracked: true})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 2 {
		t.Fatalf("expected tracked and old-clean branches to be deleted, got %#v", plan.Delete)
	}
	if plan.Delete[0].Branch != "old-clean" || plan.Delete[1].Branch != "tracked" {
		t.Fatalf("expected sorted delete list [old-clean tracked], got %#v", plan.Delete)
	}

	reasons := map[string]string{}
	for _, skip := range plan.Skip {
		reasons[skip.Branch] = skip.Reason
	}
	if reasons["remote"] != "remote branch still exists" {
		t.Fatalf("expected remote skip reason, got %#v", reasons)
	}
	if reasons["ahead"] != "local commits ahead of PR head" {
		t.Fatalf("expected ahead skip reason, got %#v", reasons)
	}
	if reasons["nopr"] != "no merged PR found" {
		t.Fatalf("expected no-PR skip reason, got %#v", reasons)
	}
	if reasons["wrong-base"] != "merged into non-trunk base" {
		t.Fatalf("expected wrong-base skip reason, got %#v", reasons)
	}
}

func TestCleanPrepassLookupJobsFiltersRemoteBranchesBeforeGHLookups(t *testing.T) {
	t.Parallel()

	planned := []cleanPlanBranch{
		{Branch: "local-tracked", HasLocal: true},
		{Branch: "missing-tracked", HasLocal: false},
		{Branch: "still-remote", HasLocal: true},
		{Branch: "remote-error", HasLocal: true},
	}
	plan := &pruneLocalPlan{}
	jobs := cleanPrepassLookupJobs(planned, fakePruneGit{
		remoteBranchExistsFn: func(branch string) (bool, error) {
			switch branch {
			case "still-remote":
				return true, nil
			case "remote-error":
				return false, errors.New("boom")
			default:
				return false, nil
			}
		},
	}, plan)

	if len(jobs) != 2 {
		t.Fatalf("expected only non-remote branches to remain for GH lookup, got %#v", jobs)
	}
	if jobs[0].Branch != "local-tracked" || !jobs[0].HasLocal {
		t.Fatalf("unexpected first job: %#v", jobs[0])
	}
	if jobs[1].Branch != "missing-tracked" || jobs[1].HasLocal {
		t.Fatalf("unexpected second job: %#v", jobs[1])
	}

	reasons := map[string]string{}
	for _, skip := range plan.Skip {
		reasons[skip.Branch] = skip.Reason
	}
	if reasons["still-remote"] != "remote branch still exists" {
		t.Fatalf("expected still-remote skip reason, got %#v", reasons)
	}
	if reasons["remote-error"] != "remote check failed" {
		t.Fatalf("expected remote-error skip reason, got %#v", reasons)
	}
}

func TestBuildPruneLocalPlanDefaultCleanExcludesUntrackedBranches(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn: func() ([]string, error) {
				return []string{"main", "tracked", "untracked"}, nil
			},
			remoteBranchExistsFn: func(string) (bool, error) {
				return false, nil
			},
			branchAtOrBehindFn: func(string, string) (bool, error) {
				return true, nil
			},
			baseContainsCommitFn: func(string, string) (bool, error) {
				return true, nil
			},
		},
		gh: fakePruneGH{findMergedByHeadFn: func(branch string) (*GhPR, error) {
			return &GhPR{Number: 10, URL: "https://example.invalid/pr/10", BaseRefName: "main", HeadRefOID: "h0", MergeCommit: &GhCommit{OID: "m0"}}, nil
		}},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked": {Parent: "main"},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "tracked" {
		t.Fatalf("expected only tracked branch selected, got %#v", plan.Delete)
	}
}

func TestCleanDiscoveryBranchesUsesTrackedScopeAndOptionalGlobalUntracked(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked-in":  {Parent: "main"},
			"tracked-out": {Parent: "main"},
		},
	}
	branches := []string{"main", "tracked-in", "tracked-out", "untracked-a", "untracked-b"}

	withoutUntracked := cleanDiscoveryBranches(state, branches, pruneLocalScope{trackedBranches: map[string]bool{"tracked-in": true}})
	if len(withoutUntracked) != 1 || withoutUntracked[0] != "tracked-in" {
		t.Fatalf("expected only in-scope tracked branch without --untracked, got %#v", withoutUntracked)
	}

	withUntracked := cleanDiscoveryBranches(state, branches, pruneLocalScope{trackedBranches: map[string]bool{"tracked-in": true}, includeUntracked: true})
	if len(withUntracked) != 3 {
		t.Fatalf("expected tracked scope plus global untracked branches, got %#v", withUntracked)
	}
	if withUntracked[0] != "tracked-in" || withUntracked[1] != "untracked-a" || withUntracked[2] != "untracked-b" {
		t.Fatalf("unexpected discovery ordering/content: %#v", withUntracked)
	}
}

func TestCleanTrackedScopeUsesCurrentStackByDefault(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"stack-a-1":    {Parent: "main"},
			"stack-a-2":    {Parent: "stack-a-1"},
			"stack-a-3":    {Parent: "stack-a-2"},
			"stack-a-side": {Parent: "stack-a-1"},
			"stack-b-1":    {Parent: "main"},
		},
	}

	selected := cleanTrackedScope(state, "stack-a-2", false)
	if !selected["stack-a-1"] || !selected["stack-a-2"] || !selected["stack-a-3"] || !selected["stack-a-side"] {
		t.Fatalf("expected current stack selected, got %#v", selected)
	}
	if selected["stack-b-1"] {
		t.Fatalf("did not expect unrelated stack selected, got %#v", selected)
	}
}

func TestCleanTrackedScopeAllSelectsAllTrackedBranches(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"stack-a-1": {Parent: "main"},
			"stack-b-1": {Parent: "main"},
		},
	}

	selected := cleanTrackedScope(state, "stack-a-1", true)
	if !selected["stack-a-1"] || !selected["stack-b-1"] {
		t.Fatalf("expected all tracked branches selected, got %#v", selected)
	}
}

func TestBuildPruneLocalPlanAllowsIntegratedBranchWithoutMergeCommit(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main", "tracked"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			branchAtOrBehindFn:   func(string, string) (bool, error) { return true, nil },
			baseContainsCommitFn: func(string, string) (bool, error) { return false, nil },
			branchIntegratedFn:   func(string, string) (bool, error) { return true, nil },
		},
		gh: fakePruneGH{findMergedByHeadFn: func(string) (*GhPR, error) {
			return &GhPR{Number: 10, URL: "https://example.invalid/pr/10", BaseRefName: "main", HeadRefOID: "h0"}, nil
		}},
	}

	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"tracked": {Parent: "main"}}}
	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "tracked" {
		t.Fatalf("expected integrated branch without merge commit to be cleaned, got %#v", plan.Delete)
	}
	if len(plan.Skip) != 0 {
		t.Fatalf("expected no skips for integrated branch without merge commit, got %#v", plan.Skip)
	}
}

func TestBuildPruneLocalPlanSkipsBranchNotFullyIntegratedWithoutMergeCommit(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main", "tracked"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			branchAtOrBehindFn:   func(string, string) (bool, error) { return true, nil },
			baseContainsCommitFn: func(string, string) (bool, error) { return false, nil },
			branchIntegratedFn:   func(string, string) (bool, error) { return false, nil },
		},
		gh: fakePruneGH{findMergedByHeadFn: func(string) (*GhPR, error) {
			return &GhPR{Number: 10, URL: "https://example.invalid/pr/10", BaseRefName: "main", HeadRefOID: "h0"}, nil
		}},
	}

	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"tracked": {Parent: "main"}}}
	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 0 {
		t.Fatalf("expected non-integrated branch without merge commit to be skipped, got %#v", plan.Delete)
	}
	if len(plan.Skip) != 1 || plan.Skip[0].Reason != "branch not fully integrated into trunk" {
		t.Fatalf("expected integration-based skip, got %#v", plan.Skip)
	}
}

func TestBuildPruneLocalPlanPrefersTrackedPRMetadataBeforeHeadLookup(t *testing.T) {
	t.Parallel()

	findCalls := 0
	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main", "tracked"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			branchAtOrBehindFn:   func(string, string) (bool, error) { return true, nil },
			baseContainsCommitFn: func(string, string) (bool, error) { return true, nil },
		},
		gh: fakePruneGH{
			findMergedByHeadFn: func(string) (*GhPR, error) {
				findCalls++
				return nil, nil
			},
			viewFn: func(number int) (*GhPR, error) {
				if number != 42 {
					return nil, nil
				}
				return &GhPR{Number: 42, URL: "https://example.invalid/pr/42", BaseRefName: "main", HeadRefOID: "h0", State: "MERGED", MergeCommit: &GhCommit{OID: "m0"}}, nil
			},
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked": {Parent: "main", PR: &PRMeta{Number: 42, Base: "main"}},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "tracked" {
		t.Fatalf("expected tracked branch selected via persisted PR metadata, got %#v", plan.Delete)
	}
	if !plan.Delete[0].HasLocal {
		t.Fatalf("expected tracked branch with local ref to stay marked local, got %#v", plan.Delete[0])
	}
	if findCalls != 0 {
		t.Fatalf("expected stored PR lookup to avoid head lookup, got %d head lookups", findCalls)
	}
}

func TestBuildPruneLocalPlanFallsBackToHeadLookupWhenTrackedPRMetadataIsStale(t *testing.T) {
	t.Parallel()

	findCalls := 0
	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main", "tracked"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			branchAtOrBehindFn:   func(string, string) (bool, error) { return true, nil },
			baseContainsCommitFn: func(string, string) (bool, error) { return true, nil },
		},
		gh: fakePruneGH{
			findMergedByHeadFn: func(string) (*GhPR, error) {
				findCalls++
				return &GhPR{Number: 99, URL: "https://example.invalid/pr/99", BaseRefName: "main", HeadRefOID: "h1", State: "MERGED", MergeCommit: &GhCommit{OID: "m1"}}, nil
			},
			viewFn: func(number int) (*GhPR, error) {
				if number != 42 {
					return nil, nil
				}
				return &GhPR{Number: 42, URL: "https://example.invalid/pr/42", BaseRefName: "main", HeadRefOID: "old", State: "CLOSED"}, nil
			},
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked": {Parent: "main", PR: &PRMeta{Number: 42, Base: "main"}},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "tracked" {
		t.Fatalf("expected tracked branch selected via head fallback, got %#v", plan.Delete)
	}
	if plan.Delete[0].PR == nil || plan.Delete[0].PR.Number != 99 {
		t.Fatalf("expected fallback head lookup PR used, got %#v", plan.Delete[0])
	}
	if findCalls != 1 {
		t.Fatalf("expected exactly one head fallback lookup, got %d", findCalls)
	}
}

func TestBuildPruneLocalPlanUsesHeadLookupOnlyForUntrackedLocalBranches(t *testing.T) {
	t.Parallel()

	viewCalls := 0
	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main", "untracked"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			branchAtOrBehindFn:   func(string, string) (bool, error) { return true, nil },
			baseContainsCommitFn: func(string, string) (bool, error) { return true, nil },
		},
		gh: fakePruneGH{
			findMergedByHeadFn: func(string) (*GhPR, error) {
				return &GhPR{Number: 77, URL: "https://example.invalid/pr/77", BaseRefName: "main", HeadRefOID: "h77", State: "MERGED", MergeCommit: &GhCommit{OID: "m77"}}, nil
			},
			viewFn: func(int) (*GhPR, error) {
				viewCalls++
				return nil, nil
			},
		},
	}

	state := &State{Trunk: "main", Branches: map[string]*BranchRef{}}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{includeUntracked: true})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "untracked" {
		t.Fatalf("expected untracked branch selected via head lookup, got %#v", plan.Delete)
	}
	if viewCalls != 0 {
		t.Fatalf("expected no PR-number lookup for untracked branch, got %d", viewCalls)
	}
}

func TestBuildPruneLocalPlanPrunesMissingTrackedStateWithoutPR(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
		},
		gh: fakePruneGH{
			findMergedByHeadFn: func(string) (*GhPR, error) { return nil, nil },
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"ghost": {Parent: "main"},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "ghost" {
		t.Fatalf("expected missing tracked state pruned, got %#v", plan.Delete)
	}
	if plan.Delete[0].HasLocal {
		t.Fatalf("expected ghost branch to have no local ref, got %#v", plan.Delete[0])
	}
}

func TestBuildPruneLocalPlanPrunesMissingTrackedStateWithClosedPR(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn:  func() ([]string, error) { return []string{"main"}, nil },
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
		},
		gh: fakePruneGH{
			findMergedByHeadFn: func(string) (*GhPR, error) { return nil, nil },
			viewFn: func(number int) (*GhPR, error) {
				if number != 42 {
					return nil, nil
				}
				return &GhPR{Number: 42, URL: "https://example.invalid/pr/42", BaseRefName: "main", State: "CLOSED"}, nil
			},
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"ghost": {Parent: "main", PR: &PRMeta{Number: 42, Base: "main"}},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state)})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "ghost" {
		t.Fatalf("expected closed-PR ghost branch pruned, got %#v", plan.Delete)
	}
	if !plan.Delete[0].Stale {
		t.Fatalf("expected ghost branch marked stale, got %#v", plan.Delete[0])
	}
	if plan.Delete[0].HasLocal {
		t.Fatalf("expected ghost branch to have no local ref, got %#v", plan.Delete[0])
	}
	if len(plan.Skip) != 0 {
		t.Fatalf("expected no skips for closed-PR ghost branch, got %#v", plan.Skip)
	}
}
