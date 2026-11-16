package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (r *repository) ensureSupportedCloneLayout() error {
	originURL, err := r.gitOutputTrimmed("config", "--get", "remote.origin.url")
	if err != nil || originURL == "" {
		return errors.New("missing required remote 'origin'; this tool expects a full clone with origin configured")
	}

	fetchSpecs, err := r.gitOutputTrimmed("config", "--get-all", "remote.origin.fetch")
	if err != nil {
		return errors.New("single-branch clones are not supported; fetch all branches or reclone without --single-branch")
	}

	for _, line := range strings.Split(strings.TrimSpace(fetchSpecs), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "refs/heads/*:refs/remotes/origin/*") {
			if _, err := r.gitOutputTrimmed("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err != nil {
				return errors.New("missing refs/remotes/origin/HEAD; run 'git remote set-head origin --auto' after fetching all branches")
			}
			return nil
		}
	}

	return errors.New("single-branch clones are not supported; fetch all branches or reclone without --single-branch")
}

func (r *repository) ensureCleanWorktree() error {
	out, err := r.gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return errors.New("working tree is not clean; commit/stash changes first")
	}
	return nil
}

func (r *repository) branchExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	err := r.gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true
	}
	err = r.gitRunQuiet("show-ref", "--verify", "--quiet", "refs/remotes/origin/"+name)
	return err == nil
}

func (r *repository) LocalBranchExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	err := r.gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func (r *repository) detectTrunk() (string, error) {
	out, err := r.gitOutputTrimmed("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", errors.New("failed to detect trunk from refs/remotes/origin/HEAD")
	}
	out = strings.TrimPrefix(out, "origin/")
	if out == "" {
		return "", errors.New("failed to detect trunk from refs/remotes/origin/HEAD")
	}
	return out, nil
}

func (r *repository) CurrentBranch() (string, error) {
	branch, err := r.gitOutputTrimmed("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if branch == "HEAD" {
		return "", errors.New("detached HEAD is not supported")
	}
	return branch, nil
}

