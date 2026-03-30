package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRestackRebasesChildrenOntoUpdatedTrunk(t *testing.T) {
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

		mustGit(t, repo, "switch", "main")
		mustWriteFile(t, filepath.Join(repo, "base.txt"), "base\n")
		mustGit(t, repo, "add", "base.txt")
		mustGit(t, repo, "commit", "-m", "base update")

		mustGit(t, repo, "switch", "feat-two")
		mustRunCLI(t, cli, []string{"restack"})

		mustGit(t, repo, "merge-base", "--is-ancestor", "main", "feat-one")
		mustGit(t, repo, "merge-base", "--is-ancestor", "feat-one", "feat-two")
	})
}

func TestRestackContinueAndAbortWithoutOperationFail(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		if out, code := runCLIAndCapture(t, cli, []string{"restack", "--continue"}); code == 0 {
			t.Fatalf("expected restack --continue to fail without operation, output:\n%s", out)
		}
		if out, code := runCLIAndCapture(t, cli, []string{"restack", "--abort"}); code == 0 {
			t.Fatalf("expected restack --abort to fail without operation, output:\n%s", out)
		}
	})
}

func TestRestackWithoutInitializedStateUsesInferredStack(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustGit(t, repo, "switch", "-c", "feat-one")
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustGit(t, repo, "switch", "-c", "feat-two")
		mustWriteFile(t, filepath.Join(repo, "feature2.txt"), "two\n")
		mustGit(t, repo, "add", "feature2.txt")
		mustGit(t, repo, "commit", "-m", "feat two")

		mustGit(t, repo, "switch", "main")
		mustWriteFile(t, filepath.Join(repo, "base.txt"), "base\n")
		mustGit(t, repo, "add", "base.txt")
		mustGit(t, repo, "commit", "-m", "base update")

		mustGit(t, repo, "switch", "feat-two")
		mustRunCLI(t, cli, []string{"restack"})

		mustGit(t, repo, "merge-base", "--is-ancestor", "main", "feat-one")
		mustGit(t, repo, "merge-base", "--is-ancestor", "feat-one", "feat-two")
	})
}

func TestRestackRecoversFromManualRebaseContinue(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		t.Setenv("GIT_EDITOR", "true")

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "base\n")
		mustGit(t, repo, "add", "conflict.txt")
		mustGit(t, repo, "commit", "-m", "add conflict file")

		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "one\n")
		mustGit(t, repo, "add", "conflict.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two"})
		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "two\n")
		mustGit(t, repo, "add", "conflict.txt")
		mustGit(t, repo, "commit", "-m", "feat two")

		mustGit(t, repo, "switch", "feat-one")
		mustGit(t, repo, "reset", "--hard", "main")
		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "uno\n")
		mustGit(t, repo, "add", "conflict.txt")
		mustGit(t, repo, "commit", "-m", "feat one rewritten")

		mustGit(t, repo, "switch", "feat-two")
		out, code := runCLIAndCapture(t, cli, []string{"restack"})
		if code == 0 {
			t.Fatalf("expected restack conflict, output:\n%s", out)
		}
		if !strings.Contains(out, "stopped for conflicts") {
			t.Fatalf("expected conflict guidance, got:\n%s", out)
		}

		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "two\n")
		mustGit(t, repo, "add", "conflict.txt")
		mustGit(t, repo, "rebase", "--continue")

		out, code = runCLIAndCapture(t, cli, []string{"restack", "--continue"})
		if code != 0 {
			t.Fatalf("expected continue to recover after manual rebase continue, exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "restack completed") {
			t.Fatalf("expected restack completion message, got:\n%s", out)
		}

		op, err := loadOperation(repo)
		if err != nil {
			t.Fatalf("load operation after recovery: %v", err)
		}
		if op != nil {
			t.Fatalf("expected operation to be cleared after recovery")
		}
	})
}
