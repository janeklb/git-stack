package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLICommandsUseInvocationRepository(t *testing.T) {
	t.Parallel()
	for _, branch := range []string{"first", "second"} {
		branch := branch
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			repo := newTestRepo(t)
			nested := filepath.Join(repo, "nested dir")
			if err := os.Mkdir(nested, 0o755); err != nil {
				t.Fatal(err)
			}
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			beforeEnv := os.Environ()
			var stdout, stderr bytes.Buffer
			input := strings.NewReader("")
			a := NewWithIO(input, &stdout, &stderr)
			a.dir = nested
			a.env = mergeEnvironment(os.Environ(), []string{"STACK_INVOCATION=" + branch})
			discoveries, creates, bodyEdits := 0, 0, 0
			a.runner = func(name string, args []string, opts commandRunOptions) (commandRunResult, error) {
				if opts.stdout != &stdout || opts.stderr != &stderr || !filepath.IsAbs(opts.dir) || !containsEnv(opts.baseEnv, "STACK_INVOCATION="+branch) {
					t.Errorf("lost invocation context: %s %v: %+v", name, args, opts)
				}
				if reflect.DeepEqual(args, []string{"rev-parse", "--path-format=absolute", "--show-toplevel"}) {
					discoveries++
				}
				if opts.streamOutput && opts.stdin != input {
					t.Errorf("streamed command lost invocation stdin: %s %v", name, args)
				}
				if name == "gh" {
					pr := `{"number":1,"url":"https://example.invalid/1","state":"OPEN","baseRefName":"main"}`
					switch args[1] {
					case "list":
						if creates == 0 {
							return commandRunResult{stdout: "[]"}, nil
						}
						return commandRunResult{stdout: "[" + pr + "]"}, nil
					case "create":
						creates++
						return commandRunResult{stdout: "https://example.invalid/1"}, nil
					case "edit":
						bodyEdits++
					}
					return commandRunResult{stdout: pr}, nil
				}
				return runCommand(name, args, opts)
			}
			run := func(args ...string) {
				t.Helper()
				before := discoveries
				if code := a.Run(args, canonicalBinName); code != 0 {
					t.Fatalf("%v: exit %d, stderr=%s", args, code, stderr.String())
				}
				if discoveries != before+1 {
					t.Fatalf("%v discovered layout %d times, want once", args, discoveries-before)
				}
			}
			run("init")
			run("new", branch)
			mustGit(t, repo, "commit", "--allow-empty", "-m", branch)
			run("restack")
			run("state", "--no-color")
			run("check")
			run("submit")
			state, err := a.repo.metadataStore().loadState()
			if err != nil || state.Branches[branch] == nil || state.Branches[branch].PR == nil || creates != 1 || bodyEdits == 0 {
				t.Fatalf("workflow escaped invocation repository: state=%+v creates=%d bodyEdits=%d err=%v", state, creates, bodyEdits, err)
			}
			stdout.Reset()
			run("__complete", "new", "--parent", branch)
			if !strings.Contains(stdout.String(), branch) {
				t.Fatalf("completion ignored invocation repository: %q", stdout.String())
			}
			after, err := os.Getwd()
			if err != nil || after != cwd || !reflect.DeepEqual(beforeEnv, os.Environ()) {
				t.Fatalf("commands changed process directory/environment: %q -> %q, %v", cwd, after, err)
			}
		})
	}
}

func TestCLIUsesInjectedFailureEvidence(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	a := NewWithIO(strings.NewReader(""), io.Discard, &stderr)
	a.dir = newTestRepo(t)
	cause := errors.New("injected status failure")
	a.runner = func(name string, args []string, opts commandRunOptions) (commandRunResult, error) {
		if name == "gh" || reflect.DeepEqual(args, []string{"status", "--porcelain"}) {
			return commandRunResult{stderr: "status evidence", exitCode: 42}, cause
		}
		return runCommand(name, args, opts)
	}
	if code := a.Run([]string{"init"}, canonicalBinName); code != 1 || !strings.Contains(stderr.String(), "status evidence") {
		t.Fatalf("CLI bypassed injected failure: exit=%d stderr=%s", code, stderr.String())
	}
	for _, call := range []func() error{
		a.repo.ensureCleanWorktree,
		func() error { _, err := a.repo.View(1); return err },
		func() error { return a.repo.ghRun("pr", "edit", "1") },
	} {
		err := call()
		var commandErr *commandError
		if !errors.Is(err, cause) || !errors.As(err, &commandErr) || commandErr.result.exitCode != 42 {
			t.Fatalf("command lost failure evidence: %v", err)
		}
	}
}

func TestEditorUsesInvocationContextWithAndWithoutRepository(t *testing.T) {
	t.Parallel()
	for _, inRepo := range []bool{false, true} {
		dir := t.TempDir()
		if inRepo {
			dir = newTestRepo(t)
		}
		var stdout, stderr bytes.Buffer
		input := strings.NewReader("editor input")
		a := NewWithIO(input, &stdout, &stderr)
		a.dir = dir
		a.env = mergeEnvironment(os.Environ(), []string{"STACK_EDITOR_CONTEXT=present"})
		editorCalls := 0
		a.runner = func(name string, args []string, opts commandRunOptions) (commandRunResult, error) {
			if reflect.DeepEqual(args, []string{"var", "GIT_EDITOR"}) {
				if opts.stdin != nil || opts.boxMode != commandBoxOnFailure {
					t.Error("editor discovery must be quiet and must not consume stdin")
				}
				return commandRunResult{stdout: "editor --wait\n"}, nil
			}
			if name == "sh" || name == "cmd" {
				editorCalls++
				gotDir, _ := filepath.EvalSymlinks(opts.dir)
				wantDir, _ := filepath.EvalSymlinks(dir)
				if gotDir != wantDir || !opts.interactive || opts.stdin != input || opts.stdout != &stdout || opts.stderr != &stderr || !containsEnv(opts.baseEnv, "STACK_EDITOR_CONTEXT=present") {
					t.Errorf("editor lost invocation context: %+v", opts)
				}
				return commandRunResult{}, nil
			}
			return runCommand(name, args, opts)
		}
		if inRepo {
			if _, err := a.discoverRepository(); err != nil {
				t.Fatal(err)
			}
		}
		runner, opts := a.editorExecution()
		if err := editFileInGitEditor(filepath.Join(dir, "template.md"), runner, opts); err != nil || editorCalls != 1 {
			t.Fatalf("inRepo=%v: calls=%d err=%v", inRepo, editorCalls, err)
		}
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
