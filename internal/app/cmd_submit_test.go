package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type submitCleanupTargetGitStub struct {
	local map[string]bool
}

func (s submitCleanupTargetGitStub) PushBranch(string) error { return nil }

func (s submitCleanupTargetGitStub) RemoteBranchExists(string) (bool, error) { return false, nil }

func (s submitCleanupTargetGitStub) LocalBranchExists(branch string) bool {
	if s.local == nil {
		return false
	}
	return s.local[branch]
}

func (s submitCleanupTargetGitStub) BranchHasCommitsSince(string, string) (bool, error) {
	return false, nil
}

func (s submitCleanupTargetGitStub) CurrentBranch() (string, error) { return "", nil }

func (s submitCleanupTargetGitStub) Run(...string) error { return nil }

func (s submitCleanupTargetGitStub) DeleteLocalBranch(string) error { return nil }

func (s submitCleanupTargetGitStub) BranchFullyIntegrated(string, string) (bool, error) {
	return true, nil
}

func TestPromptSwitchTargetForMergedBranchDeletionNoChildrenDefaultsToTrunk(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
		},
	}

	out, target, proceed := runPromptSwitchTargetWithInput(t, "", func(in io.Reader, out io.Writer) (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one", in, out)
	})
	if !proceed {
		t.Fatal("expected cleanup to proceed when merged branch has no children")
	}
	if target != "main" {
		t.Fatalf("expected trunk switch target main, got %q", target)
	}
	if !strings.Contains(out, "switching to main before cleanup") {
		t.Fatalf("expected trunk switch message, got:\n%s", out)
	}
}

func TestPromptSwitchTargetForMergedBranchDeletionSingleChildPrompt(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
			"feat-two": {Parent: "feat-one"},
		},
	}

	out, target, proceed := runPromptSwitchTargetWithInput(t, "y\n", func(in io.Reader, out io.Writer) (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one", in, out)
	})
	if !proceed {
		t.Fatal("expected cleanup to proceed when user confirms child switch")
	}
	if target != "feat-two" {
		t.Fatalf("expected switch target feat-two, got %q", target)
	}
	if !strings.Contains(out, "Switch to feat-two and delete this branch? [y/N]:") {
		t.Fatalf("expected single-child prompt, got:\n%s", out)
	}
}

func TestPromptSwitchTargetForMergedBranchDeletionMultipleChildrenSelection(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
			"feat-b":   {Parent: "feat-one"},
			"feat-a":   {Parent: "feat-one"},
		},
	}

	out, target, proceed := runPromptSwitchTargetWithInput(t, "2\n", func(in io.Reader, out io.Writer) (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one", in, out)
	})
	if !proceed {
		t.Fatal("expected cleanup to proceed for valid child selection")
	}
	if target != "feat-b" {
		t.Fatalf("expected sorted selection to choose feat-b for input 2, got %q", target)
	}
	if !strings.Contains(out, "1) feat-a") || !strings.Contains(out, "2) feat-b") {
		t.Fatalf("expected sorted child options, got:\n%s", out)
	}
}

func TestPromptSwitchTargetForMergedBranchDeletionInvalidSelectionKeepsBranch(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
			"feat-a":   {Parent: "feat-one"},
			"feat-b":   {Parent: "feat-one"},
		},
	}

	out, target, proceed := runPromptSwitchTargetWithInput(t, "9\n", func(in io.Reader, out io.Writer) (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one", in, out)
	})
	if proceed {
		t.Fatal("expected cleanup to be cancelled for invalid selection")
	}
	if target != "" {
		t.Fatalf("expected empty switch target when cancelled, got %q", target)
	}
	if !strings.Contains(out, "invalid selection, keeping local branch") {
		t.Fatalf("expected invalid-selection guidance, got:\n%s", out)
	}
}

func TestChooseSubmitCleanupSwitchTargetUsesNextOnCleanup(t *testing.T) {
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"feat-one": {Parent: "main"}}}
	var out bytes.Buffer

	target, proceed, used, err := chooseSubmitCleanupSwitchTarget(state, "feat-one", "feat-two", strings.NewReader(""), &out, submitCleanupTargetGitStub{local: map[string]bool{"feat-two": true}})
	if err != nil {
		t.Fatalf("chooseSubmitCleanupSwitchTarget returned error: %v", err)
	}
	if !proceed {
		t.Fatal("expected cleanup to proceed with next-on-cleanup target")
	}
	if !used {
		t.Fatal("expected next-on-cleanup to be marked used")
	}
	if target != "feat-two" {
		t.Fatalf("expected target feat-two, got %q", target)
	}
	if !strings.Contains(out.String(), "using --next-on-cleanup target feat-two before cleanup") {
		t.Fatalf("expected next-on-cleanup output, got:\n%s", out.String())
	}
}

func TestChooseSubmitCleanupSwitchTargetRejectsMissingNextOnCleanup(t *testing.T) {
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"feat-one": {Parent: "main"}}}

	_, _, used, err := chooseSubmitCleanupSwitchTarget(state, "feat-one", "feat-two", strings.NewReader(""), io.Discard, submitCleanupTargetGitStub{})
	if err == nil {
		t.Fatal("expected missing next-on-cleanup target to fail")
	}
	if !used {
		t.Fatal("expected next-on-cleanup failure to mark flag as used")
	}
	if !strings.Contains(err.Error(), "does not exist locally") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChooseSubmitCleanupSwitchTargetRejectsCurrentBranchTarget(t *testing.T) {
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{"feat-one": {Parent: "main"}}}

	_, _, _, err := chooseSubmitCleanupSwitchTarget(state, "feat-one", "feat-one", strings.NewReader(""), io.Discard, submitCleanupTargetGitStub{local: map[string]bool{"feat-one": true}})
	if err == nil {
		t.Fatal("expected same-branch next-on-cleanup target to fail")
	}
	if !strings.Contains(err.Error(), "cannot be the branch being cleaned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func runPromptSwitchTargetWithInput(t *testing.T, input string, fn func(io.Reader, io.Writer) (string, bool)) (string, string, bool) {
	t.Helper()

	var buf bytes.Buffer
	target, proceed := fn(strings.NewReader(input), &buf)

	return buf.String(), target, proceed
}
