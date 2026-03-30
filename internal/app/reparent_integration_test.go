package app

import (
	"path/filepath"
	"strings"
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

func TestReparentPreservesExistingLineageParent(t *testing.T) {
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

		setStateLineageParent(t, repo, "feat-two", "feat-one")

		mustRunCLI(t, cli, []string{"reparent", "--parent", "main", "feat-two"})

		state := readStateFile(t, repo)
		if got := state.Branches["feat-two"].Parent; got != "main" {
			t.Fatalf("expected feat-two parent main after reparent, got %q", got)
		}
		if got := state.Branches["feat-two"].LineageParent; got != "feat-one" {
			t.Fatalf("expected feat-two lineage parent preserved as feat-one, got %q", got)
		}
	})
}

func TestReparentRejectsSelfParent(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})

		out, code := runCLIAndCapture(t, cli, []string{"reparent", "--parent", "feat-one", "feat-one"})
		if code == 0 {
			t.Fatalf("expected reparent to fail for self parent")
		}
		if !strings.Contains(out, "branch cannot parent itself") {
			t.Fatalf("expected self-parent validation message, got:\n%s", out)
		}
	})
}

func TestReparentRejectsDescendantParent(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two", "--parent", "feat-one"})

		out, code := runCLIAndCapture(t, cli, []string{"reparent", "--parent", "feat-two", "feat-one"})
		if code == 0 {
			t.Fatalf("expected reparent to fail when new parent is a descendant")
		}
		if !strings.Contains(out, "parent cannot be a descendant") {
			t.Fatalf("expected descendant validation message, got:\n%s", out)
		}
	})
}
