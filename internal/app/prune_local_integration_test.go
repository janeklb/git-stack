package app

import (
	"strings"
	"testing"
)

func TestPruneLocalNoopSkipsPrompt(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		out, code := runCLIAndCapture(t, cli, []string{"prune-local"})
		if code != 0 {
			t.Fatalf("prune-local failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "prune-local: nothing to do") {
			t.Fatalf("expected noop output, got:\n%s", out)
		}
		if strings.Contains(out, "apply prune-local plan?") {
			t.Fatalf("did not expect prompt for noop prune-local, got:\n%s", out)
		}
	})
}
