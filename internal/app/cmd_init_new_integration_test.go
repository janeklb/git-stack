package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndNewBuildsStack(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})

		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two"})

		state := readStateFile(t, repo)
		if state.Trunk != "main" {
			t.Fatalf("expected trunk main, got %q", state.Trunk)
		}
		if got := state.Branches["feat-one"].Parent; got != "main" {
			t.Fatalf("expected feat-one parent main, got %q", got)
		}
		if got := state.Branches["feat-two"].Parent; got != "feat-one" {
			t.Fatalf("expected feat-two parent feat-one, got %q", got)
		}
	})
}

func TestNewWithoutInitIsStatelessByDefault(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		out, code := runCLIAndCapture(t, cli, []string{"new", "doing-something"})
		if code != 0 {
			t.Fatalf("expected stack new to succeed without init, got code=%d output=%s", code, out)
		}
		if _, err := os.Stat(filepath.Join(repo, ".git", "stack", "state.json")); !os.IsNotExist(err) {
			t.Fatalf("expected no persisted stack state, got err=%v", err)
		}
		mustGit(t, repo, "show-ref", "--verify", "--quiet", "refs/heads/doing-something")
	})
}

func TestInitAfterStatelessWorkPreservesInferredBranches(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two"})
		mustWriteFile(t, filepath.Join(repo, "feature2.txt"), "two\n")
		mustGit(t, repo, "add", "feature2.txt")
		mustGit(t, repo, "commit", "-m", "feat two")

		if _, err := os.Stat(filepath.Join(repo, ".git", "stack", "state.json")); !os.IsNotExist(err) {
			t.Fatalf("expected no persisted stack state before init, got err=%v", err)
		}

		mustRunCLI(t, cli, []string{"init"})

		state := readStateFile(t, repo)
		if got := state.Branches["feat-one"].Parent; got != "main" {
			t.Fatalf("expected feat-one parent main after init, got %q", got)
		}
		if got := state.Branches["feat-two"].Parent; got != "feat-one" {
			t.Fatalf("expected feat-two parent feat-one after init, got %q", got)
		}
	})
}
