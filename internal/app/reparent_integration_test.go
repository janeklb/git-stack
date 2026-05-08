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

func TestReparentPreservesDirectChildSliceAfterRestack(t *testing.T) {
	t.Parallel()
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
		oldParentHead, err := gitOutput("rev-parse", "feat-two")
		if err != nil {
			t.Fatalf("resolve feat-two head before reparent: %v", err)
		}

		mustRunCLI(t, cli, []string{"new", "feat-three"})
		mustWriteFile(t, filepath.Join(repo, "feature3.txt"), "three\n")
		mustGit(t, repo, "add", "feature3.txt")
		mustGit(t, repo, "commit", "-m", "feat three")

		mustRunCLI(t, cli, []string{"reparent", "feat-two", "--onto", "main"})

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after reparent: %v", err)
		}
		if got := state.Branches["feat-three"].PendingRebaseBase; got != strings.TrimSpace(oldParentHead) {
			t.Fatalf("expected feat-three pending rebase base %q after reparent, got %q", strings.TrimSpace(oldParentHead), got)
		}

		mustRunCLI(t, cli, []string{"restack"})

		remaining, err := gitOutput("log", "--format=%s", "feat-two..feat-three")
		if err != nil {
			t.Fatalf("inspect feat-three commits after restack: %v", err)
		}
		if trimmed := strings.TrimSpace(remaining); trimmed != "feat three" {
			t.Fatalf("expected only feat-three commit above feat-two after restack, got:\n%s", trimmed)
		}

		state, err = loadState(repo)
		if err != nil {
			t.Fatalf("load state after restack: %v", err)
		}
		if got := state.Branches["feat-three"].PendingRebaseBase; got != "" {
			t.Fatalf("expected feat-three pending rebase base cleared after restack, got %q", got)
		}
	})
}

func TestReparentClearsPendingChildBaseAfterRestackContinue(t *testing.T) {
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
		mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
		mustGit(t, repo, "add", "two.txt")
		mustGit(t, repo, "commit", "-m", "feat two")

		mustRunCLI(t, cli, []string{"new", "feat-three"})
		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "three\n")
		mustGit(t, repo, "add", "conflict.txt")
		mustGit(t, repo, "commit", "-m", "feat three")

		mustRunCLI(t, cli, []string{"reparent", "feat-two", "--onto", "main"})

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after reparent: %v", err)
		}
		if got := state.Branches["feat-three"].PendingRebaseBase; got == "" {
			t.Fatal("expected feat-three pending rebase base after reparent")
		}

		mustGit(t, repo, "switch", "feat-three")
		out, code := runCLIAndCapture(t, cli, []string{"restack"})
		if code == 0 {
			t.Fatalf("expected restack conflict, output:\n%s", out)
		}
		if !strings.Contains(out, "stopped for conflicts") {
			t.Fatalf("expected conflict guidance, got:\n%s", out)
		}

		mustWriteFile(t, filepath.Join(repo, "conflict.txt"), "three\n")
		mustGit(t, repo, "add", "conflict.txt")

		out, code = runCLIAndCapture(t, cli, []string{"restack", "--continue"})
		if code != 0 {
			t.Fatalf("expected restack continue to succeed, exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "restack completed") {
			t.Fatalf("expected restack completion message, got:\n%s", out)
		}

		state, err = loadState(repo)
		if err != nil {
			t.Fatalf("load state after continue: %v", err)
		}
		if got := state.Branches["feat-three"].PendingRebaseBase; got != "" {
			t.Fatalf("expected feat-three pending rebase base cleared after continue, got %q", got)
		}
	})
}
