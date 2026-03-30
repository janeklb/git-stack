package app

import (
	"bytes"
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
	Parent        string `json:"parent"`
	LineageParent string `json:"lineageParent,omitempty"`
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	origin := filepath.Join(base, "origin.git")

	mustGit(t, base, "init", "-b", "main", repo)
	mustGit(t, repo, "config", "user.name", "Stack Test")
	mustGit(t, repo, "config", "user.email", "stack-test@example.com")

	mustWriteFile(t, filepath.Join(repo, "README.md"), "# test\n")
	mustGit(t, repo, "add", "README.md")
	mustGit(t, repo, "commit", "-m", "initial")

	mustGit(t, repo, "init", "--bare", "--initial-branch=main", origin)
	mustGit(t, repo, "remote", "add", "origin", origin)
	mustConfigureOriginTracking(t, repo, "main")

	return repo
}

func mustConfigureOriginTracking(t *testing.T, repo, trunk string) {
	t.Helper()
	mustGit(t, repo, "push", "-u", "origin", trunk)
	mustGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+trunk)
}

func newBareOrigin(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	mustGit(t, base, "init", "--bare", "--initial-branch=main", origin)
	return origin
}

func mustPointRepoOriginAndTrack(t *testing.T, repo, origin, trunk string) {
	t.Helper()
	mustGit(t, repo, "remote", "set-url", "origin", origin)
	mustConfigureOriginTracking(t, repo, trunk)
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

func setStateLineageParent(t *testing.T, repo, branch, parent string) {
	t.Helper()
	state := readStateFile(t, repo)
	entry := state.Branches[branch]
	entry.LineageParent = parent
	state.Branches[branch] = entry
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "stack", "state.json"), data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func runCLIAndCapture(t *testing.T, cli *App, args []string) (string, int) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	os.Stderr = w

	code := cli.Run(args, "stack")

	_ = w.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()

	return buf.String(), code
}

func runCLIWithInputAndCapture(t *testing.T, cli *App, args []string, input string) (string, int) {
	t.Helper()

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if _, err := stdinWriter.WriteString(input); err != nil {
		t.Fatalf("write stdin input: %v", err)
	}
	_ = stdinWriter.Close()

	origStdin := os.Stdin
	os.Stdin = stdinReader
	defer func() {
		os.Stdin = origStdin
		_ = stdinReader.Close()
	}()

	return runCLIAndCapture(t, cli, args)
}
