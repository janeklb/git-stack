package app

import (
	"strings"
	"testing"
	"time"
)

type fakeForwardGit struct {
	listOriginBranchesFn func() ([]string, error)
	remoteBranchExistsFn func(string) (bool, error)
	localBranchExistsFn  func(string) bool
	currentBranchFn      func() (string, error)
	runFn                func(...string) error
	deleteLocalBranchFn  func(string) error
}

func (f fakeForwardGit) ListOriginBranches() ([]string, error) {
	if f.listOriginBranchesFn == nil {
		return nil, nil
	}
	return f.listOriginBranchesFn()
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
	findByHeadFn       func(string) (*GhPR, error)
	findMergedByHeadFn func(string) (*GhPR, error)
	viewFn             func(int) (*GhPR, error)
}

func (f fakeForwardGH) FindByHead(branch string) (*GhPR, error) {
	if f.findByHeadFn == nil {
		return nil, nil
	}
	return f.findByHeadFn(branch)
}

func (f fakeForwardGH) FindMergedByHead(branch string) (*GhPR, error) {
	if f.findMergedByHeadFn == nil {
		return nil, nil
	}
	return f.findMergedByHeadFn(branch)
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

func TestRepairForwardStackPRMetadataRepairsMissingPRMetadataFromMergedHead(t *testing.T) {
	t.Parallel()

	gh := fakeForwardGH{
		findMergedByHeadFn: func(branch string) (*GhPR, error) {
			if branch != "feat-a" {
				t.Fatalf("unexpected merged lookup for %s", branch)
			}
			return &GhPR{Number: 7, URL: "https://example.invalid/pr/7", State: "MERGED", BaseRefName: "main", HeadRefOID: "abc123"}, nil
		},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-a": {Parent: "main"},
		},
	}

	repaired, err := repairForwardStackPRMetadata(state, map[string]bool{"feat-a": true}, gh)
	if err != nil {
		t.Fatalf("expected repaired forward metadata, got error: %v", err)
	}
	if !repaired {
		t.Fatal("expected metadata repair to report a state update")
	}
	if state.Branches["feat-a"].PR == nil || state.Branches["feat-a"].PR.Number != 7 {
		t.Fatalf("expected repaired PR metadata, got %+v", state.Branches["feat-a"].PR)
	}
}

func TestDetectForwardCandidateUsesFetchedOriginState(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo, "branch", "feat-a")

	deps := forwardDeps{
		git: fakeForwardGit{
			remoteBranchExistsFn: func(string) (bool, error) {
				t.Fatal("expected fetched origin state to avoid live remote branch lookup")
				return false, nil
			},
			localBranchExistsFn: func(string) bool { return true },
		},
		gh: fakeForwardGH{viewFn: func(number int) (*GhPR, error) {
			return &GhPR{Number: number, State: "MERGED", BaseRefName: "main", HeadRefOID: "abc123"}, nil
		}},
		mergedCleanIntegrated: func(branch, base string, pr *GhPR) (bool, error) {
			return true, nil
		},
		mergedBranchChildren: func(state *State, branch string) []string {
			return nil
		},
		originBranches: map[string]bool{},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-a": {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
		},
	}

	var (
		eligible bool
		err      error
	)
	withRepoCwd(t, repo, func() {
		_, eligible, err = detectForwardCandidateWithDeps(state, "feat-a", deps)
	})
	if err != nil {
		t.Fatalf("expected cached origin state to work, got error: %v", err)
	}
	if !eligible {
		t.Fatal("expected merged branch to remain eligible when origin branch is absent from fetched state")
	}
}

func TestForwardTargetExistsUsesFetchedOriginState(t *testing.T) {
	t.Parallel()

	exists, err := forwardTargetExists(fakeForwardGit{
		remoteBranchExistsFn: func(string) (bool, error) {
			t.Fatal("expected fetched origin state to avoid live remote branch lookup")
			return false, nil
		},
		localBranchExistsFn: func(string) bool { return false },
	}, map[string]bool{"feat-next": true}, "feat-next")
	if err != nil {
		t.Fatalf("expected cached origin branch to verify target, got error: %v", err)
	}
	if !exists {
		t.Fatal("expected branch present in fetched origin state to exist")
	}
}

