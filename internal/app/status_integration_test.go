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

		out, code := runCLIAndCapture(t, cli, []string{"status", "--drift"})
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

func TestStatusWorksWithoutInitializedState(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustGit(t, repo, "switch", "-c", "feat-one")
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		out, code := runCLIAndCapture(t, cli, []string{"status"})
		if code != 0 {
			t.Fatalf("status failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "- main") {
			t.Fatalf("expected trunk in status output, got:\n%s", out)
		}
		if !strings.Contains(out, "feat-one") {
			t.Fatalf("expected inferred branch in status output, got:\n%s", out)
		}
	})
}

func TestStatusShowsStatelessStackCreatedByStackNew(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two"})

		out, code := runCLIAndCapture(t, cli, []string{"status"})
		if code != 0 {
			t.Fatalf("status failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "feat-one") || !strings.Contains(out, "feat-two") {
			t.Fatalf("expected both inferred branches in status output, got:\n%s", out)
		}
		if !strings.Contains(out, "[local-only]") {
			t.Fatalf("expected local-only state marker in status output, got:\n%s", out)
		}
	})
}

func TestStatusDefaultsToCurrentStackOnly(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "stack-a-1", "--parent", "main"})
		mustWriteFile(t, filepath.Join(repo, "a1.txt"), "a1\n")
		mustGit(t, repo, "add", "a1.txt")
		mustGit(t, repo, "commit", "-m", "a1")

		mustRunCLI(t, cli, []string{"new", "stack-a-2"})
		mustWriteFile(t, filepath.Join(repo, "a2.txt"), "a2\n")
		mustGit(t, repo, "add", "a2.txt")
		mustGit(t, repo, "commit", "-m", "a2")

		mustRunCLI(t, cli, []string{"new", "--parent", "main", "stack-b-1"})
		mustWriteFile(t, filepath.Join(repo, "b1.txt"), "b1\n")
		mustGit(t, repo, "add", "b1.txt")
		mustGit(t, repo, "commit", "-m", "b1")

		mustGit(t, repo, "switch", "stack-a-2")
		out, code := runCLIAndCapture(t, cli, []string{"status"})
		if code != 0 {
			t.Fatalf("status failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "stack-a-1") || !strings.Contains(out, "stack-a-2") {
			t.Fatalf("expected current stack branches in output, got:\n%s", out)
		}
		if strings.Contains(out, "stack-b-1") {
			t.Fatalf("did not expect unrelated stack branch in default status, got:\n%s", out)
		}

		outAll, codeAll := runCLIAndCapture(t, cli, []string{"status", "--all"})
		if codeAll != 0 {
			t.Fatalf("status --all failed: exit=%d\n%s", codeAll, outAll)
		}
		if !strings.Contains(outAll, "stack-b-1") {
			t.Fatalf("expected unrelated stack branch with --all, got:\n%s", outAll)
		}
	})
}
