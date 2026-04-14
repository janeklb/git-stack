package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpIncludesCompletionCommand(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help"})
	if code != 0 {
		t.Fatalf("help failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "completion") {
		t.Fatalf("expected help output to mention completion command, got:\n%s", out)
	}
	if !strings.Contains(out, "git-stack is an opinionated CLI for personal stacked PR development") {
		t.Fatalf("expected root help to include updated long description, got:\n%s", out)
	}
	if !strings.Contains(out, "Manage personal stacked PR branches") {
		t.Fatalf("expected root help to include short summary, got:\n%s", out)
	}
}

func TestBareRootOutputOmitsRootLongDescription(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, nil)
	if code != 0 {
		t.Fatalf("bare root failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "Manage personal stacked PR branches") {
		t.Fatalf("expected bare root to include short summary, got:\n%s", out)
	}
	if strings.Contains(out, "git-stack is an opinionated CLI for personal stacked PR development") {
		t.Fatalf("expected bare root to omit long description, got:\n%s", out)
	}
}

func TestCompletionBashOutputsScript(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"completion", "bash"})
	if code != 0 {
		t.Fatalf("completion bash failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "__start_git-stack") {
		t.Fatalf("expected bash completion function in output, got:\n%s", out)
	}
}

func TestKeyCommandFlagsExist(t *testing.T) {
	root := New().newRootCmd("git-stack")
	advance, _, err := root.Find([]string{"advance"})
	if err != nil {
		t.Fatalf("find advance command: %v", err)
	}
	if advance.Flags().Lookup("next") == nil {
		t.Fatalf("expected advance next flag to exist")
	}
	submit, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit command: %v", err)
	}
	if submit.Flags().Lookup("next-on-clean") == nil {
		t.Fatalf("expected submit next-on-clean flag to exist")
	}
	clean, _, err := root.Find([]string{"clean"})
	if err != nil {
		t.Fatalf("find clean command: %v", err)
	}
	if clean.Flags().Lookup("yes") == nil {
		t.Fatalf("expected clean yes flag to exist")
	}
	if clean.Flags().Lookup("all") == nil {
		t.Fatalf("expected clean all flag to exist")
	}
	if clean.Flags().Lookup("include-squash") == nil {
		t.Fatalf("expected clean include-squash flag to exist")
	}
	if clean.Flags().Lookup("untracked") == nil {
		t.Fatalf("expected clean untracked flag to exist")
	}
	reparent, _, err := root.Find([]string{"reparent"})
	if err != nil {
		t.Fatalf("find reparent command: %v", err)
	}
	if reparent.Flags().Lookup("preserve-lineage") == nil {
		t.Fatalf("expected reparent preserve-lineage flag to exist")
	}
	newCmd, _, err := root.Find([]string{"new"})
	if err != nil {
		t.Fatalf("find new command: %v", err)
	}
	if newCmd.Flags().Lookup("adopt") == nil {
		t.Fatalf("expected new adopt flag to exist")
	}
}

func TestStateHasStAlias(t *testing.T) {
	root := New().newRootCmd("git-stack")
	cmd, _, err := root.Find([]string{"st"})
	if err != nil {
		t.Fatalf("find st command: %v", err)
	}
	if cmd.Name() != "state" {
		t.Fatalf("expected alias to resolve to state, got %q", cmd.Name())
	}
}

