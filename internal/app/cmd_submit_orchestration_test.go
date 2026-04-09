package app

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeSubmitGitClient struct {
	pushCalls []string
	local     map[string]bool
	ahead     map[string]bool
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

func (f *fakeSubmitGitClient) BranchFullyIntegrated(string, string) (bool, error) {
	return true, nil
}

type fakeSubmitGHClient struct {
	view map[int]*GhPR
}

func (f fakeSubmitGHClient) View(number int) (*GhPR, error) {
	if pr, ok := f.view[number]; ok {
		return pr, nil
	}
	return nil, errors.New("not found")
}

func TestCmdSubmitNoQueueSkipsSyncAndSave(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{}}
	git := &fakeSubmitGitClient{}
	syncCalled := false
	saveCalled := false

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
		ensurePR: func(string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not be called for empty queue")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error {
			syncCalled = true
			return nil
		},
		saveState: func(string, *State) error {
			saveCalled = true
			return nil
		},
		cleanupMergedBranch: func(*State, string, string) (bool, error) {
			t.Fatal("cleanup should not be called for empty queue")
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if syncCalled {
		t.Fatal("expected sync not to run when queue is empty")
	}
	if saveCalled {
		t.Fatal("expected save not to run when queue is empty")
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no pushes, got %v", git.pushCalls)
	}
}

func TestCmdSubmitMergedPRSkipsPushAndCleansBranch(t *testing.T) {
	app := NewWithIO(strings.NewReader(""), io.Discard, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"feat-one": {Parent: "main", PR: &PRMeta{Number: 7, URL: "https://old", Base: "main"}},
	}}
	git := &fakeSubmitGitClient{}
	cleaned := ""

	err := app.cmdSubmitWithDeps(false, "", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{view: map[int]*GhPR{7: {Number: 7, URL: "https://new", State: "MERGED", BaseRefName: "main"}}},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, false, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not be called for merged PR")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error {
			return nil
		},
		saveState: func(string, *State) error {
			t.Fatal("save should not run when persisted=false")
			return nil
		},
		cleanupMergedBranch: func(_ *State, branch string, _ string) (bool, error) {
			cleaned = branch
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if cleaned != "feat-one" {
		t.Fatalf("expected cleanup for feat-one, got %q", cleaned)
	}
	if len(git.pushCalls) != 0 {
		t.Fatalf("expected no push when PR is merged, got %v", git.pushCalls)
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
		ensurePR: func(branch, parent string, existing *PRMeta, existingPR *GhPR) (*PRMeta, error) {
			if existing != nil {
				t.Fatalf("expected nil existing PR, got %+v", existing)
			}
			if existingPR != nil {
				t.Fatalf("expected nil existing GhPR snapshot, got %+v", existingPR)
			}
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
		cleanupMergedBranch: func(*State, string, string) (bool, error) {
			t.Fatal("cleanup should not be called for open PR path")
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
			return "/tmp/repo", state, false, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not run for missing local branch")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanupMergedBranch:  func(*State, string, string) (bool, error) { return false, nil },
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
			return "/tmp/repo", state, false, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(string, string, *PRMeta, *GhPR) (*PRMeta, error) {
			t.Fatal("ensurePR should not run when branch has no commits beyond parent")
			return nil, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanupMergedBranch:  func(*State, string, string) (bool, error) { return false, nil },
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

func TestCmdSubmitPrintsNoteWhenNextOnCleanupUnused(t *testing.T) {
	var out strings.Builder
	app := NewWithIO(strings.NewReader(""), &out, io.Discard)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"feat-one": {Parent: "main", PR: nil}}}
	git := &fakeSubmitGitClient{}

	err := app.cmdSubmitWithDeps(false, "later-branch", "feat-one", submitDeps{
		git:                 git,
		gh:                  fakeSubmitGHClient{},
		ensureCleanWorktree: func() error { return nil },
		loadState: func() (string, *State, bool, error) {
			return "/tmp/repo", state, false, nil
		},
		submitQueue: func(*State, bool, []string) ([]string, error) {
			return []string{"feat-one"}, nil
		},
		ensurePR: func(branch, parent string, existing *PRMeta, existingPR *GhPR) (*PRMeta, error) {
			return &PRMeta{Number: 11, URL: "https://example.invalid/pr/11", Base: parent}, nil
		},
		syncCurrentStackBody: func(*State, bool, string) error { return nil },
		saveState:            func(string, *State) error { return nil },
		cleanupMergedBranch:  func(*State, string, string) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatalf("cmdSubmitWithDeps returned error: %v", err)
	}
	if !strings.Contains(out.String(), "submit: note: --next-on-cleanup was not used") {
		t.Fatalf("expected unused next-on-cleanup note, got:\n%s", out.String())
	}
}
