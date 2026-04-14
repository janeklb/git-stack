package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForwardSingleChildRunsCleanupRestackAndSubmit(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--no-ff", "feat-one", "-m", "merge feat one")
	mergedMain := mustGitOutput(t, repo, "rev-parse", "HEAD")
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
	mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergedMain)+"\"}}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
	if err := os.Chmod(ghPath, 0o755); err != nil {
		t.Fatalf("chmod fake gh: %v", err)
	}

	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "forward completed") {
		t.Fatalf("expected forward completion output, got:\n%s", out)
	}

	if branchExistsInRepo(repo, "feat-one") {
		t.Fatalf("expected feat-one local branch to be deleted")
	}
	cur := currentBranchInRepo(t, repo)
	if cur != "feat-two" {
		t.Fatalf("expected forward to switch to single child feat-two, got %s", cur)
	}

	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after forward: %v", err)
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
}

func TestForwardMarksAllPromotedRootPRsReadyForReview(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	mustGit(t, repo, "switch", "feat-one")
	mustRunCLIInRepo(t, repo, []string{"new", "feat-three", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "three.txt"), "three\n")
	mustGit(t, repo, "add", "three.txt")
	mustGit(t, repo, "commit", "-m", "feat three")
	mustGit(t, repo, "push", "-u", "origin", "feat-three")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--no-ff", "feat-one", "-m", "merge feat one")
	mergedMain := mustGitOutput(t, repo, "rev-parse", "HEAD")
	mustGit(t, repo, "push", "origin", "main")
	mustGit(t, repo, "push", "origin", ":feat-one")
	mustGit(t, repo, "switch", "feat-one")

	state, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.Branches["feat-one"].PR = &PRMeta{Number: 1, URL: "https://example.invalid/pr/1", Base: "main"}
	state.Branches["feat-two"].PR = &PRMeta{Number: 2, URL: "https://example.invalid/pr/2", Base: "feat-one"}
	state.Branches["feat-three"].PR = &PRMeta{Number: 3, URL: "https://example.invalid/pr/3", Base: "feat-one"}
	if err := saveState(repo, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "gh.log")
	fakeBin := t.TempDir()
	ghPath := filepath.Join(fakeBin, "gh")
	mustWriteFile(t, ghPath, "#!/bin/sh\nLOG=\""+logPath+"\"\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"isDraft\":false,\"headRefOid\":\"\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergedMain)+"\"}}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open two\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"3\" ]; then\n    cat <<'EOF'\n{\"number\":3,\"url\":\"https://example.invalid/pr/3\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open three\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  printf '%s\\n' \"$*\" >> \"$LOG\"\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"ready\" ]; then\n  printf '%s\\n' \"$*\" >> \"$LOG\"\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
	if err := os.Chmod(ghPath, 0o755); err != nil {
		t.Fatalf("chmod fake gh: %v", err)
	}

	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward", "--next", "feat-two"})
	if code != 0 {
		t.Fatalf("forward failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "forward completed") {
		t.Fatalf("expected forward completion output, got:\n%s", out)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gh log: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "pr ready 2") {
		t.Fatalf("expected feat-two to be marked ready, log:\n%s", logText)
	}
	if !strings.Contains(logText, "pr ready 3") {
		t.Fatalf("expected feat-three to be marked ready, log:\n%s", logText)
	}
	if !strings.Contains(logText, "pr edit 2 --base main") {
		t.Fatalf("expected feat-two base updated to main, log:\n%s", logText)
	}
	if !strings.Contains(logText, "pr edit 3 --base main") {
		t.Fatalf("expected feat-three base updated to main, log:\n%s", logText)
	}

	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after forward: %v", err)
	}
	if got := stateAfter.Branches["feat-two"].Parent; got != "main" {
		t.Fatalf("expected feat-two parent reparented to main, got %q", got)
	}
	if got := stateAfter.Branches["feat-three"].Parent; got != "main" {
		t.Fatalf("expected feat-three parent reparented to main, got %q", got)
	}
}

func TestForwardFromOpenChildCleansMergedAncestorAndRestoresCurrentBranch(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--no-ff", "feat-one", "-m", "merge feat one")
	mergedMain := mustGitOutput(t, repo, "rev-parse", "HEAD")
	mustGit(t, repo, "push", "origin", "main")
	mustGit(t, repo, "push", "origin", ":feat-one")
	mustGit(t, repo, "switch", "feat-two")

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
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "forward completed") {
		t.Fatalf("expected forward completion output, got:\n%s", out)
	}

	if branchExistsInRepo(repo, "feat-one") {
		t.Fatalf("expected feat-one local branch to be deleted")
	}
	cur := currentBranchInRepo(t, repo)
	if cur != "feat-two" {
		t.Fatalf("expected forward to restore feat-two, got %s", cur)
	}

	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after forward: %v", err)
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
}

