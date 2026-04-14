package app

import "testing"

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
}

func (f fakePruneGH) FindMergedByHead(branch string) (*GhPR, error) {
	return f.findMergedByHeadFn(branch)
}

func TestBuildPruneLocalPlanSelectsEligibleBranchesAndSkipsOthers(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		git: fakePruneGit{
			listLocalBranchesFn: func() ([]string, error) {
				return []string{"main", "tracked", "cleanup", "remote", "ahead", "nopr", "wrong-base"}, nil
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
			case "cleanup":
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
		t.Fatalf("expected tracked and cleanup branches to be deleted, got %#v", plan.Delete)
	}
	if plan.Delete[0].Branch != "cleanup" || plan.Delete[1].Branch != "tracked" {
		t.Fatalf("expected sorted delete list [cleanup tracked], got %#v", plan.Delete)
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

func TestBuildPruneLocalPlanDefaultCleanupExcludesUntrackedBranches(t *testing.T) {
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

func TestCleanupDiscoveryBranchesUsesTrackedScopeAndOptionalGlobalUntracked(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked-in":  {Parent: "main"},
			"tracked-out": {Parent: "main"},
		},
	}
	branches := []string{"main", "tracked-in", "tracked-out", "untracked-a", "untracked-b"}

	withoutUntracked := cleanupDiscoveryBranches(state, branches, pruneLocalScope{trackedBranches: map[string]bool{"tracked-in": true}})
	if len(withoutUntracked) != 1 || withoutUntracked[0] != "tracked-in" {
		t.Fatalf("expected only in-scope tracked branch without --untracked, got %#v", withoutUntracked)
	}

	withUntracked := cleanupDiscoveryBranches(state, branches, pruneLocalScope{trackedBranches: map[string]bool{"tracked-in": true}, includeUntracked: true})
	if len(withUntracked) != 3 {
		t.Fatalf("expected tracked scope plus global untracked branches, got %#v", withUntracked)
	}
	if withUntracked[0] != "tracked-in" || withUntracked[1] != "untracked-a" || withUntracked[2] != "untracked-b" {
		t.Fatalf("unexpected discovery ordering/content: %#v", withUntracked)
	}
}

func TestCleanupTrackedScopeUsesCurrentStackByDefault(t *testing.T) {
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

	selected := cleanupTrackedScope(state, "stack-a-2", false)
	if !selected["stack-a-1"] || !selected["stack-a-2"] || !selected["stack-a-3"] || !selected["stack-a-side"] {
		t.Fatalf("expected current stack selected, got %#v", selected)
	}
	if selected["stack-b-1"] {
		t.Fatalf("did not expect unrelated stack selected, got %#v", selected)
	}
}

func TestCleanupTrackedScopeAllSelectsAllTrackedBranches(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"stack-a-1": {Parent: "main"},
			"stack-b-1": {Parent: "main"},
		},
	}

	selected := cleanupTrackedScope(state, "stack-a-1", true)
	if !selected["stack-a-1"] || !selected["stack-b-1"] {
		t.Fatalf("expected all tracked branches selected, got %#v", selected)
	}
}

func TestBuildPruneLocalPlanStrictPolicySkipsBranchWithoutMergeCommit(t *testing.T) {
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

	state := &State{Trunk: "main", Cleanup: CleanupConfig{MergeDetection: cleanupMergeDetectionStrict}, Branches: map[string]*BranchRef{"tracked": {Parent: "main"}}}
	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state), mergeDetection: cleanupMergeDetectionStrict})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 0 {
		t.Fatalf("expected no deletions under strict policy, got %#v", plan.Delete)
	}
	if len(plan.Skip) != 1 || plan.Skip[0].Reason != "missing merge commit" {
		t.Fatalf("expected strict policy to skip missing merge commit, got %#v", plan.Skip)
	}
}

func TestBuildPruneLocalPlanIncludeSquashAllowsIntegratedBranchWithoutMergeCommit(t *testing.T) {
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

	state := &State{Trunk: "main", Cleanup: CleanupConfig{MergeDetection: cleanupMergeDetectionStrict}, Branches: map[string]*BranchRef{"tracked": {Parent: "main"}}}
	plan, err := buildPruneLocalPlanWithDeps(state, deps, pruneLocalScope{trackedBranches: allTrackedBranches(state), mergeDetection: "include-squash"})
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "tracked" {
		t.Fatalf("expected include-squash to allow cleanup, got %#v", plan.Delete)
	}
}
