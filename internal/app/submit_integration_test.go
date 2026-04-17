package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitWithNoTrackedBranchesFails(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	mustRunCLIInRepo(t, repo, []string{"init", "--trunk", "main"})
	out, code := runCLIInRepoAndCapture(t, repo, []string{"submit", "--all"})
	if code == 0 {
		t.Fatalf("expected submit to fail with no tracked branches, output:\n%s", out)
	}
	if !strings.Contains(out, "submit requires at least one tracked branch") {
		t.Fatalf("expected tracked-branch error, got:\n%s", out)
	}
}

func TestSubmitWithoutInitializedStateFails(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	out, code := runCLIInRepoAndCapture(t, repo, []string{"submit", "--all"})
	if code == 0 {
		t.Fatalf("expected submit to fail without initialized state, output:\n%s", out)
	}
	if !strings.Contains(out, "submit requires initialized stack state") {
		t.Fatalf("expected initialized state error, got:\n%s", out)
	}
}

func TestSubmitSkipsMergedTrackedPR(t *testing.T) {
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
		mustWriteFile(t, ghPath, "#!/bin/sh\nstate_file=\"$(dirname \"$0\")/.created-pr\"\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\"}\nEOF\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  if [ -f \"$state_file\" ]; then\n    cat <<'EOF'\n[{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"feat two\",\"state\":\"OPEN\"}]\nEOF\n  else\n    printf '[]\\n'\n  fi\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  touch \"$state_file\"\n  printf 'https://example.invalid/pr/2\\n'\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
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

func TestSubmitAutoDeletesMergedBranchWhenSquashIntegratedAndRemoteIsGone(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature.txt"), "change\n")
		mustGit(t, repo, "add", "feature.txt")
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
			t.Fatalf("save state with PR metadata: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\"}\nEOF\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  printf '[]\\n'\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  printf 'https://example.invalid/pr/2\\n'\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
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
		if !strings.Contains(out, "feat-one -> deleted local merged branch") {
			t.Fatalf("expected local deletion message, got:\n%s", out)
		}
		if branchExists("feat-one") {
			t.Fatalf("expected local branch feat-one to be deleted")
		}
		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after submit: %v", err)
		}
		if _, ok := stateAfter.Branches["feat-one"]; ok {
			t.Fatalf("expected feat-one to be removed from state after deletion")
		}
	})
}

func TestSubmitUsesNextOnCleanToSwitchBeforeDeletingMergedCurrentBranch(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature.txt"), "change\n")
		mustGit(t, repo, "add", "feature.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")
		mustRunCLI(t, cli, []string{"new", "feat-two", "--parent", "feat-one"})
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
		if err := saveState(repo, state); err != nil {
			t.Fatalf("save state with PR metadata: %v", err)
		}

		fakeBin := t.TempDir()
		ghPath := filepath.Join(fakeBin, "gh")
		mustWriteFile(t, ghPath, "#!/bin/sh\nstate_file=\"$(dirname \"$0\")/.created-pr\"\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"merged\",\"state\":\"MERGED\"}\nEOF\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  if [ -f \"$state_file\" ]; then\n    cat <<'EOF'\n[{\"number\":2,\"url\":\"https://example.invalid/pr/2\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"feat two\",\"state\":\"OPEN\"}]\nEOF\n  else\n    printf '[]\\n'\n  fi\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  touch \"$state_file\"\n  printf 'https://example.invalid/pr/2\\n'\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"submit", "--next-on-clean", "feat-two", "feat-one"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "feat-one -> using --next-on-clean target feat-two before clean") {
			t.Fatalf("expected next-on-clean output, got:\n%s", out)
		}
		if !strings.Contains(out, "feat-one -> deleted local merged branch") {
			t.Fatalf("expected local deletion message, got:\n%s", out)
		}
		current, err := currentBranch()
		if err != nil {
			t.Fatalf("current branch after submit: %v", err)
		}
		if current != "feat-two" {
			t.Fatalf("expected submit to switch to feat-two, got %q", current)
		}
	})
}

