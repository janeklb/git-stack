package app

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeSubmitGitClient struct {
	pushCalls  []string
	local      map[string]bool
	ahead      map[string]bool
	integrated map[string]bool
}

func (f *fakeSubmitGitClient) PushBranch(branch string) error {
	f.pushCalls = append(f.pushCalls, branch)
	return nil
}

func (f *fakeSubmitGitClient) RemoteBranchExists(string) (bool, error) {
	return false, nil
}

func (f *fakeSubmitGitClient) LocalBranchExists(branch string) bool {
	if f.local == nil {
		return true
	}
	return f.local[branch]
}

func (f *fakeSubmitGitClient) BranchHasCommitsSince(base, branch string) (bool, error) {
	_ = base
	if f.ahead == nil {
		return true, nil
	}
	return f.ahead[branch], nil
}

func (f *fakeSubmitGitClient) CurrentBranch() (string, error) {
	return "", nil
}

func (f *fakeSubmitGitClient) Run(args ...string) error {
	_ = args
	return nil
}

func (f *fakeSubmitGitClient) DeleteLocalBranch(string) error {
	return nil
}

func (f *fakeSubmitGitClient) BranchFullyIntegrated(branch, base string) (bool, error) {
	_ = base
	if f.integrated == nil {
		return true, nil
	}
	return f.integrated[branch], nil
}

type fakeSubmitGHClient struct {
	findByHead       map[string]*GhPR
	findMergedByHead map[string]*GhPR
	view             map[int]*GhPR
}

func (f fakeSubmitGHClient) FindByHead(branch string) (*GhPR, error) {
	if pr, ok := f.findByHead[branch]; ok {
		return pr, nil
	}
	return nil, nil
}

func (f fakeSubmitGHClient) FindMergedByHead(branch string) (*GhPR, error) {
	if pr, ok := f.findMergedByHead[branch]; ok {
		return pr, nil
	}
	return nil, nil
}

func (f fakeSubmitGHClient) View(number int) (*GhPR, error) {
	if pr, ok := f.view[number]; ok {
		return pr, nil
	}
	return nil, errors.New("not found")
}

func TestCmdSubmitFailsWhenNoTrackedBranchesExist(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{}}
	git := &fakeSubmitGitClient{}

	err := app.cmdSubmitWithDeps(false, "", "", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not be called for empty queue")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error {
			t.Fatal("sync should not run when no tracked branches exist")
			return nil
		},
		saveState: func(string, *State) error {
			t.Fatal("save should not run when no tracked branches exist")
			return nil
		},
		cleanMergedBranch: func(*State, string, string) (bool, error) {
			t.Fatal("clean should not be called for empty queue")
			return false, nil
		},
	})
	if err == nil {
		t.Fatal("expected submit to fail when no tracked branches exist")
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no pushes, got %v", git.pushCalls)
	}
	if !strings.Contains(err.Error(), "submit requires at least one tracked branch") {
		t.Fatalf("expected tracked-branch error, got %v", err)
	}
}

func TestCmdSubmitMergedPRSkipsPushAndCleansBranch(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main", PR: &PRMeta{Number: 7, URL: "https://old", Base: "main"}},
	}}
	git := &fakeSubmitGitClient{}
	cleaned := ""
	saveCalled := false

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{view: map[int]*GhPR{7: {Number: 7, URL: "https://new", State: "MERGED", BaseRefName: "main"}}},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not be called for merged PR")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error {
			return nil
		},
		saveState: func(string, *State) error {
			saveCalled = true
			return nil
		},
		cleanMergedBranch: func(_ *State, branch string, _ string) (bool, error) {
			cleaned = branch
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if cleaned != "feat-one" {
		t.Fatalf("expected clean for feat-one, got %q", cleaned)
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no push when PR is merged, got %v", git.pushCalls)
	}
	if !saveCalled {
		t.Fatal("expected save after merged PR metadata refresh")
	}
	if got := state.Branches["feat-one"].PR.URL; got != "https://new" {
		t.Fatalf("expected PR URL refreshed from gh view, got %q", got)
	}
}

