package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPRTemplatePrefersRepoTemplateOverUserTemplate(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	repoPath := repoPRTemplatePath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatalf("mkdir repo template dir: %v", err)
	}
	if err := os.WriteFile(repoPath, []byte("repo template\n"), 0o600); err != nil {
		t.Fatalf("write repo template: %v", err)
	}

	userPath, err := userPRTemplatePath()
	if err != nil {
		t.Fatalf("resolve user template path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatalf("mkdir user template dir: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("user template\n"), 0o600); err != nil {
		t.Fatalf("write user template: %v", err)
	}

	template, ok, err := loadPRTemplate(repoRoot)
	if err != nil {
		t.Fatalf("loadPRTemplate returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected custom template to be found")
	}
	if template != "repo template\n" {
		t.Fatalf("expected repo template, got %q", template)
	}
}

func TestLoadPRTemplateUsesUserTemplateWhenRepoTemplateMissing(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	userPath, err := userPRTemplatePath()
	if err != nil {
		t.Fatalf("resolve user template path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatalf("mkdir user template dir: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("user template\n"), 0o600); err != nil {
		t.Fatalf("write user template: %v", err)
	}

	template, ok, err := loadPRTemplate(repoRoot)
	if err != nil {
		t.Fatalf("loadPRTemplate returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected custom template to be found")
	}
	if template != "user template\n" {
		t.Fatalf("expected user template, got %q", template)
	}
}

func TestLoadPRTemplateFallsBackWhenNoCustomTemplateExists(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	template, ok, err := loadPRTemplate(repoRoot)
	if err != nil {
		t.Fatalf("loadPRTemplate returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected no custom template, got %q", template)
	}
	if template != "" {
		t.Fatalf("expected empty template when none found, got %q", template)
	}
}
