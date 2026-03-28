package app

import (
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