func (r *repository) hasHeadCommit() bool {
	_, err := r.gitOutput("rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

func (r *repository) ListLocalBranches() ([]string, error) {
	out, err := r.gitOutput("for-each-ref", "--format=%(refname:short)", "refs/heads")
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

func (r *repository) ListOriginBranches() ([]string, error) {
	out, err := r.gitOutput("for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
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

func (r *repository) branchTimestamp(branch string) (int64, error) {
	out, err := r.gitOutputTrimmed("show", "-s", "--format=%ct", branch)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *repository) PushBranch(branch string) error {
	refspec := fmt.Sprintf("%s:%s", branch, branch)
	_, err := r.runGit([]string{"push", "--force-with-lease", "-u", "origin", refspec}, commandRunOptions{streamOutput: true, boxMode: commandBoxAlways})
	return err
}

func (r *repository) RemoteBranchExists(branch string) (bool, error) {
	if strings.TrimSpace(branch) == "" {
		return false, nil
	}
	out, err := r.gitOutputTrimmed("ls-remote", "--heads", "origin", branch)
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func (r *repository) DeleteLocalBranch(branch string) error {
	return r.gitRunQuiet("branch", "-D", branch)
}

func (r *repository) BranchFullyIntegrated(branch, base string) (bool, error) {
	baseRef, err := r.resolveComparisonBase(base)
	if err != nil {
		return false, err
	}
	out, err := r.gitOutput("cherry", baseRef, branch)
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

func (r *repository) BaseContainsCommit(base, commit string) (bool, error) {
	baseRef, err := r.resolveComparisonBase(base)
	if err != nil {
		return false, err
	}
	return r.commitIsAncestor(strings.TrimSpace(commit), baseRef)
}

func (r *repository) BranchAtOrBehindCommit(branch, commit string) (bool, error) {
	branchRef, err := r.resolveBranchRef(branch)
	if err != nil {
		return false, err
	}
	return r.commitIsAncestor(branchRef, strings.TrimSpace(commit))
}

func (r *repository) EnsurePRHeadCommit(number int, commit string) error {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return errors.New("empty PR head commit")
	}
	if _, err := r.gitOutput("rev-parse", "--verify", "--quiet", commit+"^{commit}"); err == nil {
		return nil
	}
	if number <= 0 {
		return errors.New("missing PR number for head fetch")
	}
	// Deleted branch heads are not fetched by the normal origin refspec. GitHub
	// retains the PR head ref even after a squash merge and branch deletion.
	ref := fmt.Sprintf("refs/pull/%d/head", number)
	if err := r.gitRunQuiet("fetch", "--no-tags", "--no-write-fetch-head", "origin", ref); err != nil {
		return err
	}
	// Check the exact head reported by GitHub, not just whatever the ref fetched.
	_, err := r.gitOutput("rev-parse", "--verify", "--quiet", commit+"^{commit}")
	return err
}

func (r *repository) BranchHasCommitsSince(base, branch string) (bool, error) {
	baseRef, err := r.resolveBranchRef(base)
	if err != nil {
		return false, err
	}
	branchRef, err := r.resolveBranchRef(branch)
	if err != nil {
		return false, err
	}
	if baseRef == branchRef {
		return false, nil
	}
	out, err := r.gitOutputTrimmed("rev-list", "--count", baseRef+".."+branchRef)
	if err != nil {
		return false, err
	}
	return out != "0", nil
}

func (r *repository) branchMatchesRemote(branch string) (bool, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false, errors.New("empty branch")
	}
	remoteRef := "refs/remotes/origin/" + branch
	if r.gitRunQuiet("show-ref", "--verify", "--quiet", remoteRef) != nil {
		return false, nil
	}
	localOID, err := r.resolveBranchRef(branch)
	if err != nil {
		return false, err
	}
	remoteOID, err := r.resolveBranchRef(remoteRef)
	if err != nil {
		return false, err
	}
	return localOID == remoteOID, nil
}

func (r *repository) resolveBranchRef(branch string) (string, error) {
	if strings.TrimSpace(branch) == "" {
		return "", errors.New("empty branch")
	}
	out, err := r.gitOutputTrimmed("rev-parse", strings.TrimSpace(branch))
	if err != nil {
		return "", err
	}
	return out, nil
}

func (r *repository) commitIsAncestor(ancestor, descendant string) (bool, error) {
	if strings.TrimSpace(ancestor) == "" || strings.TrimSpace(descendant) == "" {
		return false, errors.New("empty commit for ancestry check")
	}
	result, err := r.runGit([]string{"merge-base", "--is-ancestor", strings.TrimSpace(ancestor), strings.TrimSpace(descendant)}, commandRunOptions{streamOutput: false, boxMode: commandBoxOnFailure})
	if err != nil {
		if result.exitCode == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *repository) resolveComparisonBase(base string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", errors.New("empty comparison base")
	}
	remoteRef := "refs/remotes/origin/" + base
	if r.gitRunQuiet("show-ref", "--verify", "--quiet", remoteRef) == nil {
		return "origin/" + base, nil
	}
	if r.gitRunQuiet("show-ref", "--verify", "--quiet", "refs/heads/"+base) == nil {
		return base, nil
	}
	return "", fmt.Errorf("comparison base not found: %s", base)
}

func (r *repository) rebaseInProgress() (bool, error) {
	apply, err := r.gitPath("rebase-apply")
	if err != nil {
		return false, err
	}
	merge, err := r.gitPath("rebase-merge")
	if err != nil {
		return false, err
	}
	if pathExists(apply) || pathExists(merge) {
		return true, nil
	}
	return false, nil
}

func (r *repository) mergeInProgress() (bool, error) {
	mergeHead, err := r.gitPath("MERGE_HEAD")
	if err != nil {
		return false, err
	}
	return pathExists(mergeHead), nil
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (r *repository) gitPath(name string) (string, error) {
	return r.gitOutputTrimmed("rev-parse", "--path-format=absolute", "--git-path", name)
}

func (r *repository) Run(args ...string) error {
	_, err := r.runGit(args, commandRunOptions{streamOutput: true, boxMode: gitRunBoxMode(args)})
	return err
}

func (r *repository) gitRunWithEnv(env []string, args ...string) error {
	_, err := r.runGit(args, commandRunOptions{streamOutput: true, boxMode: gitRunBoxMode(args), env: env})
	return err
}

func (r *repository) gitRunQuiet(args ...string) error {
	_, err := r.runGit(args, commandRunOptions{streamOutput: true, boxMode: commandBoxOnFailure})
	return err
}

func (r *repository) gitOutput(args ...string) (string, error) {
	result, err := r.runGit(args, commandRunOptions{streamOutput: false, boxMode: commandBoxOnFailure})
	return result.stdout, err
}

func combineCommandOutput(result commandRunResult) string {
	parts := []string{strings.TrimSpace(result.stdout), strings.TrimSpace(result.stderr)}
	combined := strings.TrimSpace(strings.Join(parts, "\n"))
	return combined
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
