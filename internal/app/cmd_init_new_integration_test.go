package app

import (
	"os"
	"path/filepath"
	"strings"
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

func TestNewWithoutInitBootstrapsState(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		out, code := runCLIAndCapture(t, cli, []string{"new", "doing-something"})
		if code != 0 {
			t.Fatalf("expected stack new to succeed without init, got code=%d output=%s", code, out)
		}
		if _, err := os.Stat(filepath.Join(repo, ".git", "stack", "state.json")); err != nil {
			t.Fatalf("expected persisted stack state, got err=%v", err)
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

		if _, err := os.Stat(filepath.Join(repo, ".git", "stack", "state.json")); err != nil {
			t.Fatalf("expected persisted stack state before init, got err=%v", err)
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

func TestNewOnUntrackedCurrentBranchAutoTracksAndStacksFromIt(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})

		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustGit(t, repo, "switch", "-c", "manual-child")
		mustRunCLI(t, cli, []string{"new", "auto-child"})

		state := readStateFile(t, repo)
		if got := state.Branches["auto-child"].Parent; got != "manual-child" {
			t.Fatalf("expected auto-child parent manual-child, got %q", got)
		}
		if _, ok := state.Branches["manual-child"]; !ok {
			t.Fatalf("expected manual-child to be auto-tracked")
		}
	})
}

func TestNewInEmptyRepositoryShowsGuidance(t *testing.T) {
	repo := t.TempDir()
	mustGit(t, repo, "init", "-b", "main")

	withRepoCwd(t, repo, func() {
		cli := New()
		out, code := runCLIAndCapture(t, cli, []string{"new", "first-change"})
		if code == 0 {
			t.Fatalf("expected stack new to fail in empty repository")
		}
		if !strings.Contains(out, "repository has no commits yet") {
			t.Fatalf("expected no-commit guidance, got:\n%s", out)
		}
		if !strings.Contains(out, "git commit --allow-empty") {
			t.Fatalf("expected allow-empty suggestion, got:\n%s", out)
		}
	})
}
