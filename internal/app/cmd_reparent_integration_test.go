package app

import (
	"path/filepath"
	"testing"
)

func TestReparentChangesParentInState(t *testing.T) {
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

		mustRunCLI(t, cli, []string{"reparent", "--parent", "main", "feat-two"})

		state := readStateFile(t, repo)
		if got := state.Branches["feat-two"].Parent; got != "main" {
			t.Fatalf("expected feat-two parent main after reparent, got %q", got)
		}

		mustGit(t, repo, "merge-base", "--is-ancestor", "main", "feat-two")
	})
}