func TestSubmitFailsWhenNextOnCleanTargetDoesNotExist(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature.txt"), "change\n")
		mustGit(t, repo, "add", "feature.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")
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

		out, code := runCLIAndCapture(t, cli, []string{"submit", "--next-on-clean", "missing-branch", "feat-one"})
		if code == 0 {
			t.Fatalf("expected submit to fail when next-on-clean target is missing, got:\n%s", out)
		}
		if !strings.Contains(out, "submit --next-on-clean branch does not exist locally: missing-branch") {
			t.Fatalf("expected missing-target error, got:\n%s", out)
		}
		if current, err := currentBranch(); err != nil || current != "feat-one" {
			t.Fatalf("expected to remain on feat-one after failed submit, current=%q err=%v", current, err)
		}
		if !branchExists("feat-one") {
			t.Fatal("expected feat-one to remain after failed submit")
		}
	})
}

func TestSubmitKeepsBranchWithUnmergedChanges(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature.txt"), "change\n")
		mustGit(t, repo, "add", "feature.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")
		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "push", "origin", ":feat-one")

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
		if !strings.Contains(out, "unmerged local changes detected; keeping local branch") {
			t.Fatalf("expected unmerged-changes keep message, got:\n%s", out)
		}
		if !branchExists("feat-one") {
			t.Fatalf("expected local branch feat-one to be preserved")
		}
		stateAfter, err := loadState(repo)
		if err != nil {
			t.Fatalf("load state after submit: %v", err)
		}
		if _, ok := stateAfter.Branches["feat-one"]; !ok {
			t.Fatalf("expected feat-one to remain in state when not fully integrated")
		}
	})
}

func TestSubmitForcePushesWithLeaseAfterHistoryRewrite(t *testing.T) {
	repo := newTestRepo(t)
	origin := newBareOrigin(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustPointRepoOriginAndTrack(t, repo, origin, "main")

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		mustRunCLI(t, cli, []string{"new", "feat-one"})
		mustWriteFile(t, filepath.Join(repo, "feature.txt"), "first\n")
		mustGit(t, repo, "add", "feature.txt")
		mustGit(t, repo, "commit", "-m", "feat one initial")
		mustGit(t, repo, "push", "-u", "origin", "feat-one")

		mustGit(t, repo, "reset", "--hard", "HEAD~1")
		mustWriteFile(t, filepath.Join(repo, "feature.txt"), "rewritten\n")
		mustGit(t, repo, "add", "feature.txt")
		mustGit(t, repo, "commit", "-m", "feat one rewritten")

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
		mustWriteFile(t, ghPath, "#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"number\":1,\"url\":\"https://example.invalid/pr/1\",\"body\":\"\",\"baseRefName\":\"main\",\"title\":\"open\",\"state\":\"OPEN\"}\nEOF\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"edit\" ]; then\n  exit 0\nfi\necho \"unexpected gh args: $*\" >&2\nexit 1\n")
		if err := os.Chmod(ghPath, 0o755); err != nil {
			t.Fatalf("chmod fake gh: %v", err)
		}
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

		out, code := runCLIAndCapture(t, cli, []string{"submit", "feat-one"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if strings.Contains(out, "failed to push some refs") {
			t.Fatalf("expected single force-with-lease push without retry noise, got:\n%s", out)
		}

		localHead, err := gitOutput("rev-parse", "feat-one")
		if err != nil {
			t.Fatalf("local rev-parse feat-one: %v", err)
		}
		remoteHead, err := gitOutput("ls-remote", "--heads", "origin", "feat-one")
		if err != nil {
			t.Fatalf("ls-remote feat-one: %v", err)
		}
		fields := strings.Fields(strings.TrimSpace(remoteHead))
		if len(fields) == 0 {
			t.Fatalf("expected remote feat-one ref after submit")
		}
		if strings.TrimSpace(localHead) != fields[0] {
			t.Fatalf("expected remote head to match local rewritten commit; local=%s remote=%s", strings.TrimSpace(localHead), fields[0])
		}
	})
}