func TestCmdSubmitPushesEnsuresPRSyncsAndPersists(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "", PR: nil},
	}}
	git := &fakeSubmitGitClient{}
	ensurePRBranch := ""
	ensurePRBase := ""
	ensurePRTrunk := ""
	ensurePRRepoRoot := ""
	syncedAll := false
	syncedBranch := ""
	savedRoot := ""

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(_ *State, all bool, args []string) ([]string, error) {
			if all {
				t.Fatal("expected all=false")
			}
			if len(args) != 1 || args[0] != "feat-one" {
				t.Fatalf("expected branch arg feat-one, got %v", args)
			}
			return []string{"feat-one"}, nil
		},
		ensurePR: func(repoRoot, trunk, branch, parent string, existing *PRMeta, existingPR *GhPR) (*PRMeta, error) {
			if existing != nil {
				t.Fatalf("expected nil existing PR, got %+v", existing)
			}
			if existingPR != nil {
				t.Fatalf("expected nil existing GhPR snapshot, got %+v", existingPR)
			}
			ensurePRRepoRoot = repoRoot
			ensurePRTrunk = trunk
			ensurePRBranch = branch
			ensurePRBase = parent
			return &PRMeta{Number: 11, URL: "https://example.invalid/pr/11", Base: parent}, nil
		},
		syncCurrentStackBody: func(_ *State, all bool, branch string) error {
			syncedAll = all
			syncedBranch = branch
			return nil
		},
		saveState: func(root string, _ *State) error {
			savedRoot = root
			return nil
		},
		cleanMergedBranch: func(*State, string, string) (bool, error) {
			t.Fatal("clean should not be called for open PR path")
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if len(git.pushCalls) != 1 || git.pushCalls[0] != "feat-one" {
		t.Fatalf("expected push for feat-one, got %v", git.pushCalls)
	}
	if ensurePRBranch != "feat-one" {
		t.Fatalf("expected ensurePR for feat-one, got %q", ensurePRBranch)
	}
	if ensurePRTrunk != "main" {
		t.Fatalf("expected trunk main, got %q", ensurePRTrunk)
	}
	if ensurePRRepoRoot != "/tmp/repo" {
		t.Fatalf("expected repo root /tmp/repo, got %q", ensurePRRepoRoot)
	}
	if ensurePRBase != "main" {
		t.Fatalf("expected trunk fallback parent main, got %q", ensurePRBase)
	}
	if syncedAll {
		t.Fatal("expected sync all=false")
	}
	if syncedBranch != "feat-one" {
		t.Fatalf("expected sync branch feat-one, got %q", syncedBranch)
	}
	if savedRoot != "/tmp/repo" {
		t.Fatalf("expected save root /tmp/repo, got %q", savedRoot)
	}
	if state.Branches["feat-one"].PR == nil || state.Branches["feat-one"].PR.Number != 11 {
		t.Fatalf("expected state PR metadata to be updated, got %+v", state.Branches["feat-one"].PR)
	}
}

func TestCmdSubmitSkipsMissingLocalBranch(t *testing.T) {
	var out strings.Builder
	app := NewWithIO(strings.NewReader(""), &out, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main", PR: nil},
	}}
	git := &fakeSubmitGitClient{local: map[string]bool{"feat-one": false}}

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not run for missing local branch")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch:    func(*State, string, string) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no pushes, got %v", git.pushCalls)
	}
	if !strings.Contains(out.String(), "feat-one -> skipped: local branch no longer exists") {
		t.Fatalf("expected missing-branch skip output, got:\n%s", out.String())
	}
}

func TestCmdSubmitSkipsBranchWithoutCommitsBeyondParent(t *testing.T) {
	var out strings.Builder
	app := NewWithIO(strings.NewReader(""), &out, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main", PR: nil},
	}}
	git := &fakeSubmitGitClient{local: map[string]bool{"feat-one": true}, ahead: map[string]bool{"feat-one": false}}

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not run when branch has no commits beyond parent")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch:    func(*State, string, string) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no pushes, got %v", git.pushCalls)
	}
	if !strings.Contains(out.String(), "feat-one -> skipped: no commits beyond main") {
		t.Fatalf("expected empty-range skip output, got:\n%s", out.String())
	}
}

