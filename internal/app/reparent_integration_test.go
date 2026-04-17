package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReparentChangesParentInState(t *testing.T) {
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

	mustRunCLIInRepo(t, repo, []string{"reparent", "--onto", "main", "feat-two"})

	state := readStateFile(t, repo)
	if got := state.Branches["feat-two"].Parent; got != "main" {
		t.Fatalf("expected feat-two parent main after reparent, got %q", got)
	}
	if got := state.Branches["feat-two"].LineageParent; got != "main" {
		t.Fatalf("expected feat-two lineage parent main after default reparent, got %q", got)
	}

	mustGit(t, repo, "merge-base", "--is-ancestor", "main", "feat-two")
}

func TestReparentPreserveLineageKeepsExistingLineageParent(t *testing.T) {
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

	mustRunCLIInRepo(t, repo, []string{"reparent", "--onto", "main", "--preserve-lineage", "feat-two"})

	state := readStateFile(t, repo)
	if got := state.Branches["feat-two"].Parent; got != "main" {
		t.Fatalf("expected feat-two parent main after reparent, got %q", got)
	}
	if got := state.Branches["feat-two"].LineageParent; got != "feat-one" {
		t.Fatalf("expected feat-two lineage parent feat-one with preserve flag, got %q", got)
	}

	mustGit(t, repo, "merge-base", "--is-ancestor", "main", "feat-two")
}

func TestReparentWithoutInitializedStateFails(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustGit(t, repo, "switch", "-c", "feat-one")
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")

	mustGit(t, repo, "switch", "-c", "feat-two")
	mustWriteFile(t, filepath.Join(repo, "feature2.txt"), "two\n")
	mustGit(t, repo, "add", "feature2.txt")
	mustGit(t, repo, "commit", "-m", "feat two")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"reparent", "--onto", "main", "feat-two"})
	if code == 0 {
		t.Fatalf("expected reparent to fail without initialized state, output:\n%s", out)
	}
	if !strings.Contains(out, "reparent requires initialized stack state") {
		t.Fatalf("expected initialized state error, got:\n%s", out)
	}
}

func TestReparentDefaultsTargetToCurrentBranch(t *testing.T) {
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

	mustRunCLIInRepo(t, repo, []string{"reparent", "--onto", "main"})

	state := readStateFile(t, repo)
	if got := state.Branches["feat-two"].Parent; got != "main" {
		t.Fatalf("expected current branch feat-two parent main after reparent, got %q", got)
	}
	if got := currentBranchInRepo(t, repo); got != "feat-two" {
		t.Fatalf("expected to remain on feat-two after reparent, got %q", got)
	}
}