func TestForwardNoopsWhenCurrentStackHasNoMergedBranches(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

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
	mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
	if err := os.Chmod(ghPath, 0o755); err != nil {
		t.Fatalf("chmod fake gh: %v", err)
	}
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "forward: nothing to do") {
		t.Fatalf("expected no-op output, got:\n%s", out)
	}

	cur := currentBranchInRepo(t, repo)
	if cur != "feat-two" {
		t.Fatalf("expected forward no-op to keep feat-two checked out, got %s", cur)
	}

	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after forward: %v", err)
	}
	if got := stateAfter.Branches["feat-two"].Parent; got != "feat-one" {
		t.Fatalf("expected feat-two parent unchanged, got %q", got)
	}
}

func TestForwardAbortsWhenRemoteBranchStillExists(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
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
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code == 0 {
		t.Fatalf("expected forward to fail when remote still exists, output:\n%s", out)
	}
	if !strings.Contains(out, "origin/feat-one still exists") {
		t.Fatalf("expected remote-exists guidance, got:\n%s", out)
	}

	if !branchExistsInRepo(repo, "feat-one") {
		t.Fatalf("expected feat-one branch to remain after abort")
	}
	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after abort: %v", err)
	}
	if _, ok := stateAfter.Branches["feat-one"]; !ok {
		t.Fatalf("expected feat-one to remain tracked after abort")
	}
}

func TestForwardDoesNotCleanupUnrelatedMergedTrackedBranches(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "old-merged"})
	mustWriteFile(t, filepath.Join(repo, "old.txt"), "old\n")
	mustGit(t, repo, "add", "old.txt")
	mustGit(t, repo, "commit", "-m", "old merged")
	mustGit(t, repo, "push", "-u", "origin", "old-merged")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--squash", "old-merged")
	mustGit(t, repo, "commit", "-m", "squash old merged")
	mustGit(t, repo, "push", "origin", "main")
	mustGit(t, repo, "push", "origin", ":old-merged")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one", "--parent", "main"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
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
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed: exit=%d\n%s", code, out)
	}

	if !branchExistsInRepo(repo, "old-merged") {
		t.Fatalf("expected unrelated merged branch to remain after forward")
	}
	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after forward: %v", err)
	}
	if _, ok := stateAfter.Branches["old-merged"]; !ok {
		t.Fatalf("expected unrelated merged branch to remain tracked after forward")
	}
}

func TestForwardUsesFetchedRemoteTrunkWhenLocalTrunkIsStale(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--squash", "feat-one")
	mustGit(t, repo, "commit", "-m", "squash feat one")
	mergedMain := mustGitOutput(t, repo, "rev-parse", "HEAD")
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
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed with stale local trunk: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "forward completed") {
		t.Fatalf("expected forward completion output, got:\n%s", out)
	}
	remaining := mustGitOutput(t, repo, "log", "--format=%s", "origin/main..feat-two")
	trimmed := strings.TrimSpace(remaining)
	if trimmed != "feat two" {
		t.Fatalf("expected feat-two to restack onto fetched trunk without carrying merged parent commits, got:\n%s", trimmed)
	}
}

func TestForwardFastForwardsLocalTrunkForMergedLastSlice(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--squash", "feat-one")
	mustGit(t, repo, "commit", "-m", "squash feat one")
	mergedMain := mustGitOutput(t, repo, "rev-parse", "HEAD")
	mustGit(t, repo, "push", "origin", "main")
	mustGit(t, repo, "push", "origin", ":feat-one")
	mustGit(t, repo, "reset", "--hard", "HEAD~1")
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
	mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":{\"oid\":\""+strings.TrimSpace(mergedMain)+"\"}}\nEOF\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
	if err := os.Chmod(ghPath, 0o755); err != nil {
		t.Fatalf("chmod fake gh: %v", err)
	}
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed with stale local trunk: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "forward completed") {
		t.Fatalf("expected forward completion output, got:\n%s", out)
	}

	cur := currentBranchInRepo(t, repo)
	if cur != "main" {
		t.Fatalf("expected forward to switch to main after last-slice clean, got %s", cur)
	}
	localMain := mustGitOutput(t, repo, "rev-parse", "main")
	if strings.TrimSpace(localMain) != strings.TrimSpace(mergedMain) {
		t.Fatalf("expected local main fast-forwarded to fetched origin/main, got %q want %q", strings.TrimSpace(localMain), strings.TrimSpace(mergedMain))
	}
}

