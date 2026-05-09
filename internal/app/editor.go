package app

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

func editFileInGitEditor(path string, stdin io.Reader, stdout, stderr io.Writer) error {
	editor, err := gitOutputTrimmed("var", "GIT_EDITOR")
	if err != nil {
		return fmt.Errorf("determine git editor: %w", err)
	}
	editor = strings.TrimSpace(editor)
	if editor == "" {
		return fmt.Errorf("determine git editor: resolved editor is empty")
	}

	name, args := editorCommand(editor, path)
	cmd := exec.Command(name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open editor: %w", err)
	}
	return nil
}

func editorCommand(editor, path string) (string, []string) {
	quotedPath := shellQuotePath(path)
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", editor + " " + quotedPath}
	}
	return "sh", []string{"-c", "exec " + editor + " " + quotedPath}
}

func shellQuotePath(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
