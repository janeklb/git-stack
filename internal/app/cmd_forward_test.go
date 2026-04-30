package app

import (
	"strings"
	"testing"
)

type fakeForwardGit struct {
	remoteBranchExistsFn func(string) (bool, error)
	localBranchExistsFn  func(string) bool
	currentBranchFn      func() (string, error)
	runFn                func(...string) error
	deleteLocalBranchFn  func(string) error
}

func (f fakeForwardGit) RemoteBranchExists(branch string) (bool, error) {
	return f.remoteBranchExistsFn(branch)
}

func (f fakeForwardGit) LocalBranchExists(branch string) bool {
	return f.localBranchExistsFn(branch)
}

func (f fakeForwardGit) CurrentBranch() (string, error) {
	if f.currentBranchFn == nil {
		panic("not used in planner tests")
	}
	return f.currentBranchFn()
}

func (f fakeForwardGit) Run(args ...string) error {
	if f.runFn == nil {
		panic("not used in planner tests")
	}
	return f.runFn(args...)
}

func (f fakeForwardGit) DeleteLocalBranch(branch string) error {
	if f.deleteLocalBranchFn == nil {
		panic("not used in planner tests")
	}
	return f.deleteLocalBranchFn(branch)
}

type fakeForwardGH struct {
	viewFn func(int) (*GhPR, error)
}

func (f fakeForwardGH) View(number int) (*GhPR, error) {
	return f.viewFn(number)
}

func TestBuildForwardCandidateRequiresRemoteDeletion(t *testing.T) {
	t.Parallel()

	deps := forwardDeps{
		git: fakeForwardGit{
			remoteBranchExistsFn: func(branch string) (bool, error) {
				if branch == "feat-a" {
					return true, nil
				}
				return false, nil
			},
			localBranchExistsFn: func(string) bool { return true },
		},
		gh: fakeForwardGH{viewFn: func(number int) (*GhPR, error) {
			return &GhPR{Number: number, State: "MERGED", BaseRefName: "main"}, nil
		}},
		mergedCleanIntegrated: func(branch, base string, pr *GhPR) (bool, error) {
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

	_, err := buildForwardCandidateWithDeps(state, "feat-a", deps)
	if err == nil {
		t.Fatalf("expected forward candidate build to fail when remote branch still exists")
	}
	if !strings.Contains(err.Error(), "origin/feat-a still exists") {
		t.Fatalf("expected remote deletion guidance, got: %v", err)
	}
}

func TestForwardRebaseBasesAssignsOldBaseToRoots(t *testing.T) {
	t.Parallel()

	bases := forwardRebaseBases([]string{"feat-two", "feat-three"}, "abc123")
	if len(bases) != 2 {
		t.Fatalf("expected two roots, got %#v", bases)
	}
	if bases["feat-two"] != "abc123" || bases["feat-three"] != "abc123" {
		t.Fatalf("expected shared old base for roots, got %#v", bases)
	}
}

func TestBuildForwardCandidateDeletedLocalBranchWithoutMergeCommitShowsRepair(t *testing.T) {
	t.Parallel()

	deps := forwardDeps{
		git: fakeForwardGit{
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			localBranchExistsFn:  func(string) bool { return false },
		},
		gh: fakeForwardGH{viewFn: func(number int) (*GhPR, error) {
			return &GhPR{Number: number, State: "MERGED", BaseRefName: "main"}, nil
		}},
		mergedCleanIntegrated: func(string, string, *GhPR) (bool, error) {
			t.Fatal("mergedCleanIntegrated should not run when local branch is already deleted")
			return false, nil
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

	_, err := buildForwardCandidateWithDeps(state, "feat-a", deps)
	if err == nil {
		t.Fatal("expected deleted local branch without merge commit to fail")
	}
	if !strings.Contains(err.Error(), "repair with: git-stack clean --all --yes && git-stack restack && git-stack submit") {
		t.Fatalf("expected repair guidance, got: %v", err)
	}
}