func TestInitHelpMarksCommandAsRepairFlow(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help", "init"})
	if code != 0 {
		t.Fatalf("help init failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "Initialize or repair persisted stack state") {
		t.Fatalf("expected init help to mention repair/config role, got:\n%s", out)
	}
	if !strings.Contains(out, "auto-bootstrap state when possible") {
		t.Fatalf("expected init help to mention auto-bootstrap happy path, got:\n%s", out)
	}
	if !strings.Contains(out, "supports {slug} and {n}") {
		t.Fatalf("expected init help to describe naming template placeholders, got:\n%s", out)
	}
	if !strings.Contains(out, "Initialize or repair stack state") {
		t.Fatalf("expected init help to include short summary, got:\n%s", out)
	}
}

func TestSubmitHelpDescribesDefaultsAndCleanFlag(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help", "submit"})
	if code != 0 {
		t.Fatalf("help submit failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "By default, submit operates on the current stack component in topological order") {
		t.Fatalf("expected submit help to describe default scope and ordering, got:\n%s", out)
	}
	if !strings.Contains(out, "force-pushes the local branch to origin with force-with-lease") {
		t.Fatalf("expected submit help to describe push semantics, got:\n%s", out)
	}
	if !strings.Contains(out, "git-stack submit --next-on-clean feat/two feat/one") {
		t.Fatalf("expected submit help to include clean example, got:\n%s", out)
	}
}

func TestAdvanceHelpDescribesStrictPostMergeFlow(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help", "advance"})
	if code != 0 {
		t.Fatalf("help advance failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "advance is a strict post-merge workflow") {
		t.Fatalf("expected advance help to describe strict post-merge behavior, got:\n%s", out)
	}
	if !strings.Contains(out, "whose remote branches have already been deleted") {
		t.Fatalf("expected advance help to describe remote deletion precondition, got:\n%s", out)
	}
}

func TestCleanHelpDescribesPlanAndScope(t *testing.T) {
	cli := New()
	out, code := runCLIAndCapture(t, cli, []string{"help", "clean"})
	if code != 0 {
		t.Fatalf("help clean failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "builds a clean plan, prints that plan") {
		t.Fatalf("expected clean help to describe plan output, got:\n%s", out)
	}
	if !strings.Contains(out, "By default it only considers tracked branches in the current stack component") {
		t.Fatalf("expected clean help to describe default scope, got:\n%s", out)
	}
}

func TestReparentPositionalCompletionListsLocalBranches(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustGit(t, repo, "switch", "-c", "feat-one")
		mustWriteFile(t, filepath.Join(repo, "feature1.txt"), "one\n")
		mustGit(t, repo, "add", "feature1.txt")
		mustGit(t, repo, "commit", "-m", "feat one")
		mustGit(t, repo, "switch", "main")

		out, code := runCLIAndCapture(t, cli, []string{"__complete", "reparent", "fe"})
		if code != 0 {
			t.Fatalf("reparent completion failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "feat-one") {
			t.Fatalf("expected reparent completion to include feat-one, got:\n%s", out)
		}
		if strings.Contains(out, "README.md") {
			t.Fatalf("expected reparent completion to suppress file suggestions, got:\n%s", out)
		}
	})
}

func TestReparentParentFlagCompletionIncludesOriginBranches(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustGit(t, repo, "switch", "-c", "remote-parent")
		mustWriteFile(t, filepath.Join(repo, "remote.txt"), "remote\n")
		mustGit(t, repo, "add", "remote.txt")
		mustGit(t, repo, "commit", "-m", "remote parent")
		mustGit(t, repo, "push", "-u", "origin", "remote-parent")
		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "branch", "-D", "remote-parent")

		out, code := runCLIAndCapture(t, cli, []string{"__complete", "reparent", "feat-one", "--parent", "rem"})
		if code != 0 {
			t.Fatalf("reparent parent completion failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "remote-parent") {
			t.Fatalf("expected reparent --parent completion to include remote-parent, got:\n%s", out)
		}
	})
}

func TestSubmitPositionalCompletionListsLocalBranches(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustGit(t, repo, "switch", "-c", "feat-submit")
		mustWriteFile(t, filepath.Join(repo, "submit.txt"), "submit\n")
		mustGit(t, repo, "add", "submit.txt")
		mustGit(t, repo, "commit", "-m", "submit branch")
		mustGit(t, repo, "switch", "main")

		out, code := runCLIAndCapture(t, cli, []string{"__complete", "submit", "feat-s"})
		if code != 0 {
			t.Fatalf("submit completion failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "feat-submit") {
			t.Fatalf("expected submit completion to include feat-submit, got:\n%s", out)
		}
	})
}

func TestAdvanceNextFlagCompletionListsOnlyLocalBranches(t *testing.T) {
	repo := newTestRepo(t)

	withRepoCwd(t, repo, func() {
		cli := New()
		mustGit(t, repo, "switch", "-c", "local-next")
		mustWriteFile(t, filepath.Join(repo, "local.txt"), "local\n")
		mustGit(t, repo, "add", "local.txt")
		mustGit(t, repo, "commit", "-m", "local next")
		mustGit(t, repo, "push", "-u", "origin", "local-next")
		mustGit(t, repo, "switch", "-c", "remote-next")
		mustWriteFile(t, filepath.Join(repo, "remote-next.txt"), "remote\n")
		mustGit(t, repo, "add", "remote-next.txt")
		mustGit(t, repo, "commit", "-m", "remote next")
		mustGit(t, repo, "push", "-u", "origin", "remote-next")
		mustGit(t, repo, "switch", "main")
		mustGit(t, repo, "branch", "-D", "remote-next")

		out, code := runCLIAndCapture(t, cli, []string{"__complete", "advance", "--next", ""})
		if code != 0 {
			t.Fatalf("advance next completion failed: exit=%d\n%s", code, out)
		}
		if !strings.Contains(out, "local-next") {
			t.Fatalf("expected advance --next completion to include local-next, got:\n%s", out)
		}
		if strings.Contains(out, "remote-next") {
			t.Fatalf("expected advance --next completion to exclude remote-only branches, got:\n%s", out)
		}
	})
}
