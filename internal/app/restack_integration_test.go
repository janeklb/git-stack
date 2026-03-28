package app

import (
	"path/filepath"
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
