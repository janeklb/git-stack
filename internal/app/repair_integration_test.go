package app

import (
	"path/filepath"
	"testing"
)

func TestRepairRebuildsParentRelationships(t *testing.T) {
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

		corruptStateParent(t, repo, "feat-two", "main")
		state := readStateFile(t, repo)
		if got := state.Branches["feat-two"].Parent; got != "main" {
			t.Fatalf("expected corrupted parent main, got %q", got)
		}

		mustRunCLI(t, cli, []string{"repair"})

		repaired := readStateFile(t, repo)
		if got := repaired.Branches["feat-two"].Parent; got != "feat-one" {
			t.Fatalf("expected repaired parent feat-one, got %q", got)
		}
	})
}
