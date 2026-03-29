package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitWithNoTrackedBranchesIsNoop(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		mustRunCLI(t, cli, []string{"init", "--trunk", "main"})
		out, code := runCLIAndCapture(t, cli, []string{"submit", "--all"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "nothing to submit") {
			t.Fatalf("expected noop submit message, got:\n%s", out)
		}
	})
}

func TestSubmitWithoutInitializedStateIsNoop(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()

		out, code := runCLIAndCapture(t, cli, []string{"submit", "--all"})
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "nothing to submit") {
			t.Fatalf("expected noop submit message, got:\n%s", out)
		}
	})
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

func TestSubmitKeepsBranchWithUnmergedChangesEvenWithYes(t *testing.T) {
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

		out, code := runCLIAndCapture(t, cli, []string{"submit", "--yes", "feat-one"})
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

func TestSubmitPromptsToSwitchAndDeleteWhenMergedBranchIsCheckedOut(t *testing.T) {
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
		mustRunCLI(t, cli, []string{"new", "feat-two", "--parent", "feat-one"})
		mustGit(t, repo, "switch", "feat-one")
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

		out, code := runCLIWithInputAndCapture(t, cli, []string{"submit", "feat-one"}, "y\n")
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "Switch to feat-two and delete this branch? [y/N]") {
			t.Fatalf("expected switch prompt, got:\n%s", out)
		}
		if !strings.Contains(out, "feat-one -> deleted local merged branch") {
			t.Fatalf("expected local deletion message, got:\n%s", out)
		}
		if branchExists("feat-one") {
			t.Fatalf("expected local branch feat-one to be deleted")
		}
		cur, err := currentBranch()
		if err != nil {
			t.Fatalf("current branch after submit: %v", err)
		}
		if cur != "feat-two" {
			t.Fatalf("expected branch switch to feat-two, got %s", cur)
		}
	})
}

func TestSubmitSwitchesToTrunkAndDeletesWhenCheckedOutMergedBranchHasNoChildren(t *testing.T) {
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
		mustGit(t, repo, "switch", "feat-one")
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
		if !strings.Contains(out, "feat-one -> merged and remote deleted; switching to main before cleanup") {
			t.Fatalf("expected trunk switch cleanup message, got:\n%s", out)
		}
		if !strings.Contains(out, "feat-one -> deleted local merged branch") {
			t.Fatalf("expected local deletion message, got:\n%s", out)
		}
		if branchExists("feat-one") {
			t.Fatalf("expected local branch feat-one to be deleted")
		}
		cur, err := currentBranch()
		if err != nil {
			t.Fatalf("current branch after submit: %v", err)
		}
		if cur != "main" {
			t.Fatalf("expected branch switch to main, got %s", cur)
		}
	})
}

func TestSubmitOffersChildChoiceWhenCheckedOutMergedBranchHasMultipleChildren(t *testing.T) {
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
		mustRunCLI(t, cli, []string{"new", "feat-a", "--parent", "feat-one"})
		mustGit(t, repo, "switch", "feat-one")
		mustRunCLI(t, cli, []string{"new", "feat-b", "--parent", "feat-one"})
		mustGit(t, repo, "switch", "feat-one")
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

		out, code := runCLIWithInputAndCapture(t, cli, []string{"submit", "feat-one"}, "2\n")
		if code != 0 {
			t.Fatalf("submit failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "Choose branch to switch to before deleting it") {
			t.Fatalf("expected child selection prompt, got:\n%s", out)
		}
		if !strings.Contains(out, "1) feat-a") || !strings.Contains(out, "2) feat-b") {
			t.Fatalf("expected sorted child options, got:\n%s", out)
		}
		if branchExists("feat-one") {
			t.Fatalf("expected local branch feat-one to be deleted")
		}
		cur, err := currentBranch()
		if err != nil {
			t.Fatalf("current branch after submit: %v", err)
		}
		if cur != "feat-b" {
			t.Fatalf("expected branch switch to feat-b, got %s", cur)
		}
	})
}
