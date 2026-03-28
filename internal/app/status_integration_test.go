package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusShowsDriftWhenParentIsNotAncestor(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two"})
		mustWriteFile(t, filepath.Join(repo, "feature2.txt"), "two\n")
		mustGit(t, repo, "add", "feature2.txt")
		mustGit(t, repo, "commit", "-m", "feat two")
		mustGit(t, repo, "switch", "feat-two")
		mustGit(t, repo, "rebase", "--onto", "main", "feat-one")

		out, code := runCLIAndCapture(t, cli, []string{"status"})
		if code != 0 {
			t.Fatalf("status failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "feat-two") {
			t.Fatalf("expected status to include feat-two, got:\n%s", out)
		}
		if !strings.Contains(out, "[drift: parent-not-ancestor]") {
			t.Fatalf("expected drift marker in status output, got:\n%s", out)
		}
	})
}
