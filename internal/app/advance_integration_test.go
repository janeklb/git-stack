package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdvanceSingleChildRunsCleanupRestackAndSubmit(t *testing.T) {
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
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"advance"})
		if code != 0 {
			t.Fatalf("advance failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "advance completed") {
			t.Fatalf("expected advance completion output, got:\n%s", out)
		}

		if branchExists("feat-one") {
			t.Fatalf("expected feat-one local branch to be deleted")
		}
		cur, err := currentBranch()
		if err != nil {
			t.Fatalf("current branch: %v", err)
		}
		if cur != "feat-two" {
			t.Fatalf("expected advance to switch to single child feat-two, got %s", cur)
		}

		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after advance: %v", err)
		}
		if _, ok := stateAfter.Branches["feat-one"]; ok {
			t.Fatalf("expected feat-one removed from active branches")
		}
		if got := stateAfter.Branches["feat-two"].Parent; got != "main" {
			t.Fatalf("expected feat-two parent reparented to main, got %q", got)
		}
		if got := stateAfter.Branches["feat-two"].PR.Base; got != "main" {
			t.Fatalf("expected feat-two PR base updated to main after submit, got %q", got)
		}
	})
}

func TestAdvanceAbortsWhenRemoteBranchStillExists(t *testing.T) {
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
		mustGit(t, repo, "switch", "feat-one")

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
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"advance"})
		if code == 0 {
			t.Fatalf("expected advance to fail when remote still exists, output:\n%s", out)
		}
		if !strings.Contains(out, "origin/feat-one still exists") {
			t.Fatalf("expected remote-exists guidance, got:\n%s", out)
		}

		if !branchExists("feat-one") {
			t.Fatalf("expected feat-one branch to remain after abort")
		}
		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after abort: %v", err)
		}
		if _, ok := stateAfter.Branches["feat-one"]; !ok {
			t.Fatalf("expected feat-one to remain tracked after abort")
		}
	})
}

func TestAdvanceDoesNotCleanupUnrelatedMergedTrackedBranches(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")
		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})

		mustRunCLI(t, cli, []string{"new", "old-merged"})
		mustWriteFile(t, filepath.Join(repo, "old.txt"), "old\n")
		mustGit(t, repo, "add", "old.txt")
		mustGit(t, repo, "commit", "-m", "old merged")
		mustGit(t, repo, "push", "-u", "origin", "old-merged")

		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "merge", "--squash", "old-merged")
		mustGit(t, repo, "commit", "-m", "squash old merged")
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":old-merged")

		mustRunCLI(t, cli, []string{"new", "feat-one", "--parent", "main"})
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
		state.Branches["old-merged"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
		state.Branches["feat-one"].PR = &PRMeta{Number: 2, URL: "https://example.invalid/pr/2", Base: "main"}
		state.Branches["feat-two"].PR = &PRMeta{Number: 3, URL: "https://example.invalid/pr/3", Base: "feat-one"}
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"3\" ]; then\n    cat <<'EOF'\n{\"number\":3,\"url\":\"https://example.invalid/pr/3\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"advance"})
		if code != 0 {
			t.Fatalf("advance failed: exit=%d\n%s", code, out)
		}

		if !branchExists("old-merged") {
			t.Fatalf("expected unrelated merged branch to remain after advance")
		}
		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after advance: %v", err)
		}
		if _, ok := stateAfter.Branches["old-merged"]; !ok {
			t.Fatalf("expected unrelated merged branch to remain tracked after advance")
		}
	})
}

func TestAdvanceUsesFetchedRemoteTrunkWhenLocalTrunkIsStale(t *testing.T) {
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
		mergedMain, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("resolve merged main head: %v", err)
		}
		mustGit(t, repo, "push", "origin", "main")
		mustGit(t, repo, "push", "origin", ":feat-one")
		mustGit(t, repo, "reset", "--hard", "HEAD~1")
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
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergedMain)+"\"}}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"advance"})
		if code != 0 {
			t.Fatalf("advance failed with stale local trunk: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "advance completed") {
			t.Fatalf("expected advance completion output, got:\n%s", out)
		}
	})
}

func TestAdvanceRestacksOnlyAffectedStack(t *testing.T) {
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
		mustRunCLI(t, cli, []string{"new", "other-root", "--parent", "main"})
		mustWriteFile(t, filepath.Join(repo, "other.txt"), "other\n")
		mustGit(t, repo, "add", "other.txt")
		mustGit(t, repo, "commit", "-m", "other root")
		mustGit(t, repo, "push", "-u", "origin", "other-root")

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
		state.Branches["other-root"].PR = &PRMeta{Number: 3, URL: "https://example.invalid/pr/3", Base: "main"}
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state: %v", err)
		}

		otherBefore, err := gitOutput("rev-parse", "other-root")
		if err != nil {
			t.Fatalf("resolve other-root before advance: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"3\" ]; then\n    cat <<'EOF'\n{\"number\":3,\"url\":\"https://example.invalid/pr/3\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"advance"})
		if code != 0 {
			t.Fatalf("advance failed: exit=%d\n%s", code, out)
		}
		if strings.Contains(out, "other-root") {
			t.Fatalf("did not expect advance output to mention unrelated root stack, got:\n%s", out)
		}

		otherAfter, err := gitOutput("rev-parse", "other-root")
		if err != nil {
			t.Fatalf("resolve other-root after advance: %v", err)
		}
		if strings.TrimSpace(otherBefore) != strings.TrimSpace(otherAfter) {
			t.Fatalf("expected unrelated root branch to remain untouched: before=%s after=%s", strings.TrimSpace(otherBefore), strings.TrimSpace(otherAfter))
		}
	})
}
