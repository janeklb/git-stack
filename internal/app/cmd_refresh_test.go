package app

import (
	"errors"
	"strings"
	"testing"
)

type fakeRefreshGit struct {
	remoteBranchExistsFn func(string) (bool, error)
	localBranchExistsFn  func(string) bool
}

func (f fakeRefreshGit) RemoteBranchExists(branch string) (bool, error) {
	return f.remoteBranchExistsFn(branch)
}

func (f fakeRefreshGit) LocalBranchExists(branch string) bool {
	return f.localBranchExistsFn(branch)
}

func (f fakeRefreshGit) CurrentBranch() (string, error) {
	panic("not used in planner tests")
}

func (f fakeRefreshGit) Run(args ...string) error {
	panic("not used in planner tests")
}

func (f fakeRefreshGit) DeleteLocalBranch(branch string) error {
	panic("not used in planner tests")
}

type fakeRefreshGH struct {
	viewFn func(int) (*GhPR, error)
}

func (f fakeRefreshGH) View(number int) (*GhPR, error) {
	return f.viewFn(number)
}

func TestCmdRefreshRejectsInvalidPublishValue(t *testing.T) {
	t.Parallel()

	err := New().cmdRefresh(false, "invalid", false, "")
	if err == nil {
		t.Fatalf("expected refresh to fail for invalid publish scope")
	}
	if !strings.Contains(err.Error(), "--publish must be one of: current, all") {
		t.Fatalf("expected validation message, got: %v", err)
	}
}

func TestCmdRefreshRejectsAdvanceCombinationFlags(t *testing.T) {
	t.Parallel()

	err := New().cmdRefresh(true, "", true, "")
	if err == nil || !strings.Contains(err.Error(), "--advance cannot be combined with --restack") {
		t.Fatalf("expected advance/restack validation error, got: %v", err)
	}

	err = New().cmdRefresh(false, "all", true, "")
	if err == nil || !strings.Contains(err.Error(), "--advance cannot be combined with --publish") {
		t.Fatalf("expected advance/publish validation error, got: %v", err)
	}

	err = New().cmdRefresh(false, "", false, "feat-next")
	if err == nil || !strings.Contains(err.Error(), "--next requires --advance") {
		t.Fatalf("expected --next validation error, got: %v", err)
	}
}

func TestBuildRefreshPlanSkipsMergedBranchWhenLocalNotIntegrated(t *testing.T) {
	t.Parallel()

	deps := refreshPlanDeps{
		git: fakeRefreshGit{
			remoteBranchExistsFn: func(branch string) (bool, error) {
				return false, nil
			},
			localBranchExistsFn: func(branch string) bool {
				return true
			},
		},
		gh: fakeRefreshGH{viewFn: func(number int) (*GhPR, error) {
			return &GhPR{Number: number, State: "MERGED"}, nil
		}},
		mergedCleanupIntegrated: func(branch, base string, pr *GhPR) (bool, error) {
			return false, nil
		},
		mergedBranchChildren: func(state *State, branch string) []string {
			return nil
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
		},
	}

	plan, err := buildRefreshPlanWithDeps(state, deps)
	if err != nil {
		t.Fatalf("buildRefreshPlan returned error: %v", err)
	}
	if len(plan.Cleanup) != 0 {
		t.Fatalf("expected no cleanup candidates when local branch is not integrated, got: %#v", plan.Cleanup)
	}
}

func TestBuildRefreshPlanIncludesMergedRemoteDeletedBranches(t *testing.T) {
	t.Parallel()

	deps := refreshPlanDeps{
		git: fakeRefreshGit{
			remoteBranchExistsFn: func(branch string) (bool, error) {
				if branch == "feat-remote" {
					return true, nil
				}
				return false, nil
			},
			localBranchExistsFn: func(branch string) bool {
				return branch == "feat-a"
			},
		},
		gh: fakeRefreshGH{viewFn: func(number int) (*GhPR, error) {
			if number == 2 {
				return &GhPR{Number: number, State: "OPEN"}, nil
			}
			if number == 3 {
				return nil, errors.New("lookup failed")
			}
			return &GhPR{Number: number, State: "MERGED"}, nil
		}},
		mergedCleanupIntegrated: func(branch, base string, pr *GhPR) (bool, error) {
			return true, nil
		},
		mergedBranchChildren: func(state *State, branch string) []string {
			if branch == "feat-a" {
				return []string{"child-one"}
			}
			return nil
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-a":      {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
			"feat-open":   {Parent: "main", PR: &PRMeta{Number: 2, Base: "main"}},
			"feat-err":    {Parent: "main", PR: &PRMeta{Number: 3, Base: "main"}},
			"feat-remote": {Parent: "main", PR: &PRMeta{Number: 4, Base: "main"}},
			"feat-nopr":   {Parent: "main"},
		},
	}

	plan, err := buildRefreshPlanWithDeps(state, deps)
	if err != nil {
		t.Fatalf("buildRefreshPlan returned error: %v", err)
	}
	if len(plan.Cleanup) != 1 {
		t.Fatalf("expected one cleanup candidate, got: %#v", plan.Cleanup)
	}
	if plan.Cleanup[0].Branch != "feat-a" {
		t.Fatalf("expected feat-a candidate, got %#v", plan.Cleanup[0])
	}
	if !plan.Cleanup[0].HasLocal {
		t.Fatalf("expected local candidate, got %#v", plan.Cleanup[0])
	}
	if len(plan.Cleanup[0].Children) != 1 || plan.Cleanup[0].Children[0] != "child-one" {
		t.Fatalf("expected child reparent data, got %#v", plan.Cleanup[0].Children)
	}
}

func TestBuildRefreshAdvanceCandidateRequiresRemoteDeletion(t *testing.T) {
	t.Parallel()

	deps := refreshPlanDeps{
		git: fakeRefreshGit{
			remoteBranchExistsFn: func(branch string) (bool, error) {
				if branch == "feat-a" {
					return true, nil
				}
				return false, nil
			},
			localBranchExistsFn: func(string) bool { return true },
		},
		gh: fakeRefreshGH{viewFn: func(number int) (*GhPR, error) {
			return &GhPR{Number: number, State: "MERGED", BaseRefName: "main"}, nil
		}},
		mergedCleanupIntegrated: func(branch, base string, pr *GhPR) (bool, error) {
			return true, nil
		},
		mergedBranchChildren: func(state *State, branch string) []string {
			return nil
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-a": {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
		},
	}

	_, err := buildRefreshAdvanceCandidateWithDeps(state, "feat-a", deps)
	if err == nil {
		t.Fatalf("expected refresh advance candidate build to fail when remote branch still exists")
	}
	if !strings.Contains(err.Error(), "origin/feat-a still exists") {
		t.Fatalf("expected remote deletion guidance, got: %v", err)
	}
}
