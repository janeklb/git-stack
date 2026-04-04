package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestPruneLocalDeletesMergedTrackedBranchAndUpdatesState(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "tracked-old"})

		mustWriteFile(t, filepath.Join(repo, "tracked.txt"), "tracked\n")
		mustGit(t, repo, "add", "tracked.txt")
		mustGit(t, repo, "commit", "-m", "tracked feature")
		headOID, err := gitOutput("rev-parse", "tracked-old")
		if err != nil {
			t.Fatalf("resolve tracked-old head: %v", err)
		}
		mustGit(t, repo, "push", "-u", "origin", "tracked-old")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--no-ff", "tracked-old", "-m", "merge tracked-old")
		mergeOID, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve merge commit: %v", err)
		}
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":tracked-old")

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  cat <<'EOF'\n[{\"number\":17,\"url\":\"https://example.invalid/pr/17\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headOID)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeOID)+"\"}}]\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"prune-local", "--yes"})
		if code != 0 {
			t.Fatalf("prune-local failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "tracked-old -> deleted local branch") {
			t.Fatalf("expected local branch deletion output, got:\n%s", out)
		}
		if branchExists("tracked-old") {
			t.Fatalf("expected tracked-old to be removed")
		}

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		if _, ok := state.Branches["tracked-old"]; ok {
			t.Fatalf("expected tracked-old to be removed from stack state")
		}
	})
}
