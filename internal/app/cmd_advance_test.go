package app

import (
	"strings"
	"testing"
)

type fakeAdvanceGit struct {
	remoteBranchExistsFn func(string) (bool, error)
	localBranchExistsFn  func(string) bool
	currentBranchFn      func() (string, error)
	runFn                func(...string) error
	deleteLocalBranchFn  func(string) error
}

func (f fakeAdvanceGit) RemoteBranchExists(branch string) (bool, error) {
	return f.remoteBranchExistsFn(branch)
}

func (f fakeAdvanceGit) LocalBranchExists(branch string) bool {
	return f.localBranchExistsFn(branch)
}

func (f fakeAdvanceGit) CurrentBranch() (string, error) {
	if f.currentBranchFn == nil {
		panic("not used in planner tests")
	}
	return f.currentBranchFn()
}

func (f fakeAdvanceGit) Run(args ...string) error {
	if f.runFn == nil {
		panic("not used in planner tests")
	}
	return f.runFn(args...)
}

func (f fakeAdvanceGit) DeleteLocalBranch(branch string) error {
	if f.deleteLocalBranchFn == nil {
		panic("not used in planner tests")
	}
	return f.deleteLocalBranchFn(branch)
}

type fakeAdvanceGH struct {
	viewFn func(int) (*GhPR, error)
}

func (f fakeAdvanceGH) View(number int) (*GhPR, error) {
	return f.viewFn(number)
}

func TestBuildAdvanceCandidateRequiresRemoteDeletion(t *testing.T) {
	t.Parallel()

	deps := advanceDeps{
		git: fakeAdvanceGit{
			remoteBranchExistsFn: func(branch string) (bool, error) {
				if branch == "feat-a" {
					return true, nil
				}
				return false, nil
			},
			localBranchExistsFn: func(string) bool { return true },
		},
		gh: fakeAdvanceGH{viewFn: func(number int) (*GhPR, error) {
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

	_, err := buildAdvanceCandidateWithDeps(state, "feat-a", deps)
	if err == nil {
		t.Fatalf("expected advance candidate build to fail when remote branch still exists")
	}
	if !strings.Contains(err.Error(), "origin/feat-a still exists") {
		t.Fatalf("expected remote deletion guidance, got: %v", err)
	}
}

func TestAdvanceRebaseBasesAssignsOldBaseToRoots(t *testing.T) {
	t.Parallel()

	bases := advanceRebaseBases([]string{"feat-two", "feat-three"}, "abc123")
	if len(bases) != 2 {
		t.Fatalf("expected two roots, got %#v", bases)
	}
	if bases["feat-two"] != "abc123" || bases["feat-three"] != "abc123" {
		t.Fatalf("expected shared old base for roots, got %#v", bases)
	}
}
