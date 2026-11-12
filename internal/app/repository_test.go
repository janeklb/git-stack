package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRepositoryDiscoversGitLayouts(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	base := t.TempDir()
	linked := filepath.Join(base, "linked worktree")
	detached := filepath.Join(base, "detached\nworktree")
	separate := filepath.Join(base, "separate worktree")
	gitDir := filepath.Join(base, "separate git dir")
	clone := filepath.Join(base, "clone with spaces")
	nested := filepath.Join(repo, "sub dir")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "worktree", "add", "-b", "feature", linked)
	mustGit(t, repo, "worktree", "add", "--detach", detached)
	mustGit(t, base, "init", "--separate-git-dir", gitDir, separate)
	mustGit(t, base, "clone", repo, clone)
	for _, tc := range []struct {
		name, dir, root, git, common string
	}{
		{"ordinary", repo, repo, filepath.Join(repo, ".git"), filepath.Join(repo, ".git")},
		{"nested", nested, repo, filepath.Join(repo, ".git"), filepath.Join(repo, ".git")},
		{"spaces", clone, clone, filepath.Join(clone, ".git"), filepath.Join(clone, ".git")},
		{"linked", linked, linked, filepath.Join(repo, ".git", "worktrees", "linked-worktree"), filepath.Join(repo, ".git")},
		{"separate", separate, separate, gitDir, gitDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := discoverRepository(tc.dir, repositoryOptions{stdout: io.Discard, stderr: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []struct{ got, want string }{
				{r.worktreeRoot, tc.root}, {r.worktreeGitDir, tc.git}, {r.commonGitDir, tc.common},
			} {
				want, err := filepath.EvalSymlinks(path.want)
				if err != nil {
					t.Fatal(err)
				}
				if path.got != want {
					t.Fatalf("path = %q, want %q", path.got, want)
				}
			}
		})
	}
	r, err := discoverRepository(linked, repositoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	checkouts, err := r.worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(checkouts) != 3 {
		t.Fatalf("worktrees = %+v, want three", checkouts)
	}
	for _, tc := range []struct {
		path, branch string
		detached     bool
	}{
		{repo, "refs/heads/main", false}, {linked, "refs/heads/feature", false}, {detached, "", true},
	} {
		path, err := filepath.EvalSymlinks(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, checkout := range checkouts {
			if checkout.path == path {
				found = true
				if checkout.branch != tc.branch || checkout.detached != tc.detached {
					t.Fatalf("unexpected checkout: %+v", checkout)
				}
			}
		}
		if !found {
			t.Fatalf("no checkout for %q in %+v", path, checkouts)
		}
	}
}

func TestRepositoryInstancesOwnExecution(t *testing.T) {
	t.Parallel()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	beforeEnv := os.Environ()
	var wg sync.WaitGroup
	for _, marker := range []string{"first", "second"} {
		repo := newTestRepo(t)
		var stdout, stderr bytes.Buffer
		env := mergeEnvironment(os.Environ(), []string{"STACK_REPO_MARKER=" + marker, "GIT_EDITOR=base"})
		r, err := discoverRepository(repo, repositoryOptions{
			in: strings.NewReader(marker + " input\n"), stdout: &stdout, stderr: &stderr, env: env,
		})
		if err != nil {
			t.Fatal(err)
		}
		// The repository keeps its own snapshot, not the caller's mutable slice.
		for i := range env {
			env[i] = "UNUSED=changed"
		}
		wg.Add(1)
		go func(r *repository, marker string, stdout, stderr *bytes.Buffer) {
			defer wg.Done()
			alias := `!read line; printf '%s|%s|%s\n' "$STACK_REPO_MARKER" "$GIT_EDITOR" "$line"; printf 'stderr\n' >&2`
			result, err := r.runGit([]string{"-c", "alias.probe=" + alias, "probe"}, commandRunOptions{
				streamOutput: true, env: []string{"GIT_EDITOR=override"},
			})
			if err != nil {
				t.Error(err)
				return
			}
			want := marker + "|override|" + marker + " input\n"
			if stdout.String() != want || result.stdout != want || stderr.String() != "stderr\n" || result.stderr != "stderr\n" {
				t.Errorf("instance %s: result=%+v stdout=%q stderr=%q", marker, result, stdout, stderr)
			}
			root, err := r.gitOutputTrimmed("rev-parse", "--show-toplevel")
			if err != nil || root != r.worktreeRoot {
				t.Errorf("instance %s ran in wrong directory: %q, %v", marker, root, err)
			}
		}(r, marker, &stdout, &stderr)
	}
	wg.Wait()
	after, err := os.Getwd()
	if err != nil || cwd != after || !reflect.DeepEqual(beforeEnv, os.Environ()) {
		t.Fatalf("execution changed process directory/environment: %q -> %q, %v", cwd, after, err)
	}
}

func TestRepositoryCommandErrorsPreserveEvidence(t *testing.T) {
	t.Parallel()
	r, err := discoverRepository(newTestRepo(t), repositoryOptions{stdout: io.Discard, stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.runGit([]string{"-c", "alias.fail=!printf out; printf err >&2; exit 23", "fail"}, commandRunOptions{})
	var commandErr *commandError
	var exitErr *exec.ExitError
	if !errors.As(err, &commandErr) || !errors.As(err, &exitErr) {
		t.Fatalf("lost command error or cause: %v", err)
	}
	if result.exitCode != 23 || commandErr.result.exitCode != 23 || exitErr.ExitCode() != 23 || result.stdout != "out" || result.stderr != "err" {
		t.Fatalf("lost command evidence: %+v, %v", result, err)
	}
	if !strings.Contains(err.Error(), "out") || !strings.Contains(err.Error(), "err") {
		t.Fatalf("error omitted captured output: %v", err)
	}
}

func TestRepositoryRunnerInjectsFailuresAtProductionSeam(t *testing.T) {
	t.Parallel()
	cause := errors.New("injected execution failure")
	runner := func(name string, args []string, opts commandRunOptions) (commandRunResult, error) {
		if name != "git" || !filepath.IsAbs(opts.dir) {
			t.Fatalf("invalid execution request: %q %v in %q", name, args, opts.dir)
		}
		if reflect.DeepEqual(args, []string{"worktree", "list", "--porcelain", "-z"}) {
			return commandRunResult{stderr: "injected stderr", exitCode: 42}, cause
		}
		return runCommand(name, args, opts)
	}
	r, err := discoverRepository(newTestRepo(t), repositoryOptions{runner: runner, stdout: io.Discard, stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.worktrees()
	var commandErr *commandError
	if !errors.Is(err, cause) || !errors.As(err, &commandErr) || commandErr.result.exitCode != 42 || commandErr.result.stderr != "injected stderr" {
		t.Fatalf("lost injected failure evidence: %v", err)
	}
}

func TestRepositoryRejectsBareWorkingTrees(t *testing.T) {
	t.Parallel()
	_, err := discoverRepository(newBareOrigin(t), repositoryOptions{stdout: io.Discard, stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "bare repositories") {
		t.Fatalf("expected bare repository refusal, got %v", err)
	}
}

func TestRepositoryUsesItsOwnExecutableSearchPath(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'instance git\\n'; else exec \"$STACK_REAL_GIT\" \"$@\"; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := mergeEnvironment(os.Environ(), []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "STACK_REAL_GIT=" + git,
	})
	r, err := discoverRepository(newTestRepo(t), repositoryOptions{env: env, stdout: io.Discard, stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	version, err := r.gitOutputTrimmed("--version")
	if err != nil || version != "instance git" {
		t.Fatalf("instance PATH was ignored: %q, %v", version, err)
	}
}

func TestRepositoryStartupFailureIsNotASuccessfulExit(t *testing.T) {
	t.Parallel()
	r, err := discoverRepository(newTestRepo(t), repositoryOptions{stdout: io.Discard, stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range []bool{false, true} {
		result, err := r.runGit([]string{"--version"}, commandRunOptions{streamOutput: stream, env: []string{"PATH=" + t.TempDir()}})
		if !errors.Is(err, exec.ErrNotFound) || result.exitCode != -1 {
			t.Fatalf("stream=%v: startup failure = %+v, %v", stream, result, err)
		}
	}
}

func TestRepositoryClonePolicyRequiresOriginHEAD(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mustGit(t, repo, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
	r, err := discoverRepository(repo, repositoryOptions{stdout: io.Discard, stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ensureSupportedCloneLayout(); err == nil || !strings.Contains(err.Error(), "missing refs/remotes/origin/HEAD") {
		t.Fatalf("expected existing origin/HEAD requirement, got %v", err)
	}
}

func TestCLIDoesNotEnableLinkedWorktreeMutationYet(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	mustGit(t, repo, "worktree", "add", "-b", "feature", linked)
	out, code := runCLIInRepoAndCapture(t, linked, []string{"new", "must-not-exist"})
	if code == 0 || !strings.Contains(out, "not supported yet") {
		t.Fatalf("expected worktree refusal, got exit %d: %s", code, out)
	}
	if branchExistsInRepo(repo, "must-not-exist") {
		t.Fatal("created a branch before worktree ownership safeguards")
	}
}
