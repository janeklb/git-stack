package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckReportsParentNotAncestorAsError(t *testing.T) {
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
	mustGit(t, repo, "switch", "feat-two")
	mustGit(t, repo, "rebase", "--onto", "main", "feat-one")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"doctor"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for doctor with errors, got output:\n%s", out)
	}
	if !strings.Contains(out, "ERROR parent-not-ancestor branch=feat-two parent=feat-one") {
		t.Fatalf("expected parent-not-ancestor error in doctor output, got:\n%s", out)
	}
	if !strings.Contains(out, "errors") {
		t.Fatalf("expected summary in doctor output, got:\n%s", out)
	}
}

func TestCheckReportsInfoForLocalUntrackedBranches(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustGit(t, repo, "switch", "-c", "scratch")
	mustWriteFile(t, filepath.Join(repo, "scratch.txt"), "scratch\n")
	mustGit(t, repo, "add", "scratch.txt")
	mustGit(t, repo, "commit", "-m", "scratch")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"doctor"})
	if code != 0 {
		t.Fatalf("expected zero exit for info-only doctor output, got exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "INFO missing-state-entry branch=scratch") {
		t.Fatalf("expected info line for local untracked branch, got:\n%s", out)
	}
	if !strings.Contains(out, "0 errors") {
		t.Fatalf("expected zero errors in summary, got:\n%s", out)
	}
}

func TestCheckErrorsWhenStateIsNotInitialized(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"doctor"})
	if code == 0 {
		t.Fatalf("expected non-zero exit when state is not initialized, got output:\n%s", out)
	}
	if !strings.Contains(out, "ERROR state-not-initialized") {
		t.Fatalf("expected state-not-initialized error in doctor output, got:\n%s", out)
	}
}
