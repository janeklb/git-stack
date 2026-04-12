package app

import (
	"strings"
	"testing"
)

func TestInitPrintsRepairModeNote(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"init", "--trunk", "main"})
	if code != 0 {
		t.Fatalf("init failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "repair/reconfiguration command") {
		t.Fatalf("expected init note about repair role, got:\n%s", out)
	}
	if !strings.Contains(out, "initialized stack state") {
		t.Fatalf("expected init success output, got:\n%s", out)
	}
}
