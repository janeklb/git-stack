package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTemplatePRDefaultsToRepoScopeAndSeedsTemplate(t *testing.T) {
	repo := newTestRepo(t)
	fakeBin := t.TempDir()
	logPath := filepath.Join(fakeBin, "editor.log")
	editorPath := filepath.Join(fakeBin, "editor")
	mustWriteFile(t, editorPath, "#!/bin/sh\nprintf '%s\\n' \"$1\" > \"$TEMPLATE_LOG\"\nprintf '\\nEdited by fake editor\\n' >> \"$1\"\n")
	if err := os.Chmod(editorPath, 0o755); err != nil {
		t.Fatalf("chmod fake editor: %v", err)
	}

	env := append(envWithPathPrepended(fakeBin), "GIT_EDITOR="+editorPath, "TEMPLATE_LOG="+logPath)
	out, code := runCLIInRepoAndCaptureWithEnv(t, repo, env, []string{"template", "pr"})
	if code != 0 {
		t.Fatalf("template pr failed: exit=%d\n%s", code, out)
	}
	if !strings.Contains(out, "opening repo PR template:") {
		t.Fatalf("expected template command output, got:\n%s", out)
	}

	templatePath := filepath.Join(repo, ".git", "stack", "PR_TEMPLATE.md")
	loggedPath, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read editor log: %v", err)
	}
	gotPath := strings.TrimSpace(string(loggedPath))
	expectedPath, err := filepath.EvalSymlinks(templatePath)
	if err != nil {
		t.Fatalf("eval expected repo template path: %v", err)
	}
	gotPath, err = filepath.EvalSymlinks(gotPath)
	if err != nil {
		t.Fatalf("eval logged repo template path: %v", err)
	}
	if gotPath != expectedPath {
		t.Fatalf("expected editor to open repo template %q, got %q", expectedPath, gotPath)
	}
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read repo template: %v", err)
	}
	if !strings.Contains(string(templateData), "## Summary") {
		t.Fatalf("expected seeded default template content, got:\n%s", string(templateData))
	}
	if !strings.Contains(string(templateData), "Edited by fake editor") {
		t.Fatalf("expected editor marker in template, got:\n%s", string(templateData))
	}
}

func TestTemplatePRUserScopeWorksOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	fakeBin := t.TempDir()
	logPath := filepath.Join(fakeBin, "editor.log")
	editorPath := filepath.Join(fakeBin, "editor")
	mustWriteFile(t, editorPath, "#!/bin/sh\nprintf '%s\\n' \"$1\" > \"$TEMPLATE_LOG\"\nprintf '\\nEdited by fake editor\\n' >> \"$1\"\n")
	if err := os.Chmod(editorPath, 0o755); err != nil {
		t.Fatalf("chmod fake editor: %v", err)
	}

	cmd := exec.Command(testCLIBinary, "template", "pr", "--scope", "user")
	cmd.Dir = dir
	cmd.Env = append(envWithPathPrepended(fakeBin), "HOME="+home, "XDG_CONFIG_HOME="+xdg, "GIT_EDITOR="+editorPath, "TEMPLATE_LOG="+logPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("template pr --scope user failed: exit=%d\n%s", exitErr.ExitCode(), string(out))
		}
		t.Fatalf("run template pr --scope user: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "opening user PR template:") {
		t.Fatalf("expected template command output, got:\n%s", string(out))
	}

	userConfigDir := filepath.Join(xdg, "git-stack")
	if runtime.GOOS == "darwin" {
		userConfigDir = filepath.Join(home, "Library", "Application Support", "git-stack")
	}
	templatePath := filepath.Join(userConfigDir, "PR_TEMPLATE.md")
	loggedPath, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read editor log: %v", err)
	}
	if strings.TrimSpace(string(loggedPath)) != templatePath {
		t.Fatalf("expected editor to open user template %q, got %q", templatePath, strings.TrimSpace(string(loggedPath)))
	}
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read user template: %v", err)
	}
	if !strings.Contains(string(templateData), "## Summary") {
		t.Fatalf("expected seeded default template content, got:\n%s", string(templateData))
	}
	if !strings.Contains(string(templateData), "Edited by fake editor") {
		t.Fatalf("expected editor marker in template, got:\n%s", string(templateData))
	}
}
