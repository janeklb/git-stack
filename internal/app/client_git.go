package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	err := gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true
	}
	err = gitRunQuiet("show-ref", "--verify", "--quiet", "refs/remotes/origin/"+name)
	return err == nil
}

func localBranchExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	err := gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+name)
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

func listOriginBranches() ([]string, error) {
	out, err := gitOutput("for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	if err != nil {
		return nil, err
	}
	branches := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "origin/HEAD" {
			continue
		}
		line = strings.TrimPrefix(line, "origin/")
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
	result, err := runCommand("git", []string{"push", "--force-with-lease", "-u", "origin", refspec}, commandRunOptions{streamOutput: true, boxMode: commandBoxAlways})
	if err != nil {
		return commandRunError(result, err)
	}
	return nil
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
	return gitRunQuiet("branch", "-D", branch)
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

func baseContainsCommit(base, commit string) (bool, error) {
	baseRef, err := resolveComparisonBase(base)
	if err != nil {
		return false, err
	}
	return commitIsAncestor(strings.TrimSpace(commit), baseRef)
}

func branchAtOrBehindCommit(branch, commit string) (bool, error) {
	branchRef, err := resolveBranchRef(branch)
	if err != nil {
		return false, err
	}
	return commitIsAncestor(branchRef, strings.TrimSpace(commit))
}

func branchHasCommitsSince(base, branch string) (bool, error) {
	baseRef, err := resolveBranchRef(base)
	if err != nil {
		return false, err
	}
	branchRef, err := resolveBranchRef(branch)
	if err != nil {
		return false, err
	}
	if baseRef == branchRef {
		return false, nil
	}
	out, err := gitOutput("rev-list", "--count", baseRef+".."+branchRef)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

func branchMatchesRemote(branch string) (bool, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false, errors.New("empty branch")
	}
	remoteRef := "refs/remotes/origin/" + branch
	if gitRunQuiet("show-ref", "--verify", "--quiet", remoteRef) != nil {
		return false, nil
	}
	localOID, err := resolveBranchRef(branch)
	if err != nil {
		return false, err
	}
	remoteOID, err := resolveBranchRef(remoteRef)
	if err != nil {
		return false, err
	}
	return localOID == remoteOID, nil
}

func resolveBranchRef(branch string) (string, error) {
	if strings.TrimSpace(branch) == "" {
		return "", errors.New("empty branch")
	}
	out, err := gitOutput("rev-parse", strings.TrimSpace(branch))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func commitIsAncestor(ancestor, descendant string) (bool, error) {
	if strings.TrimSpace(ancestor) == "" || strings.TrimSpace(descendant) == "" {
		return false, errors.New("empty commit for ancestry check")
	}
	result, err := runCommand("git", []string{"merge-base", "--is-ancestor", strings.TrimSpace(ancestor), strings.TrimSpace(descendant)}, commandRunOptions{streamOutput: false, boxMode: commandBoxOnFailure})
	if err != nil {
		if result.exitCode == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func resolveComparisonBase(base string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", errors.New("empty comparison base")
	}
	remoteRef := "refs/remotes/origin/" + base
	if gitRunQuiet("show-ref", "--verify", "--quiet", remoteRef) == nil {
		return "origin/" + base, nil
	}
	if gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+base) == nil {
		return base, nil
	}
	return "", fmt.Errorf("comparison base not found: %s", base)
}

func rebaseInProgress() (bool, error) {
	apply, err := gitOutput("rev-parse", "--git-path", "rebase-apply")
	if err != nil {
		return false, err
	}
	merge, err := gitOutput("rev-parse", "--git-path", "rebase-merge")
	if err != nil {
		return false, err
	}
	if pathExists(strings.TrimSpace(apply)) || pathExists(strings.TrimSpace(merge)) {
		return true, nil
	}
	return false, nil
}

func mergeInProgress() (bool, error) {
	mergeHead, err := gitOutput("rev-parse", "--git-path", "MERGE_HEAD")
	if err != nil {
		return false, err
	}
	return pathExists(strings.TrimSpace(mergeHead)), nil
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func gitRun(args ...string) error {
	if _, err := runCommand("git", args, commandRunOptions{streamOutput: true, boxMode: gitRunBoxMode(args)}); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func gitRunQuiet(args ...string) error {
	if _, err := runCommand("git", args, commandRunOptions{streamOutput: true, boxMode: commandBoxOnFailure}); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func gitOutput(args ...string) (string, error) {
	result, err := runCommand("git", args, commandRunOptions{streamOutput: false, boxMode: commandBoxOnFailure})
	if err != nil {
		msg := strings.TrimSpace(result.stderr)
		if msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return result.stdout, nil
}

func combineCommandOutput(result commandRunResult) string {
	parts := []string{strings.TrimSpace(result.stdout), strings.TrimSpace(result.stderr)}
	combined := strings.TrimSpace(strings.Join(parts, "\n"))
	return combined
}

func commandRunError(result commandRunResult, fallback error) error {
	msg := combineCommandOutput(result)
	if msg == "" {
		return fallback
	}
	return errors.New(msg)
}

func gitRunBoxMode(args []string) commandBoxMode {
	if len(args) == 0 {
		return commandBoxAlways
	}

	switch args[0] {
	case "switch", "fetch", "show-ref", "merge-base":
		return commandBoxOnFailure
	case "branch":
		if len(args) > 1 && (args[1] == "-d" || args[1] == "-D") {
			return commandBoxOnFailure
		}
	}

	return commandBoxAlways
}