func TestBuildForwardCandidatesKeepsDeterministicOrderingWithParallelPRLookup(t *testing.T) {
	repo := newTestRepo(t)
	for _, branch := range []string{"alpha", "bravo", "charlie"} {
		mustGit(t, repo, "branch", branch)
	}

	delays := map[string]time.Duration{
		"alpha":   60 * time.Millisecond,
		"bravo":   20 * time.Millisecond,
		"charlie": 0,
	}
	deps := forwardDeps{
		git: fakeForwardGit{
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			localBranchExistsFn:  func(string) bool { return true },
		},
		gh: fakeForwardGH{viewFn: func(number int) (*GhPR, error) {
			branchByNumber := map[int]string{1: "charlie", 2: "alpha", 3: "bravo"}
			branch := branchByNumber[number]
			time.Sleep(delays[branch])
			return &GhPR{Number: number, State: "MERGED", BaseRefName: "main", HeadRefOID: branch + "-head"}, nil
		}},
		mergedCleanIntegrated: func(string, string, *GhPR) (bool, error) { return true, nil },
		mergedBranchChildren:  func(*State, string) []string { return nil },
		originBranches:        map[string]bool{},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"charlie": {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
			"alpha":   {Parent: "main", PR: &PRMeta{Number: 2, Base: "main"}},
			"bravo":   {Parent: "main", PR: &PRMeta{Number: 3, Base: "main"}},
		},
	}

	var (
		candidates []forwardCleanupCandidate
		err        error
	)
	withRepoCwd(t, repo, func() {
		candidates, err = buildForwardCandidatesWithDeps(state, "main", deps)
	})
	if err != nil {
		t.Fatalf("buildForwardCandidatesWithDeps returned error: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("expected all merged branches selected, got %#v", candidates)
	}
	if candidates[0].Branch != "alpha" || candidates[1].Branch != "bravo" || candidates[2].Branch != "charlie" {
		t.Fatalf("expected sorted topo order despite parallel completion, got %#v", candidates)
	}
	for _, candidate := range candidates {
		if candidate.Head == "" {
			t.Fatalf("expected local candidate head to be resolved, got %#v", candidates)
		}
	}
}

func TestBuildForwardCandidatesParallelPRLookupImprovesWallClock(t *testing.T) {
	repo := newTestRepo(t)
	for _, branch := range []string{"alpha", "bravo", "charlie", "delta"} {
		mustGit(t, repo, "branch", branch)
	}

	deps := forwardDeps{
		git: fakeForwardGit{
			remoteBranchExistsFn: func(string) (bool, error) { return false, nil },
			localBranchExistsFn:  func(string) bool { return true },
		},
		gh: fakeForwardGH{viewFn: func(number int) (*GhPR, error) {
			time.Sleep(75 * time.Millisecond)
			return &GhPR{Number: number, State: "MERGED", BaseRefName: "main", HeadRefOID: "head"}, nil
		}},
		mergedCleanIntegrated: func(string, string, *GhPR) (bool, error) { return true, nil },
		mergedBranchChildren:  func(*State, string) []string { return nil },
		originBranches:        map[string]bool{},
	}

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"alpha":   {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
			"bravo":   {Parent: "main", PR: &PRMeta{Number: 2, Base: "main"}},
			"charlie": {Parent: "main", PR: &PRMeta{Number: 3, Base: "main"}},
			"delta":   {Parent: "main", PR: &PRMeta{Number: 4, Base: "main"}},
		},
	}

	start := time.Now()
	var (
		candidates []forwardCleanupCandidate
		err        error
	)
	withRepoCwd(t, repo, func() {
		candidates, err = buildForwardCandidatesWithDeps(state, "main", deps)
	})
	if err != nil {
		t.Fatalf("buildForwardCandidatesWithDeps returned error: %v", err)
	}
	if len(candidates) != 4 {
		t.Fatalf("expected all merged branches selected, got %#v", candidates)
	}
	if elapsed := time.Since(start); elapsed >= 250*time.Millisecond {
		t.Fatalf("expected bounded parallel PR lookup to finish faster than sequential baseline, took %v", elapsed)
	}
}
