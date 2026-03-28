package app

import (
	"strings"
	"testing"
)

func TestSubmitWithNoTrackedBranchesIsNoop(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		out, code := runCLIAndCapture(t, cli, []string{"submit", "--all"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "nothing to submit") {
			t.Fatalf("expected noop submit message, got:\n%s", out)
		}
	})
}

func TestSubmitWithoutInitializedStateIsNoop(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		out, code := runCLIAndCapture(t, cli, []string{"submit", "--all"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "nothing to submit") {
			t.Fatalf("expected noop submit message, got:\n%s", out)
		}
	})
}
