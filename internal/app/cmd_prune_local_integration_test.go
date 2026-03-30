package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPruneLocalDeletesMergedUntrackedBranch(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustGit(t, repo, "switch", "-c", "cleanup-me")
		mustWriteFile(t, filepath.Join(repo, "cleanup.txt"), "cleanup\n")
		mustGit(t, repo, "add", "cleanup.txt")
		mustGit(t, repo, "commit", "-m", "cleanup me")
		headSHA, err := gitOutput("rev-parse", "cleanup-me")
		if err != nil {
			t.Fatalf("read head sha: %v", err)
		}
		headSHA = strings.TrimSpace(headSHA)
		mustGit(t, repo, "push", "-u", "origin", "cleanup-me")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--no-ff", "cleanup-me", "-m", "merge cleanup branch")
		mergeSHA, err := gitOutput("rev-parse", "main")
		if err != nil {
			t.Fatalf("read merge sha: %v", err)
		}
		mergeSHA = strings.TrimSpace(mergeSHA)
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":cleanup-me")

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		ghScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  cat <<'EOF'\n[{\"number\":11,\"url\":\"https://example.invalid/pr/11\",\"baseRefName\":\"main\",\"headRefOid\":\"%s\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\"%s\"}}]\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n", headSHA, mergeSHA)
		mustWriteFile(t, ghPath, ghScript)
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIWithInputAndCapture(t, cli, []string{"prune-local"}, "y\n")
		if code != 0 {
			t.Fatalf("prune-local failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "apply prune-local plan?") {
			t.Fatalf("expected confirmation prompt, got:\n%s", out)
		}
		if !strings.Contains(out, "cleanup-me -> deleted local branch") {
			t.Fatalf("expected deletion output, got:\n%s", out)
		}
		if branchExists("cleanup-me") {
			t.Fatalf("expected cleanup-me local branch deleted")
		}
	})
}

func TestPruneLocalNoopSkipsPrompt(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		out, code := runCLIAndCapture(t, cli, []string{"prune-local"})
		if code != 0 {
			t.Fatalf("prune-local failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "prune-local: nothing to do") {
			t.Fatalf("expected noop output, got:\n%s", out)
		}
		if strings.Contains(out, "apply prune-local plan?") {
			t.Fatalf("did not expect prompt for noop prune-local, got:\n%s", out)
		}
	})
}
