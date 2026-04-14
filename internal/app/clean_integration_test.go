package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanDefaultsToCurrentStackScope(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustRunCLI(t, cli, []string{"new", "old-a"})
		mustWriteFile(t, filepath.Join(repo, "old-a.txt"), "old-a\n")
		mustGit(t, repo, "add", "old-a.txt")
		mustGit(t, repo, "commit", "-m", "old a")
		headA, err := gitOutput("rev-parse", "old-a")
		if err != nil {
			t.Fatalf("resolve old-a head: %v", err)
		}
		mustGit(t, repo, "push", "-u", "origin", "old-a")

		mustRunCLI(t, cli, []string{"new", "active-a", "--parent", "old-a"})
		mustWriteFile(t, filepath.Join(repo, "active-a.txt"), "active-a\n")
		mustGit(t, repo, "add", "active-a.txt")
		mustGit(t, repo, "commit", "-m", "active a")

		mustGit(t, repo, "switch", "main")
		mustRunCLI(t, cli, []string{"new", "old-b", "--parent", "main"})
		mustWriteFile(t, filepath.Join(repo, "old-b.txt"), "old-b\n")
		mustGit(t, repo, "add", "old-b.txt")
		mustGit(t, repo, "commit", "-m", "old b")
		headB, err := gitOutput("rev-parse", "old-b")
		if err != nil {
			t.Fatalf("resolve old-b head: %v", err)
		}
		mustGit(t, repo, "push", "-u", "origin", "old-b")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--no-ff", "old-a", "-m", "merge old-a")
		mergeA, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve old-a merge commit: %v", err)
		}
		mustGit(t, repo, "merge", "--no-ff", "old-b", "-m", "merge old-b")
		mergeB, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve old-b merge commit: %v", err)
		}
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":old-a")
		mustGit(t, repo, "push", "origin", ":old-b")
		mustGit(t, repo, "switch", "active-a")

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ] && [ \"$3\" = \"--head\" ]; then\n  if [ \"$4\" = \"old-a\" ]; then\n    cat <<'EOF'\n[{\"number\":21,\"url\":\"https://example.invalid/pr/21\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headA)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeA)+"\"}}]\nEOF\n    exit 0\n  fi\n  if [ \"$4\" = \"old-b\" ]; then\n    cat <<'EOF'\n[{\"number\":22,\"url\":\"https://example.invalid/pr/22\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headB)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeB)+"\"}}]\nEOF\n    exit 0\n  fi\n  echo '[]'\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"clean", "--yes"})
		if code != 0 {
			t.Fatalf("clean failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "old-a -> deleted local branch") {
			t.Fatalf("expected old-a clean output, got:\n%s", out)
		}
		if strings.Contains(out, "old-b -> deleted local branch") {
			t.Fatalf("did not expect unrelated stack clean output, got:\n%s", out)
		}
		if branchExists("old-a") {
			t.Fatalf("expected old-a to be removed")
		}
		if !branchExists("old-b") {
			t.Fatalf("expected old-b to remain outside current stack scope")
		}

		state, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state: %v", err)
		}
		if _, ok := state.Branches["old-a"]; ok {
			t.Fatalf("expected old-a removed from stack state")
		}
		if _, ok := state.Branches["old-b"]; !ok {
			t.Fatalf("expected old-b to remain tracked outside current stack scope")
		}
		if got := state.Branches["active-a"].Parent; got != "main" {
			t.Fatalf("expected active-a reparented to main, got %q", got)
		}
	})
}

