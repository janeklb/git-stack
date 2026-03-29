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

func ensureSupportedCloneLayout() error {
	originURL, err := gitOutput("config", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(originURL) == "" {
		return errors.New("missing required remote 'origin'; this tool expects a full clone with origin configured")
	}

	fetchSpecs, err := gitOutput("config", "--get-all", "remote.origin.fetch")
	if err != nil {
		return errors.New("single-branch clones are not supported; fetch all branches or reclone without --single-branch")
	}

	for _, line := range strings.Split(strings.TrimSpace(fetchSpecs), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "refs/heads/*:refs/remotes/origin/*") {
			if _, err := gitOutput("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err != nil {
				return errors.New("missing refs/remotes/origin/HEAD; run 'git remote set-head origin --auto' after fetching all branches")
			}
			return nil
		}
	}

	return errors.New("single-branch clones are not supported; fetch all branches or reclone without --single-branch")
}

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

func localBranchExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	err := gitRun("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func detectTrunk() (string, error) {
	out, err := gitOutput("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", errors.New("failed to detect trunk from refs/remotes/origin/HEAD")
	}
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "origin/")
	if out == "" {
		return "", errors.New("failed to detect trunk from refs/remotes/origin/HEAD")
	}
	return out, nil
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

func hasHeadCommit() bool {
	_, err := gitOutput("rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
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
	refspec := fmt.Sprintf("%s:%s", branch, branch)
	out, err := gitCombined("push", "-u", "origin", refspec)
	if err == nil {
		printGitOutput(out)
		return nil
	}
	if shouldRetryPushWithLease(out) {
		out, forceErr := gitCombined("push", "--force-with-lease", "-u", "origin", refspec)
		printGitOutput(out)
		if forceErr == nil {
			return nil
		}
		return forceErr
	}
	return err
}

func shouldRetryPushWithLease(output string) bool {
	msg := strings.ToLower(output)
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "behind its remote") ||
		strings.Contains(msg, "behind its remote counterpart")
}

func printGitOutput(out string) {
	out = strings.TrimSpace(out)
	if out == "" {
		return
	}
	fmt.Println(out)
}

func gitCombined(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(combined))
		if msg == "" {
			msg = err.Error()
		}
		return msg, errors.New(msg)
	}
	return string(combined), nil
}

func remoteBranchExists(branch string) (bool, error) {
	if strings.TrimSpace(branch) == "" {
		return false, nil
	}
	out, err := gitOutput("ls-remote", "--heads", "origin", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func deleteLocalBranch(branch string) error {
	if err := gitRun("branch", "-d", branch); err != nil {
		return gitRun("branch", "-D", branch)
	}
	return nil
}

func branchFullyIntegrated(branch, base string) (bool, error) {
	baseRef, err := resolveComparisonBase(base)
	if err != nil {
		return false, err
	}
	out, err := gitOutput("cherry", baseRef, branch)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "+") {
			return false, nil
		}
	}
	return true, nil
}

func resolveComparisonBase(base string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", errors.New("empty comparison base")
	}
	if gitRun("show-ref", "--verify", "--quiet", "refs/heads/"+base) == nil {
		return base, nil
	}
	remoteRef := "refs/remotes/origin/" + base
	if gitRun("show-ref", "--verify", "--quiet", remoteRef) == nil {
		return "origin/" + base, nil
	}
	return "", fmt.Errorf("comparison base not found: %s", base)
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
