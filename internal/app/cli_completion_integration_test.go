package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashGitSubcommandWrapperCompletesRootCommands(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	cli := New()
	script, code := runCLIAndCapture(t, cli, []string{"completion", "bash"})
	if code != 0 {
		t.Fatalf("completion bash failed: exit=%d\n%s", code, script)
	}

	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "git-stack-completion.bash")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write completion script: %v", err)
	}
	if err := os.Symlink(testCLIBinary, filepath.Join(tmp, canonicalBinName)); err != nil {
		t.Fatalf("symlink test binary: %v", err)
	}

	cmd := exec.Command("bash", "-lc", "compopt() { return 0; }; source \"$1\"; words=(git stack su); cword=2; cur=su; prev=stack; _git_stack; printf '%s\n' \"${COMPREPLY[@]}\"", "bash", scriptPath)
	cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash completion smoke test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "submit") {
		t.Fatalf("expected git stack bash completion to include submit, got:\n%s", out)
	}
}
