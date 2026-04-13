package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testCLIBinary string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "stack-git-config-")
	if err != nil {
		panic(err)
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repoRoot = filepath.Clean(filepath.Join(repoRoot, "..", ".."))
	global := filepath.Join(tmp, "global.gitconfig")
	if err := os.WriteFile(global, []byte{}, 0o600); err != nil {
		panic(err)
	}
	testCLIBinary = filepath.Join(tmp, "git-stack-test")
	build := exec.Command("go", "build", "-o", testCLIBinary, "./cmd/git-stack")
	build.Dir = repoRoot
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}

	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	_ = os.Setenv("GIT_CONFIG_GLOBAL", global)
	_ = os.Setenv("GIT_TERMINAL_PROMPT", "0")

	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
