package app

import (
	"fmt"
	"runtime"
	"strings"
)

func editFileInGitEditor(path string, runner commandRunner, opts commandRunOptions) error {
	query := opts
	query.stdin = nil
	query.boxMode = commandBoxOnFailure
	result, err := runner("git", []string{"var", "GIT_EDITOR"}, query)
	if err != nil {
		return fmt.Errorf("determine git editor: %w", err)
	}
	editor := strings.TrimSpace(result.stdout)
	if editor == "" {
		return fmt.Errorf("determine git editor: resolved editor is empty")
	}

	name, args := editorCommand(editor, path)
	opts.interactive = true
	if _, err := runner(name, args, opts); err != nil {
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
