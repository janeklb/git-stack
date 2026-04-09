package app

import (
	"strings"
	"testing"
)

func TestHelpIncludesCompletionCommand(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help"})
	if code != 0 {
		t.Fatalf("help failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "completion") {
		t.Fatalf("expected help output to mention completion command, got:\n%s", out)
	}
}

func TestCompletionBashOutputsScript(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"completion", "bash"})
	if code != 0 {
		t.Fatalf("completion bash failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "__start_stack") {
		t.Fatalf("expected bash completion function in output, got:\n%s", out)
	}
}

func TestKeyCommandFlagsExist(t *testing.T) {
	root := New().newRootCmd("stack")
	advance, _, err := root.Find([]string{"advance"})
	if err != nil {
		t.Fatalf("find advance command: %v", err)
	}
	if advance.Flags().Lookup("next") == nil {
		t.Fatalf("expected advance next flag to exist")
	}
	submit, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit command: %v", err)
	}
	if submit.Flags().Lookup("next-on-cleanup") == nil {
		t.Fatalf("expected submit next-on-cleanup flag to exist")
	}
	cleanup, _, err := root.Find([]string{"cleanup"})
	if err != nil {
		t.Fatalf("find cleanup command: %v", err)
	}
	if cleanup.Flags().Lookup("yes") == nil {
		t.Fatalf("expected cleanup yes flag to exist")
	}
	if cleanup.Flags().Lookup("all") == nil {
		t.Fatalf("expected cleanup all flag to exist")
	}
	if cleanup.Flags().Lookup("include-squash") == nil {
		t.Fatalf("expected cleanup include-squash flag to exist")
	}
	if cleanup.Flags().Lookup("untracked") == nil {
		t.Fatalf("expected cleanup untracked flag to exist")
	}
	reparent, _, err := root.Find([]string{"reparent"})
	if err != nil {
		t.Fatalf("find reparent command: %v", err)
	}
	if reparent.Flags().Lookup("preserve-lineage") == nil {
		t.Fatalf("expected reparent preserve-lineage flag to exist")
	}
	newCmd, _, err := root.Find([]string{"new"})
	if err != nil {
		t.Fatalf("find new command: %v", err)
	}
	if newCmd.Flags().Lookup("adopt") == nil {
		t.Fatalf("expected new adopt flag to exist")
	}
}

func TestStatusHasStatAlias(t *testing.T) {
	root := New().newRootCmd("stack")
	cmd, _, err := root.Find([]string{"stat"})
	if err != nil {
		t.Fatalf("find stat command: %v", err)
	}
	if cmd.Name() != "status" {
		t.Fatalf("expected alias to resolve to status, got %q", cmd.Name())
	}
}

func TestInitHelpMarksCommandAsRepairFlow(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help", "init"})
	if code != 0 {
		t.Fatalf("help init failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "Initialize or repair persisted stack state") {
		t.Fatalf("expected init help to mention repair/config role, got:\n%s", out)
	}
	if !strings.Contains(out, "auto-bootstrap state when possible") {
		t.Fatalf("expected init help to mention auto-bootstrap happy path, got:\n%s", out)
	}
}
