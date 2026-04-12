package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Integration smoke tests cover end-to-end git/gh wiring with minimal scenarios.

func TestIntegrationSmokeSubmitMergedSkipPath(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state with PR metadata: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\"}\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"submit", "feat-one"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "feat-one -> PR #1 already merged, skipping") {
			t.Fatalf("expected merged skip message, got:\n%s", out)
		}
	})
}

func TestIntegrationSmokePruneLocalDeletesMergedUntrackedBranch(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustGit(t, repo, "switch", "-c", "old-feature")
		mustWriteFile(t, filepath.Join(repo, "old.txt"), "old\n")
		mustGit(t, repo, "add", "old.txt")
		mustGit(t, repo, "commit", "-m", "old feature")
		headOID, err := gitOutput("rev-parse", "old-feature")
		if err != nil {
			t.Fatalf("resolve old-feature head: %v", err)
		}
		mustGit(t, repo, "push", "-u", "origin", "old-feature")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--no-ff", "old-feature", "-m", "merge old feature")
		mergeOID, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve merge commit: %v", err)
		}
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":old-feature")

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  cat <<'EOF'\n[{\"number\":9,\"url\":\"https://example.invalid/pr/9\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headOID)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeOID)+"\"}}]\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"cleanup", "--yes", "--untracked"})
		if code != 0 {
			t.Fatalf("cleanup failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "old-feature -> deleted local branch") {
			t.Fatalf("expected local branch deletion output, got:\n%s", out)
		}
		if branchExists("old-feature") {
			t.Fatalf("expected old-feature to be removed")
		}
	})
}

func TestIntegrationSmokeStatusShowsTrackedBranch(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})

	out, code := runCLIInRepoAndCapture(t, repo, []string{"status"})
	if code != 0 {
		t.Fatalf("status failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "main (trunk)") || !strings.Contains(out, "feat-one") {
		t.Fatalf("expected trunk and branch in status output, got:\n%s", out)
	}
}