func TestForwardAbortsBeforeCleanupWhenLocalTrunkDiverged(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	mustGit(t, repo, "switch", "main")
	mustGit(t, repo, "merge", "--squash", "feat-one")
	mustGit(t, repo, "commit", "-m", "squash feat one")
	mustGit(t, repo, "push", "origin", "main")
	mustWriteFile(t, filepath.Join(repo, "local-main.txt"), "local\n")
	mustGit(t, repo, "add", "local-main.txt")
	mustGit(t, repo, "commit", "-m", "local main diverged")
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
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code == 0 {
		t.Fatalf("expected forward to fail when local trunk diverged, output:\n%s", out)
	}
	if !strings.Contains(out, "local trunk main has diverged from fetched origin/main") {
		t.Fatalf("expected diverged-trunk guidance, got:\n%s", out)
	}
	if !branchExistsInRepo(repo, "feat-one") {
		t.Fatalf("expected feat-one to remain after abort")
	}
	stateAfter, err := loadState(repo)
	if err != nil {
		t.Fatalf("load state after abort: %v", err)
	}
	if _, ok := stateAfter.Branches["feat-one"]; !ok {
		t.Fatalf("expected feat-one to remain tracked after diverged-trunk abort")
	}
	if got := stateAfter.Branches["feat-two"].Parent; got != "feat-one" {
		t.Fatalf("expected feat-two parent to remain feat-one after abort, got %q", got)
	}
}

func TestForwardRestacksOnlyAffectedStack(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	mustPointRepoOriginAndTrack(t, repo, origin, "main")
	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})

	mustRunCLIInRepo(t, repo, []string{"new", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "one.txt"), "one\n")
	mustGit(t, repo, "add", "one.txt")
	mustGit(t, repo, "commit", "-m", "feat one")
	mustGit(t, repo, "push", "-u", "origin", "feat-one")

	mustRunCLIInRepo(t, repo, []string{"new", "feat-two", "--parent", "feat-one"})
	mustWriteFile(t, filepath.Join(repo, "two.txt"), "two\n")
	mustGit(t, repo, "add", "two.txt")
	mustGit(t, repo, "commit", "-m", "feat two")
	mustGit(t, repo, "push", "-u", "origin", "feat-two")

	mustGit(t, repo, "switch", "main")
	mustRunCLIInRepo(t, repo, []string{"new", "other-root", "--parent", "main"})
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

	otherBefore := mustGitOutput(t, repo, "rev-parse", "other-root")

	fakeBin := t.TempDir()
	ghPath := filepath.Join(fakeBin, "gh")
	mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  if [ \"$3\" = \"1\" ]; then\n    cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"2\" ]; then\n    cat <<'EOF'\n{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"feat-one\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\n  if [ \"$3\" = \"3\" ]; then\n    cat <<'EOF'\n{\"number\":3,\"url\":\"https://example.invalid/pr/3\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"open\",\"state\":\"OPEN\",\"headRefOid\":\"\",\"mergeCommit\":null}\nEOF\n    exit 0\n  fi\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
	if err := os.Chmod(ghPath, 0o755); err != nil {
		t.Fatalf("chmod fake gh: %v", err)
	}
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, envWithPathPrepended(fakeBin), []string{"forward"})
	if code != 0 {
		t.Fatalf("forward failed: exit=%d\n%s", code, out)
	}
	if strings.Contains(out, "other-root") {
		t.Fatalf("did not expect forward output to mention unrelated root stack, got:\n%s", out)
	}

	otherAfter := mustGitOutput(t, repo, "rev-parse", "other-root")
	if strings.TrimSpace(otherBefore) != strings.TrimSpace(otherAfter) {
		t.Fatalf("expected unrelated root branch to remain untouched: before=%s after=%s", strings.TrimSpace(otherBefore), strings.TrimSpace(otherAfter))
	}
}
