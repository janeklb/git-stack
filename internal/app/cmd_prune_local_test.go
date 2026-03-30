package app

import "testing"

func TestBuildPruneLocalPlanSelectsEligibleBranchAndSkipsOthers(t *testing.T) {
	t.Parallel()

	deps := pruneLocalPlanDeps{
		listLocalBranches: func() ([]string, error) {
			return []string{"main", "tracked", "cleanup", "remote", "ahead", "nopr", "wrong-base"}, nil
		},
		remoteBranchExists: func(branch string) (bool, error) {
			return branch == "remote", nil
		},
		findMergedByHead: func(branch string) (*GhPR, error) {
			switch branch {
			case "cleanup":
				return &GhPR{Number: 11, URL: "https://example.invalid/pr/11", BaseRefName: "main", HeadRefOID: "h1", MergeCommit: &GhCommit{OID: "m1"}}, nil
			case "ahead":
				return &GhPR{Number: 12, URL: "https://example.invalid/pr/12", BaseRefName: "main", HeadRefOID: "h2", MergeCommit: &GhCommit{OID: "m2"}}, nil
			case "wrong-base":
				return &GhPR{Number: 13, URL: "https://example.invalid/pr/13", BaseRefName: "release", HeadRefOID: "h3", MergeCommit: &GhCommit{OID: "m3"}}, nil
			default:
				return nil, nil
			}
		},
		branchAtOrBehind: func(branch, commit string) (bool, error) {
			if branch == "ahead" {
				return false, nil
			}
			return true, nil
		},
		baseContainsCommit: func(base, commit string) (bool, error) {
			return true, nil
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"tracked": {Parent: "main"},
		},
	}

	plan, err := buildPruneLocalPlanWithDeps(state, deps)
	if err != nil {
		t.Fatalf("buildPruneLocalPlan returned error: %v", err)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Branch != "cleanup" {
		t.Fatalf("expected only cleanup branch to be deleted, got %#v", plan.Delete)
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
