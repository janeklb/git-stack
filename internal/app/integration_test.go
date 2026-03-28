package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type testState struct {
	Trunk    string                         `json:"trunk"`
	Branches map[string]testBranchReference `json:"branches"`
}

type testBranchReference struct {
	Parent string `json:"parent"`
}

func TestInitAndNewBuildsStack(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})

		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")

		mustRunCLI(t, cli, []string{"new", "feat-two"})

		state := readStateFile(t, repo)
		if state.Trunk != "main" {
			t.Fatalf("expected trunk main, got %q", state.Trunk)
		}
		if got := state.Branches["feat-one"].Parent; got != "main" {
			t.Fatalf("expected feat-one parent main, got %q", got)
		}
		if got := state.Branches["feat-two"].Parent; got != "feat-one" {
			t.Fatalf("expected feat-two parent feat-one, got %q", got)
		}
	})
}

func TestRestackRebasesChildrenOntoUpdatedTrunk(t *testing.T) {
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

		mustGit(t, repo, "switch", "main")
		mustWriteFile(t, filepath.Join(repo, "base.txt"), "base\n")
		mustGit(t, repo, "add", "base.txt")
		mustGit(t, repo, "commit", "-m", "base update")

		mustGit(t, repo, "switch", "feat-two")
		mustRunCLI(t, cli, []string{"restack"})

		mustGit(t, repo, "merge-base", "--is-ancestor", "main", "feat-one")
		mustGit(t, repo, "merge-base", "--is-ancestor", "feat-one", "feat-two")
	})
}

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

func newTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	mustGit(t, repo, "init", "-b", "main")
	mustGit(t, repo, "config", "user.name", "Stack Test")
	mustGit(t, repo, "config", "user.email", "stack-test@example.com")

	mustWriteFile(t, filepath.Join(repo, "README.md"), "# test\n")
	mustGit(t, repo, "add", "README.md")
	mustGit(t, repo, "commit", "-m", "initial")

	return repo
}

func withRepoCwd(t *testing.T, repo string, fn func()) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir to repo: %v", err)
	}
	defer func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("chdir back: %v", err)
		}
	}()
	fn()
}

func mustRunCLI(t *testing.T, cli *App, args []string) {
	t.Helper()
	if code := cli.Run(args, "stack"); code != 0 {
		t.Fatalf("cli failed: stack %s (exit=%d)", strings.Join(args, " "), code)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func mustWriteFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func readStateFile(t *testing.T, repo string) testState {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, ".git", "stack", "state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state testState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	if state.Branches == nil {
		state.Branches = map[string]testBranchReference{}
	}
	return state
}

func corruptStateParent(t *testing.T, repo, branch, parent string) {
	t.Helper()
	state := readStateFile(t, repo)
	entry := state.Branches[branch]
	entry.Parent = parent
	state.Branches[branch] = entry
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal corrupted state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "stack", "state.json"), data, 0o600); err != nil {
		t.Fatalf("write corrupted state: %v", err)
	}
}