func TestCmdSubmitPrintsNoteWhenNextOnCleanUnused(t *testing.T) {
	var out strings.Builder
	app := NewWithIO(strings.NewReader(""), &out, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"feat-one": {Parent: "main", PR: nil}}}
	git := &fakeSubmitGitClient{}

	err := app.cmdSubmitWithDeps(false, "later-branch", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(_ string, trunk, branch, parent string, existing *PRMeta, existingPR *GhPR) (*PRMeta, error) {
			if trunk != "main" {
				t.Fatalf("expected trunk main, got %q", trunk)
			}
			if branch != "feat-one" {
				t.Fatalf("expected branch feat-one, got %q", branch)
			}
			return &PRMeta{Number: 11, URL: "https://example.invalid/pr/11", Base: parent}, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch:    func(*State, string, string) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if !strings.Contains(out.String(), "submit: note: --next-on-clean was not used") {
		t.Fatalf("expected unused next-on-clean note, got:\n%s", out.String())
	}
}

func TestCmdSubmitTrimsNextOnCleanBeforeCleanCallback(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main", PR: &PRMeta{Number: 7, URL: "https://old", Base: "main"}},
	}}
	seen := ""

	err := app.cmdSubmitWithDeps(false, "  feat-two  ", "feat-one", submitDeps{
		git:                 &fakeSubmitGitClient{},
		gh:                  fakeSubmitGHClient{view: map[int]*GhPR{7: {Number: 7, URL: "https://new", State: "MERGED", BaseRefName: "main"}}},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not be called for merged PR")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch: func(_ *State, _ string, nextOnClean string) (bool, error) {
			seen = nextOnClean
			return true, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if seen != "feat-two" {
		t.Fatalf("expected trimmed next-on-clean, got %q", seen)
	}
}

func TestCmdSubmitRepairsMissingOpenPRMetadataBeforeEnsuringPR(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main"},
	}}
	git := &fakeSubmitGitClient{}
	ensureCalled := false

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git: git,
		gh: fakeSubmitGHClient{findByHead: map[string]*GhPR{
			"feat-one": {Number: 11, URL: "https://example.invalid/pr/11", State: "OPEN", BaseRefName: "main"},
		}},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			ensureCalled = true
			if state.Branches["feat-one"].PR == nil || state.Branches["feat-one"].PR.Number != 11 {
				t.Fatalf("expected repaired PR metadata before ensurePR, got %+v", state.Branches["feat-one"].PR)
			}
			if state.Branches["feat-one"].PR.URL != "https://example.invalid/pr/11" {
				t.Fatalf("expected repaired PR URL, got %+v", state.Branches["feat-one"].PR)
			}
			return state.Branches["feat-one"].PR, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch: func(*State, string, string) (bool, error) {
			t.Fatal("clean should not be called for repaired open PR")
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if !ensureCalled {
		t.Fatal("expected ensurePR to run for repaired open PR")
	}
	if len(git.pushCalls) != 1 || git.pushCalls[0] != "feat-one" {
		t.Fatalf("expected push for feat-one, got %v", git.pushCalls)
	}
}

func TestCmdSubmitRepairsMissingMergedPRMetadataBeforePush(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main"},
	}}
	git := &fakeSubmitGitClient{integrated: map[string]bool{"feat-one": true}}
	cleaned := ""

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git: git,
		gh: fakeSubmitGHClient{findMergedByHead: map[string]*GhPR{
			"feat-one": {Number: 12, URL: "https://example.invalid/pr/12", State: "MERGED", BaseRefName: "main"},
		}},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not run for repaired merged PR")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch: func(*State, string, string) (bool, error) {
			cleaned = "feat-one"
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no push for repaired merged PR, got %v", git.pushCalls)
	}
	if cleaned != "feat-one" {
		t.Fatalf("expected clean for feat-one, got %q", cleaned)
	}
	if state.Branches["feat-one"].PR == nil || state.Branches["feat-one"].PR.Number != 12 {
		t.Fatalf("expected repaired merged PR metadata, got %+v", state.Branches["feat-one"].PR)
	}
}

func TestCmdSubmitFailsWhenRepairedMergedPRIsNotIntegrated(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main"},
	}}
	git := &fakeSubmitGitClient{integrated: map[string]bool{"feat-one": false}}

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git: git,
		gh: fakeSubmitGHClient{findMergedByHead: map[string]*GhPR{
			"feat-one": {Number: 12, URL: "https://example.invalid/pr/12", State: "MERGED", BaseRefName: "main"},
		}},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, true, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not run for repaired merged PR")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanMergedBranch: func(*State, string, string) (bool, error) {
			t.Fatal("clean should not run when repaired merged PR is not integrated")
			return false, nil
		},
	})
	if err == nil {
		t.Fatal("expected submit to fail when repaired merged PR is not integrated")
	}
	if !strings.Contains(err.Error(), "local commits are not fully integrated") {
		t.Fatalf("expected integration failure, got %v", err)
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no pushes, got %v", git.pushCalls)
	}
}
