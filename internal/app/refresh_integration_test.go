package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshCleansSquashMergedBranchAndReparentsChildren(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
		mustGit(t, repo, "add", "one.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")

		mustRunCLI(t, cli, []string{"new", "feat-two", "--parent", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
		mustGit(t, repo, "add", "two.txt")
		mustGit(t, repo, "commit", "-m", "feat two")
		mustGit(t, repo, "push", "-u", "origin", "feat-two")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--squash", "feat-one")
		mustGit(t, repo, "commit", "-m", "squash feat one")
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":feat-one")
		mustGit(t, repo, "switch", "feat-one")

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
		state.Branches["feat-two"].PR = &PRMeta{Number: 2, URL: "https://example.invalid/pr/2", Base: "feat-one"}
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\"}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\"}\nEOF\n    exit 0\n  fi\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIWithInputAndCapture(t, cli, []string{"refresh"}, "y\n")
		if code != 0 {
			t.Fatalf("refresh failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "refresh completed") {
			t.Fatalf("expected completion output, got:\n%s", out)
		}

		if branchExists("feat-one") {
			t.Fatalf("expected feat-one local branch to be deleted")
		}
		cur, err := currentBranch()
		if err != nil {
			t.Fatalf("current branch: %v", err)
		}
		if cur != "main" {
			t.Fatalf("expected refresh to switch to trunk before deletion, got %s", cur)
		}

		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after refresh: %v", err)
		}
		if _, ok := stateAfter.Branches["feat-one"]; ok {
			t.Fatalf("expected feat-one removed from active branches")
		}
		if stateAfter.Archived["feat-one"] == nil {
			t.Fatalf("expected feat-one archived lineage to persist")
		}
		if got := stateAfter.Branches["feat-two"].Parent; got != "main" {
			t.Fatalf("expected feat-two parent reparented to main, got %q", got)
		}
		if got := stateAfter.Branches["feat-two"].LineageParent; got != "feat-one" {
			t.Fatalf("expected feat-two lineage parent to remain feat-one, got %q", got)
		}
	})
}

func TestRefreshCancelLeavesStateUntouched(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
		mustGit(t, repo, "add", "one.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")
		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--squash", "feat-one")
		mustGit(t, repo, "commit", "-m", "squash feat one")
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":feat-one")

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\"}\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIWithInputAndCapture(t, cli, []string{"refresh"}, "n\n")
		if code != 0 {
			t.Fatalf("refresh failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "refresh cancelled") {
			t.Fatalf("expected refresh cancelled message, got:\n%s", out)
		}
		if !branchExists("feat-one") {
			t.Fatalf("expected feat-one branch to remain when refresh cancelled")
		}
	})
}

func TestRefreshKeepsMergedBranchWhenLocalHasNewCommitsAfterPRHead(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
		mustGit(t, repo, "add", "one.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--no-ff", "feat-one", "-m", "merge feat one")
		mergeSHA, err := gitOutput("rev-parse", "main")
		if err != nil {
			t.Fatalf("read merge sha: %v", err)
		}
		mergeSHA = strings.TrimSpace(mergeSHA)
		headSHA, err := gitOutput("rev-parse", "feat-one")
		if err != nil {
			t.Fatalf("read head sha: %v", err)
		}
		headSHA = strings.TrimSpace(headSHA)
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":feat-one")

		mustGit(t, repo, "switch", "feat-one")
		mustWriteFile(t, filepath.Join(repo, "one.txt"), "diverged\n")
		mustGit(t, repo, "add", "one.txt")
		mustGit(t, repo, "commit", "-m", "local diverged commit")

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		ghScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"headRefOid\":\"%s\",\"title\":\"merged\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\"%s\"}}\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n", headSHA, mergeSHA)
		mustWriteFile(t, ghPath, ghScript)
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"refresh"})
		if code != 0 {
			t.Fatalf("refresh failed: exit=%d\n%s", code, out)
		}
		if strings.Contains(out, "feat-one -> cleaned merged branch from local stack state") {
			t.Fatalf("expected merged branch to be kept when local branch has new commits, got:\n%s", out)
		}
		if !strings.Contains(out, "refresh: nothing to do") {
			t.Fatalf("expected noop message, got:\n%s", out)
		}

		if !branchExists("feat-one") {
			t.Fatalf("expected feat-one local branch to remain")
		}
		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after refresh: %v", err)
		}
		if _, ok := stateAfter.Branches["feat-one"]; !ok {
			t.Fatalf("expected feat-one to remain in active branches")
		}
	})
}

func TestRefreshNoopExitsWithoutPrompt(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		out, code := runCLIAndCapture(t, cli, []string{"refresh"})
		if code != 0 {
			t.Fatalf("refresh failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "refresh: nothing to do") {
			t.Fatalf("expected noop message, got:\n%s", out)
		}
		if strings.Contains(out, "apply refresh plan?") {
			t.Fatalf("did not expect confirmation prompt for noop refresh, got:\n%s", out)
		}
		if strings.Contains(out, "refresh cancelled") {
			t.Fatalf("did not expect cancel output for noop refresh, got:\n%s", out)
		}
	})
}