func TestCleanUntrackedIncludesGlobalUntrackedBranches(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustRunCLI(t, cli, []string{"new", "tracked-a"})
		mustWriteFile(t, filepath.Join(repo, "tracked-a.txt"), "tracked-a\n")
		mustGit(t, repo, "add", "tracked-a.txt")
		mustGit(t, repo, "commit", "-m", "tracked a")
		headA, err := gitOutput("rev-parse", "tracked-a")
		if err != nil {
			t.Fatalf("resolve tracked-a head: %v", err)
		}
		mustGit(t, repo, "push", "-u", "origin", "tracked-a")

		mustRunCLI(t, cli, []string{"new", "active-child", "--parent", "tracked-a"})
		mustWriteFile(t, filepath.Join(repo, "active-child.txt"), "active-child\n")
		mustGit(t, repo, "add", "active-child.txt")
		mustGit(t, repo, "commit", "-m", "active child")

		mustGit(t, repo, "switch", "main")
		mustRunCLI(t, cli, []string{"new", "tracked-b", "--parent", "main"})
		mustWriteFile(t, filepath.Join(repo, "tracked-b.txt"), "tracked-b\n")
		mustGit(t, repo, "add", "tracked-b.txt")
		mustGit(t, repo, "commit", "-m", "tracked b")
		headB, err := gitOutput("rev-parse", "tracked-b")
		if err != nil {
			t.Fatalf("resolve tracked-b head: %v", err)
		}
		mustGit(t, repo, "push", "-u", "origin", "tracked-b")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "switch", "-c", "untracked-old")
		mustWriteFile(t, filepath.Join(repo, "untracked-old.txt"), "untracked-old\n")
		mustGit(t, repo, "add", "untracked-old.txt")
		mustGit(t, repo, "commit", "-m", "untracked old")
		headUntracked, err := gitOutput("rev-parse", "untracked-old")
		if err != nil {
			t.Fatalf("resolve untracked-old head: %v", err)
		}

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--no-ff", "tracked-a", "-m", "merge tracked-a")
		mergeA, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve tracked-a merge commit: %v", err)
		}
		mustGit(t, repo, "merge", "--no-ff", "tracked-b", "-m", "merge tracked-b")
		mergeB, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve tracked-b merge commit: %v", err)
		}
		mustGit(t, repo, "merge", "--no-ff", "untracked-old", "-m", "merge untracked-old")
		mergeUntracked, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve untracked-old merge commit: %v", err)
		}
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":tracked-a")
		mustGit(t, repo, "push", "origin", ":tracked-b")
		mustGit(t, repo, "switch", "active-child")

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ] && [ \"$3\" = \"--head\" ]; then\n  if [ \"$4\" = \"tracked-a\" ]; then\n    cat <<'EOF'\n[{\"number\":31,\"url\":\"https://example.invalid/pr/31\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headA)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeA)+"\"}}]\nEOF\n    exit 0\n  fi\n  if [ \"$4\" = \"tracked-b\" ]; then\n    cat <<'EOF'\n[{\"number\":32,\"url\":\"https://example.invalid/pr/32\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headB)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeB)+"\"}}]\nEOF\n    exit 0\n  fi\n  if [ \"$4\" = \"untracked-old\" ]; then\n    cat <<'EOF'\n[{\"number\":33,\"url\":\"https://example.invalid/pr/33\",\"baseRefName\":\"main\",\"headRefOid\":\""+strings.TrimSpace(headUntracked)+"\",\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergeUntracked)+"\"}}]\nEOF\n    exit 0\n  fi\n  echo '[]'\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"clean", "--yes", "--untracked"})
		if code != 0 {
			t.Fatalf("clean failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "tracked-a -> deleted local branch") {
			t.Fatalf("expected tracked-a clean output, got:\n%s", out)
		}
		if strings.Contains(out, "tracked-b -> deleted local branch") {
			t.Fatalf("did not expect unrelated tracked stack clean output, got:\n%s", out)
		}
		if !strings.Contains(out, "untracked-old -> deleted local branch") {
			t.Fatalf("expected global untracked clean output, got:\n%s", out)
		}
	})
}

func TestCleanWithoutInitializedStateAutoBootstraps(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"clean"})
	if code != 0 {
		t.Fatalf("clean failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "initialized stack state") {
		t.Fatalf("expected auto-bootstrap output, got:\n%s", out)
	}
	if !strings.Contains(out, "clean: nothing to do") {
		t.Fatalf("expected noop cleanup output, got:\n%s", out)
	}
	if _, err := loadState(repo); err != nil {
		t.Fatalf("expected state file to be persisted, got: %v", err)
	}
}
