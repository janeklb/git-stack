package app

import "testing"

func TestRecordRestackContinueProgressLeavesIndexWhenOperationStillActive(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	op := &RestackOperation{Type: "restack", Mode: "rebase", Queue: []string{"feat-one", "feat-two"}, Index: 0}
	completed, err := recordRestackContinueProgress(repo, op, false)
	if err != nil {
		t.Fatalf("recordRestackContinueProgress returned error: %v", err)
	}
	if completed {
		t.Fatal("expected operation to remain incomplete")
	}
	if op.Index != 0 {
		t.Fatalf("expected index to stay at current branch, got %d", op.Index)
	}
}

func TestRecordRestackContinueProgressAdvancesIndexWhenBranchCompletes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	op := &RestackOperation{Type: "restack", Mode: "rebase", Queue: []string{"feat-one", "feat-two"}, Index: 0}
	completed, err := recordRestackContinueProgress(repo, op, true)
	if err != nil {
		t.Fatalf("recordRestackContinueProgress returned error: %v", err)
	}
	if !completed {
		t.Fatal("expected operation to report completion")
	}
	if op.Index != 1 {
		t.Fatalf("expected index to advance to next branch, got %d", op.Index)
	}
}
