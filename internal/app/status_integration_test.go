package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusShowsDriftWhenParentIsNotAncestor(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two"})
	mustWriteFile(t, filepath.Join(repo, "feature2.txt"), "two\n")
	mustGit(t, repo, "add", "feature2.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "switch", "feat-two")
	mustGit(t, repo, "rebase", "--onto", "main", "feat-one")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state", "--drift"})
	if code != 0 {
		t.Fatalf("status failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-two") {
		t.Fatalf("expected status to include feat-two, got:\n%s", out)
	}
	if !strings.Contains(out, "[drift: parent-not-ancestor]") {
		t.Fatalf("expected drift marker in status output, got:\n%s", out)
	}
}

func TestStatusWorksWithoutInitializedState(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustGit(t, repo, "switch", "-c", "feat-one")
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("status failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "main (trunk)") {
		t.Fatalf("expected trunk in status output, got:\n%s", out)
	}
	if !strings.Contains(out, "feat-one") {
		t.Fatalf("expected inferred branch in status output, got:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected plain output without ANSI escapes in non-TTY, got:\n%s", out)
	}
}

func TestStatusShowsStatelessStackCreatedByStackNew(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two"})

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("status failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-one") || !strings.Contains(out, "feat-two") {
		t.Fatalf("expected both inferred branches in status output, got:\n%s", out)
	}
	if !strings.Contains(out, "[local-only]") {
		t.Fatalf("expected local-only state marker in status output, got:\n%s", out)
	}
}
