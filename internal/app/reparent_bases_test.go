package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordChildRestackBasesPreservesOriginalHistory(t *testing.T) {
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"parent":     {Parent: "main"},
		"child":      {Parent: "parent"},
		"sibling":    {Parent: "parent", PendingRebaseBase: "original"},
		"grandchild": {Parent: "child"},
	}}
	recordChildRestackBases(state, "parent", "first")
	recordChildRestackBases(state, "parent", "second")
	if got := state.Branches["child"].PendingRebaseBase; got != "first" {
		t.Fatalf("child base = %q, want first", got)
	}
	if got := state.Branches["sibling"].PendingRebaseBase; got != "original" {
		t.Fatalf("sibling base = %q, want original", got)
	}
	if got := state.Branches["grandchild"].PendingRebaseBase; got != "" {
		t.Fatalf("unrewritten child's descendant base = %q", got)
	}
	clearPendingRebaseBase(state, "child")
	recordChildRestackBases(state, "parent", "third")
	if got := state.Branches["child"].PendingRebaseBase; got != "third" {
		t.Fatalf("completed child's new base = %q, want third", got)
	}
}

func TestMergedRebaseBasesKeepsPendingBaseOverPlanOverride(t *testing.T) {
	state := &State{Branches: map[string]*BranchRef{
		"child": {PendingRebaseBase: "original"},
		"other": {},
	}}
	bases := mergedRebaseBases(state, []string{"child", "other"}, map[string]string{
		"child": "intermediate", "other": "explicit",
	})
	if bases["child"] != "original" || bases["other"] != "explicit" {
		t.Fatalf("unexpected bases: %v", bases)
	}
}

func TestRepeatedReparentPreservesDescendantSlices(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	readState := func() *State {
		t.Helper()
		state, err := loadState(repo)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	addBranch := func(branch, parent string) {
		t.Helper()
		mustRunCLIInRepo(t, repo, []string{"new", branch, "--parent", parent})
		mustWriteFile(t, filepath.Join(repo, branch+".txt"), branch+"\n")
		mustGit(t, repo, "add", branch+".txt")
		mustGit(t, repo, "commit", "-m", branch)
	}
	addBranch("other", "main")
	addBranch("ancestor", "main")
	addBranch("parent", "ancestor")
	addBranch("child", "parent")
	addBranch("grandchild", "child")
	addBranch("sibling", "parent")
	original := strings.TrimSpace(mustGitOutput(t, repo, "rev-parse", "parent"))
	for _, onto := range []string{"main", "other"} {
		mustRunCLIInRepo(t, repo, []string{"reparent", "parent", "--onto", onto})
		state := readState()
		for _, child := range []string{"child", "sibling"} {
			if got := state.Branches[child].PendingRebaseBase; got != original {
				t.Fatalf("%s base after reparent onto %s = %q, want %q", child, onto, got, original)
			}
		}
	}
	mustRunCLIInRepo(t, repo, []string{"restack"})
	for branch, parent := range map[string]string{"child": "parent", "sibling": "parent", "grandchild": "child"} {
		if got := strings.TrimSpace(mustGitOutput(t, repo, "log", "--format=%s", parent+".."+branch)); got != branch {
			t.Fatalf("%s slice = %q, want only its own commit", branch, got)
		}
		if got := strings.TrimSpace(mustGitOutput(t, repo, "ls-tree", "--name-only", branch, "ancestor.txt")); got != "" {
			t.Fatalf("%s regained removed ancestor file", branch)
		}
		if got := readState().Branches[branch].PendingRebaseBase; got != "" {
			t.Fatalf("%s completed with pending base %q", branch, got)
		}
	}
	newBase := strings.TrimSpace(mustGitOutput(t, repo, "rev-parse", "parent"))
	mustRunCLIInRepo(t, repo, []string{"reparent", "parent", "--onto", "main"})
	if got := readState().Branches["child"].PendingRebaseBase; got != newBase {
		t.Fatalf("next rewrite base = %q, want fresh %q", got, newBase)
	}
	mustRunCLIInRepo(t, repo, []string{"restack"})
}

func TestRestackConflictAndAbortRetainOriginalChildBase(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	commitFile := func(name, contents string) {
		t.Helper()
		mustWriteFile(t, filepath.Join(repo, name), contents)
		mustGit(t, repo, "add", name)
		mustGit(t, repo, "commit", "-m", contents)
	}
	commitFile("conflict.txt", "base\n")
	mustRunCLIInRepo(t, repo, []string{"new", "ancestor"})
	commitFile("conflict.txt", "ancestor\n")
	mustRunCLIInRepo(t, repo, []string{"new", "parent"})
	commitFile("parent.txt", "parent\n")
	base := strings.TrimSpace(mustGitOutput(t, repo, "rev-parse", "parent"))
	mustRunCLIInRepo(t, repo, []string{"new", "child"})
	commitFile("conflict.txt", "child\n")
	head := strings.TrimSpace(mustGitOutput(t, repo, "rev-parse", "child"))
	mustRunCLIInRepo(t, repo, []string{"reparent", "parent", "--onto", "main"})
	if out, code := runCLIInRepoAndCapture(t, repo, []string{"restack"}); code == 0 {
		t.Fatalf("expected conflict, got: %s", out)
	}
	assertPending := func() {
		t.Helper()
		state, err := loadState(repo)
		if err != nil {
			t.Fatal(err)
		}
		if got := state.Branches["child"].PendingRebaseBase; got != base {
			t.Fatalf("pending base = %q, want original %q", got, base)
		}
	}
	assertPending()
	mustRunCLIInRepo(t, repo, []string{"restack", "--abort"})
	assertPending()
	if got := strings.TrimSpace(mustGitOutput(t, repo, "rev-parse", "child")); got != head {
		t.Fatalf("aborted child head = %q, want %q", got, head)
	}
}
