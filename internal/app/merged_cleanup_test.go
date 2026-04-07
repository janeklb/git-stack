package app

import (
	"bytes"
	"errors"
	"testing"
)

type fakeMergedCleanupGit struct {
	currentBranch string
	runCalls      [][]string
	deleted       []string
	runErr        error
	deleteErr     error
}

func (f *fakeMergedCleanupGit) CurrentBranch() (string, error) {
	return f.currentBranch, nil
}

func (f *fakeMergedCleanupGit) Run(args ...string) error {
	f.runCalls = append(f.runCalls, append([]string{}, args...))
	return f.runErr
}

func (f *fakeMergedCleanupGit) DeleteLocalBranch(branch string) error {
	f.deleted = append(f.deleted, branch)
	return f.deleteErr
}

func TestSwitchAwayThenDeleteMergedBranchSwitchesAndDeletesCurrentBranch(t *testing.T) {
	t.Parallel()

	git := &fakeMergedCleanupGit{currentBranch: "feat-one"}
	if err := switchAwayThenDeleteMergedBranch(git, "feat-one", true, "main"); err != nil {
		t.Fatalf("switchAwayThenDeleteMergedBranch returned error: %v", err)
	}
	if len(git.runCalls) != 1 || len(git.runCalls[0]) != 2 || git.runCalls[0][0] != "switch" || git.runCalls[0][1] != "main" {
		t.Fatalf("expected switch to main before deletion, got %#v", git.runCalls)
	}
	if len(git.deleted) != 1 || git.deleted[0] != "feat-one" {
		t.Fatalf("expected feat-one to be deleted, got %#v", git.deleted)
	}
}

func TestSwitchAwayThenDeleteMergedBranchRequiresTargetWhenCurrentBranchMatches(t *testing.T) {
	t.Parallel()

	git := &fakeMergedCleanupGit{currentBranch: "feat-one"}
	err := switchAwayThenDeleteMergedBranch(git, "feat-one", true, "")
	if err == nil {
		t.Fatal("expected error when no switch target is available")
	}
	if got := err.Error(); got != "no switch target available before cleanup of feat-one" {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(git.deleted) != 0 {
		t.Fatalf("expected no deletion when switch target missing, got %#v", git.deleted)
	}
}

func TestSwitchAwayThenDeleteMergedBranchReturnsDeleteError(t *testing.T) {
	t.Parallel()

	git := &fakeMergedCleanupGit{currentBranch: "main", deleteErr: errors.New("boom")}
	err := switchAwayThenDeleteMergedBranch(git, "feat-one", true, "")
	if err == nil {
		t.Fatal("expected delete error")
	}
	if got := err.Error(); got != "failed to delete local merged branch feat-one: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanupMergedBranchStateReparentsChildrenAndArchivesLineage(t *testing.T) {
	t.Parallel()

	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main", PR: &PRMeta{Number: 1, Base: "main"}},
			"feat-two": {Parent: "feat-one", LineageParent: "feat-one", PR: &PRMeta{Number: 2, Base: "feat-one"}},
		},
	}

	var out bytes.Buffer
	if err := cleanupMergedBranchState(&out, state, "feat-one", "main"); err != nil {
		t.Fatalf("cleanupMergedBranchState returned error: %v", err)
	}

	if _, ok := state.Branches["feat-one"]; ok {
		t.Fatal("expected feat-one removed from active branches")
	}
	if got := state.Branches["feat-two"].Parent; got != "main" {
		t.Fatalf("expected feat-two reparented to main, got %q", got)
	}
	if got := state.Branches["feat-two"].PR.Base; got != "main" {
		t.Fatalf("expected feat-two PR base updated to main, got %q", got)
	}
	if state.Archived["feat-one"] == nil {
		t.Fatal("expected feat-one archived for lineage preservation")
	}
}

func TestCleanupMergedBranchStateErrorsForMissingTrackedBranch(t *testing.T) {
	t.Parallel()

	state := &State{Trunk: "main", Branches: map[string]*BranchRef{}}
	var out bytes.Buffer
	err := cleanupMergedBranchState(&out, state, "missing", "main")
	if err == nil {
		t.Fatal("expected cleanupMergedBranchState to reject missing branch metadata")
	}
	if got := err.Error(); got != "cleanup requires tracked branch metadata for missing" {
		t.Fatalf("unexpected error: %v", err)
	}
}
