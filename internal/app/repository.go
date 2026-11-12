package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// repository owns Git layout and execution, not stack or forge policy. Shared
// metadata uses commonGitDir; recovery belongs under worktreeGitDir.
type repository struct {
	worktreeRoot   string
	worktreeGitDir string
	commonGitDir   string
	runner         commandRunner
	options        commandRunOptions
}

type repositoryOptions struct {
	runner commandRunner
	in     io.Reader
	stdout io.Writer
	stderr io.Writer
	env    []string // nil snapshots the inherited environment.
}

type commandError struct {
	command string
	result  commandRunResult
	cause   error
}

func (e *commandError) Error() string {
	message := fmt.Sprintf("%s (exit %d): %v", e.command, e.result.exitCode, e.cause)
	if output := combineCommandOutput(e.result); output != "" {
		message += "\n" + output
	}
	return message
}

func (e *commandError) Unwrap() error { return e.cause }

func discoverRepository(dir string, opts repositoryOptions) (*repository, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if opts.runner == nil {
		opts.runner = runCommand
	}
	if opts.env == nil {
		opts.env = os.Environ()
	}
	r := &repository{runner: opts.runner, options: commandRunOptions{
		dir: absDir, stdin: opts.in, stdout: opts.stdout, stderr: opts.stderr,
		baseEnv: append([]string{}, opts.env...),
	}}
	bare, err := r.gitOutputTrimmed("rev-parse", "--is-bare-repository")
	if err != nil {
		return nil, fmt.Errorf("discover repository: %w", err)
	}
	if bare == "true" {
		return nil, errors.New("bare repositories are not supported as command working trees")
	}
	for _, path := range []struct {
		flag string
		dest *string
	}{
		{"--show-toplevel", &r.worktreeRoot},
		{"--absolute-git-dir", &r.worktreeGitDir},
		{"--git-common-dir", &r.commonGitDir},
	} {
		result, err := r.runGit([]string{"rev-parse", "--path-format=absolute", path.flag}, commandRunOptions{boxMode: commandBoxOnFailure})
		if err != nil {
			return nil, fmt.Errorf("discover %s: %w", path.flag, err)
		}
		*path.dest = strings.TrimSuffix(result.stdout, "\n")
		if !filepath.IsAbs(*path.dest) {
			return nil, fmt.Errorf("git returned a non-absolute %s: %q", path.flag, *path.dest)
		}
	}
	r.options.dir = r.worktreeRoot
	return r, nil
}

func (r *repository) runGit(args []string, opts commandRunOptions) (commandRunResult, error) {
	opts.dir = r.options.dir
	opts.stdout, opts.stderr = r.options.stdout, r.options.stderr
	// Discovery and other captured queries must not consume interactive input.
	if opts.stdin == nil && opts.streamOutput {
		opts.stdin = r.options.stdin
	}
	opts.baseEnv = r.options.baseEnv
	result, err := r.runner("git", args, opts)
	if err != nil {
		return result, &commandError{command: formatCommand("git", args), result: result, cause: err}
	}
	return result, nil
}

func (r *repository) gitOutputTrimmed(args ...string) (string, error) {
	result, err := r.runGit(args, commandRunOptions{boxMode: commandBoxOnFailure})
	return strings.TrimSpace(result.stdout), err
}

type worktreeCheckout struct {
	path     string
	branch   string // Full ref; empty for detached worktrees.
	detached bool
}

func (r *repository) worktrees() ([]worktreeCheckout, error) {
	result, err := r.runGit([]string{"worktree", "list", "--porcelain", "-z"}, commandRunOptions{boxMode: commandBoxOnFailure})
	if err != nil {
		return nil, err
	}
	var worktrees []worktreeCheckout
	for _, record := range strings.Split(result.stdout, "\x00\x00") {
		if record == "" {
			continue
		}
		var checkout worktreeCheckout
		for _, field := range strings.Split(record, "\x00") {
			key, value, _ := strings.Cut(field, " ")
			switch key {
			case "worktree":
				checkout.path = value
			case "branch":
				checkout.branch = value
			case "detached":
				checkout.detached = true
			}
		}
		if checkout.path == "" {
			return nil, errors.New("git worktree list returned a record without a path")
		}
		worktrees = append(worktrees, checkout)
	}
	return worktrees, nil
}
