package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func ensureCleanWorktree() error {
	out, err := gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return errors.New("working tree is not clean; commit/stash changes first")
	}
	return nil
}

func branchExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	err := gitRun("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true
	}
	err = gitRun("show-ref", "--verify", "--quiet", "refs/remotes/origin/"+name)
	return err == nil
}

func detectTrunk() (string, error) {
	out, err := gitOutput("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		out = strings.TrimSpace(out)
		out = strings.TrimPrefix(out, "origin/")
		if out != "" {
			return out, nil
		}
	}
	if branchExists("main") {
		return "main", nil
	}
	if branchExists("master") {
		return "master", nil
	}
	cur, err := currentBranch()
	if err != nil {
		return "", errors.New("failed to detect trunk; pass --trunk explicitly")
	}
	return cur, nil
}

func currentBranch() (string, error) {
	out, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(out)
	if branch == "HEAD" {
		return "", errors.New("detached HEAD is not supported")
	}
	return branch, nil
}

func listLocalBranches() ([]string, error) {
	out, err := gitOutput("for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		branches = append(branches, line)
	}
	return branches, nil
}

func branchTimestamp(branch string) (int64, error) {
	out, err := gitOutput("show", "-s", "--format=%ct", branch)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func pushBranch(branch string) error {
	if err := gitRun("push", "-u", "origin", fmt.Sprintf("%s:%s", branch, branch)); err != nil {
		if strings.Contains(err.Error(), "upstream") || strings.Contains(err.Error(), "set-upstream") {
			return gitRun("push", "-u", "origin", branch)
		}
		if strings.Contains(err.Error(), "already exists") {
			return gitRun("push", "origin", fmt.Sprintf("%s:%s", branch, branch))
		}
		return err
	}
	return nil
}

func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return out.String(), nil
}
