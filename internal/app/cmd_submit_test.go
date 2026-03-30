package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPromptSwitchTargetForMergedBranchDeletionNoChildrenDefaultsToTrunk(t *testing.T) {
	state := &State{
		Trunk: "main",
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "main"},
		},
	}

	out, target, proceed := runPromptSwitchTargetWithInput(t, "", func() (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one")
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

	out, target, proceed := runPromptSwitchTargetWithInput(t, "y\n", func() (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one")
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

	out, target, proceed := runPromptSwitchTargetWithInput(t, "2\n", func() (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one")
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

	out, target, proceed := runPromptSwitchTargetWithInput(t, "9\n", func() (string, bool) {
		return promptSwitchTargetForMergedBranchDeletion(state, "feat-one")
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

func runPromptSwitchTargetWithInput(t *testing.T, input string, fn func() (string, bool)) (string, string, bool) {
	t.Helper()

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if input != "" {
		if _, err := stdinWriter.WriteString(input); err != nil {
			t.Fatalf("write stdin input: %v", err)
		}
	}
	_ = stdinWriter.Close()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	origStdin := os.Stdin
	origStdout := os.Stdout
	os.Stdin = stdinReader
	os.Stdout = stdoutWriter

	target, proceed := fn()

	_ = stdoutWriter.Close()
	os.Stdin = origStdin
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(stdoutReader)
	_ = stdoutReader.Close()
	_ = stdinReader.Close()

	return buf.String(), target, proceed
}
