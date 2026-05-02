package app

import (
	"path/filepath"
	"testing"
)

func TestNewAdoptTracksCurrentExistingBranchAfterStateExists(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustGit(t, repo, "switch", "-c", "manual-branch")
	mustWriteFile(t, filepath.Join(repo, "manual.txt"), "manual\n")
	mustGit(t, repo, "add", "manual.txt")
	mustGit(t, repo, "commit", "-m", "manual branch")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"new", "--adopt"})
	if code != 0 {
		t.Fatalf("new --adopt failed: exit=%d\n%s", code, out)
	}
	state := readStateFile(t, repo)
	meta, ok := state.Branches["manual-branch"]
	if !ok {
		t.Fatal("expected manual-branch to be tracked after adopt")
	}
	if meta.Parent != "main" {
		t.Fatalf("expected adopted branch parent main, got %q", meta.Parent)
	}
}

func TestNewAdoptAllowsExplicitParentOverride(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "base"})
	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "switch", "-c", "manual-branch")
	mustWriteFile(t, filepath.Join(repo, "manual.txt"), "manual\n")
	mustGit(t, repo, "add", "manual.txt")
	mustGit(t, repo, "commit", "-m", "manual branch")

	out, code := runCLIInRepoAndCapture(t, repo, []string{"new", "--adopt", "--parent", "base"})
	if code != 0 {
		t.Fatalf("new --adopt --parent failed: exit=%d\n%s", code, out)
	}
	state := readStateFile(t, repo)
	meta, ok := state.Branches["manual-branch"]
	if !ok {
		t.Fatal("expected manual-branch to be tracked after adopt")
	}
	if meta.Parent != "base" || meta.LineageParent != "base" {
		t.Fatalf("expected adopted branch parent/lineage to be base, got %+v", meta)
	}
}

func TestNewPreservesCaseInExplicitBranchName(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"new", "sliceA"})
	if code != 0 {
		t.Fatalf("new sliceA failed: exit=%d\n%s", code, out)
	}
	if !branchExistsInRepo(repo, "sliceA") {
		t.Fatal("expected branch sliceA to be created without case normalization")
	}
}
