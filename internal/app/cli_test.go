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

func TestRefreshPublishFlagNoOptDefaultIsCurrent(t *testing.T) {
	root := New().newRootCmd("stack")
	refresh, _, err := root.Find([]string{"refresh"})
	if err != nil {
		t.Fatalf("find refresh command: %v", err)
	}
	flag := refresh.Flags().Lookup("publish")
	if flag == nil {
		t.Fatalf("expected publish flag to exist")
	}
	if flag.NoOptDefVal != "current" {
		t.Fatalf("expected publish no-opt default current, got %q", flag.NoOptDefVal)
	}
}
