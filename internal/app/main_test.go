package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "stack-git-config-")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(tmp, "global.gitconfig")
	if err := os.WriteFile(global, []byte{}, 0o600); err != nil {
		panic(err)
	}

	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	_ = os.Setenv("GIT_CONFIG_GLOBAL", global)
	_ = os.Setenv("GIT_TERMINAL_PROMPT", "0")

	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
