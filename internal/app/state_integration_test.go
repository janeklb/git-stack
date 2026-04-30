package app

import (
	"os"
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
	if !strings.Contains(out, "[local-only, needs-restack, drifted-from-ancestor]") {
		t.Fatalf("expected drift marker in state output, got:\n%s", out)
	}
}

func TestStateOmitsHistoricalPRStatusForSyncedPRBranch(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	state, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main", Updated: true}
	if err := saveState(repo, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-one (PR #1 https://example.invalid/pr/1)") {
		t.Fatalf("expected PR branch to render without synthetic status label, got:\n%s", out)
	}
	if strings.Contains(out, "[submitted]") || strings.Contains(out, "[updated]") {
		t.Fatalf("expected synced PR branch to omit historical submit labels, got:\n%s", out)
	}
}

func TestStateShowsNeedsRestackAndNeedsSubmitForOutOfSyncPRBranch(t *testing.T) {
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
	mustGit(t, repo, "push", "-u", "origin", "feat-one")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	state, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
	state.Branches["feat-two"].PR = &PRMeta{Number: 2, URL: "https://example.invalid/pr/2", Base: "feat-one"}
	if err := saveState(repo, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	mustGit(t, repo, "switch", "feat-one")
	mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\nparent moved\n")
	mustGit(t, repo, "add", "feature1.txt")
	mustGit(t, repo, "commit", "-m", "feat one moved")

	mustGit(t, repo, "switch", "feat-two")
	mustWriteFile(t, filepath.Join(repo, "feature2.txt"), "two\nchild moved\n")
	mustGit(t, repo, "add", "feature2.txt")
	mustGit(t, repo, "commit", "-m", "feat two moved")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-two [needs-restack, needs-submit] (PR #2 https://example.invalid/pr/2)") {
		t.Fatalf("expected state to show actionable sync labels for out-of-sync PR branch, got:\n%s", out)
	}
	if strings.Contains(out, "updated") {
		t.Fatalf("expected state to stop using updated label, got:\n%s", out)
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
	if !strings.Contains(out, "state: inferred local graph") {
		t.Fatalf("expected inferred-state label in output, got:\n%s", out)
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
	if strings.Contains(out, "WARN ") {
		t.Fatalf("expected inline invalid annotation without top warnings, got:\n%s", out)
	}
	if !strings.Contains(out, "ghost [local-only, invalid]") {
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
	state.Archived["feat-zero"] = testArchivedReference{Parent: "main"}
	state.Archived["feat-one"] = testArchivedReference{Parent: "feat-zero"}
	writeStateFile(t, repo, state)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "feat-zero [merged]") {
		t.Fatalf("expected oldest archived lineage line in state output, got:\n%s", out)
	}
	if !strings.Contains(out, "└─ feat-one [merged]") {
		t.Fatalf("expected newest archived lineage line in state output, got:\n%s", out)
	}
	if strings.Index(out, "feat-zero [merged]") > strings.Index(out, "└─ feat-one [merged]") {
		t.Fatalf("expected archived lineage oldest-to-newest, got:\n%s", out)
	}
	if strings.Index(out, "└─ feat-one [merged]") > strings.Index(out, "   └─ main (trunk)") {
		t.Fatalf("expected archived lineage before trunk, got:\n%s", out)
	}
	if !strings.Contains(out, "   └─ main (trunk)") {
		t.Fatalf("expected trunk indented beneath archived lineage, got:\n%s", out)
	}
	if !strings.Contains(out, "feat-two") {
		t.Fatalf("expected active descendant in state output, got:\n%s", out)
	}
}

func TestStateShowsUnrootedBranchAsSeparateStatusItems(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})

	state := readStateFile(t, repo)
	state.Branches["feat-one"] = testBranchReference{Parent: "ghost-parent", LineageParent: "ghost-parent"}
	writeStateFile(t, repo, state)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "? feat-one [unrooted, local-only, missing-parent=ghost-parent]") {
		t.Fatalf("expected unrooted output to use separate status items, got:\n%s", out)
	}
	if strings.Contains(out, "state=") {
		t.Fatalf("expected unrooted output without state= prefix, got:\n%s", out)
	}
}

func TestStateShowsMetadataMissingBranchExplicitly(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})

	state := readStateFile(t, repo)
	delete(state.Branches, "feat-one")
	writeStateFile(t, repo, state)

	statePath := filepath.Join(repo, ".git", "stack", "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	updated := strings.Replace(string(data), "\"branches\": {}", "\"branches\": {\n    \"feat-one\": null\n  }", 1)
	if err := os.WriteFile(statePath, []byte(updated), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "? feat-one [invalid, metadata-missing]") {
		t.Fatalf("expected metadata-missing branch in state output, got:\n%s", out)
	}
}

func TestStateShowsCyclesExplicitly(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-two"})

	state := readStateFile(t, repo)
	state.Branches["feat-one"] = testBranchReference{Parent: "feat-two", LineageParent: "main"}
	state.Branches["feat-two"] = testBranchReference{Parent: "feat-one", LineageParent: "feat-one"}
	writeStateFile(t, repo, state)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"state"})
	if code != 0 {
		t.Fatalf("state failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "? feat-one [cycle, local-only]") {
		t.Fatalf("expected cycle root in state output, got:\n%s", out)
	}
	if !strings.Contains(out, "└─ feat-two [cycle, local-only]") {
		t.Fatalf("expected cycle child in state output, got:\n%s", out)
	}
	if !strings.Contains(out, "└─ feat-one [cycle]") {
		t.Fatalf("expected cycle closure in state output, got:\n%s", out)
	}
}
