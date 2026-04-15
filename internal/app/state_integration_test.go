package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStateShowsDriftWhenParentIsNotAncestor(t *testing.T) {
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

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state", "--drift"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-two") {
		t.Fatalf("expected state to include feat-two, got:\n%s", out)
	}
	if !strings.Contains(out, "[drift: parent-not-ancestor]") {
		t.Fatalf("expected drift marker in state output, got:\n%s", out)
	}
}

func TestStateWorksWithoutInitializedState(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustGit(t, repo, "switch", "-c", "feat-one")
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state", "--all"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "main (trunk)") {
		t.Fatalf("expected trunk in state output, got:\n%s", out)
	}
	if !strings.Contains(out, "feat-one") {
		t.Fatalf("expected inferred branch in state output, got:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected plain output without ANSI escapes in non-TTY, got:\n%s", out)
	}
}

func TestStateShowsStatelessStackCreatedByStackNew(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two"})

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-one") || !strings.Contains(out, "feat-two") {
		t.Fatalf("expected both inferred branches in state output, got:\n%s", out)
	}
	if !strings.Contains(out, "[local-only]") {
		t.Fatalf("expected local-only state marker in state output, got:\n%s", out)
	}
}

func TestStateWarnsWhenTrackedBranchIsMissingLocally(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})

	state := readStateFile(t, repo)
	state.Branches["ghost"] = testBranchReference{Parent: "feat-one", LineageParent: "feat-one"}
	writeStateFile(t, repo, state)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "WARN tracked branch missing locally: ghost") {
		t.Fatalf("expected missing-local warning in state output, got:\n%s", out)
	}
	if !strings.Contains(out, "ghost [local-only] [invalid: missing-local]") {
		t.Fatalf("expected missing-local annotation in state graph, got:\n%s", out)
	}
}

func TestStateShowsArchivedLineageParentsBeforeTrunk(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustRunCLIInRepo(t, repo, []string{"new", "feat-two"})
	mustGit(t, repo, "branch", "-D", "feat-one")

	state := readStateFile(t, repo)
	delete(state.Branches, "feat-one")
	state.Branches["feat-two"] = testBranchReference{Parent: "main", LineageParent: "feat-one"}
	state.Archived["feat-one"] = testArchivedReference{Parent: "main"}
	writeStateFile(t, repo, state)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	archivedLine := "feat-one (archived) -> main (trunk)"
	if !strings.Contains(out, archivedLine) {
		t.Fatalf("expected archived lineage line in state output, got:\n%s", out)
	}
	if strings.Index(out, archivedLine) > strings.Index(out, "main (trunk)") {
		t.Fatalf("expected archived lineage before trunk, got:\n%s", out)
	}
	if !strings.Contains(out, "feat-two") {
		t.Fatalf("expected active descendant in state output, got:\n%s", out)
	}
}
