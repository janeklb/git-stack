package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateNormalizesWhitespace(t *testing.T) {
	repo := newTestRepo(t)
	state := &State{
		Version:     stateVersion,
		Trunk:       "  main  ",
		RestackMode: "  rebase  ",
		Naming: NamingConfig{
			Template:  "  {slug}  ",
			NextIndex: 2,
		},
		Clean: CleanConfig{MergeDetection: "  strict  "},
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "  main  ", LineageParent: "   ", PR: &PRMeta{Number: 1, URL: "  https://example.invalid/pr/1  ", Base: "  main  "}},
		},
		Archived: map[string]*ArchivedRef{
			"old-one": {Parent: "  main  ", PR: &PRMeta{Number: 2, URL: "  https://example.invalid/pr/2  ", Base: "  main  "}},
		},
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	path := filepath.Join(repo, ".git", "stack", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	loaded, err := loadState(repo)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if loaded.Trunk != "main" {
		t.Fatalf("expected trimmed trunk, got %q", loaded.Trunk)
	}
	if loaded.RestackMode != "rebase" {
		t.Fatalf("expected trimmed restack mode, got %q", loaded.RestackMode)
	}
	if loaded.Naming.Template != "{slug}" {
		t.Fatalf("expected trimmed naming template, got %q", loaded.Naming.Template)
	}
	if loaded.Clean.MergeDetection != cleanMergeDetectionStrict {
		t.Fatalf("expected trimmed clean policy, got %q", loaded.Clean.MergeDetection)
	}
	if loaded.Branches["feat-one"].Parent != "main" {
		t.Fatalf("expected trimmed parent, got %q", loaded.Branches["feat-one"].Parent)
	}
	if loaded.Branches["feat-one"].LineageParent != "main" {
		t.Fatalf("expected default lineage parent, got %q", loaded.Branches["feat-one"].LineageParent)
	}
	if loaded.Branches["feat-one"].PR.URL != "https://example.invalid/pr/1" {
		t.Fatalf("expected trimmed PR URL, got %q", loaded.Branches["feat-one"].PR.URL)
	}
	if loaded.Archived["old-one"].Parent != "main" {
		t.Fatalf("expected trimmed archived parent, got %q", loaded.Archived["old-one"].Parent)
	}
}

func TestSaveStateCanonicalizesWhitespace(t *testing.T) {
	repo := newTestRepo(t)
	state := &State{
		Trunk:       "  main  ",
		RestackMode: "  rebase  ",
		Naming: NamingConfig{
			Template:  "  {slug}  ",
			NextIndex: 1,
		},
		Clean: CleanConfig{MergeDetection: "  strict  "},
		Branches: map[string]*BranchRef{
			"feat-one": {Parent: "  main  ", LineageParent: "  feat-root  ", PR: &PRMeta{Number: 1, URL: "  https://example.invalid/pr/1  ", Base: "  main  "}},
		},
	}

	if err := saveState(repo, state); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	written := readStateFile(t, repo)
	if written.Trunk != "main" {
		t.Fatalf("expected canonical trunk in file, got %q", written.Trunk)
	}
	if written.Branches["feat-one"].Parent != "main" {
		t.Fatalf("expected canonical parent in file, got %q", written.Branches["feat-one"].Parent)
	}
	if written.Branches["feat-one"].LineageParent != "feat-root" {
		t.Fatalf("expected canonical lineage parent in file, got %q", written.Branches["feat-one"].LineageParent)
	}
	if state.Branches["feat-one"].PR.Base != "main" {
		t.Fatalf("expected in-memory PR base to be canonical, got %q", state.Branches["feat-one"].PR.Base)
	}
}

func TestLoadOperationNormalizesWhitespace(t *testing.T) {
	repo := newTestRepo(t)
	op := &RestackOperation{
		Type:           "  restack  ",
		Mode:           "  rebase  ",
		OriginalBranch: "  feat-one  ",
		Queue:          []string{"  feat-one  ", "  feat-two  "},
		OriginalHeads:  map[string]string{"feat-one": "  abc123  "},
		RebaseBases:    map[string]string{"feat-two": "  def456  "},
	}

	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		t.Fatalf("marshal operation: %v", err)
	}
	path := filepath.Join(repo, ".git", "stack", "operation.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir operation dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write operation: %v", err)
	}

	loaded, err := loadOperation(repo)
	if err != nil {
		t.Fatalf("loadOperation: %v", err)
	}

	if loaded.Type != "restack" || loaded.Mode != "rebase" || loaded.OriginalBranch != "feat-one" {
		t.Fatalf("expected normalized operation, got %+v", loaded)
	}
	if loaded.Queue[0] != "feat-one" || loaded.Queue[1] != "feat-two" {
		t.Fatalf("expected normalized queue, got %v", loaded.Queue)
	}
	if loaded.OriginalHeads["feat-one"] != "abc123" {
		t.Fatalf("expected normalized original head, got %q", loaded.OriginalHeads["feat-one"])
	}
	if loaded.RebaseBases["feat-two"] != "def456" {
		t.Fatalf("expected normalized rebase base, got %q", loaded.RebaseBases["feat-two"])
	}
}

func TestNormalizeGHJSON(t *testing.T) {
	prs := []GhPR{{
		URL:         "  https://example.invalid/pr/1  ",
		BaseRefName: "  main  ",
		HeadRefOID:  "  abc123  ",
		Title:       "  Feature one  ",
		State:       "  MERGED  ",
		MergeCommit: &GhCommit{OID: "  def456  "},
	}}

	normalizeGHJSON(&prs)

	if prs[0].URL != "https://example.invalid/pr/1" {
		t.Fatalf("expected trimmed URL, got %q", prs[0].URL)
	}
	if prs[0].BaseRefName != "main" || prs[0].HeadRefOID != "abc123" {
		t.Fatalf("expected trimmed refs, got %+v", prs[0])
	}
	if prs[0].Title != "Feature one" || prs[0].State != "MERGED" {
		t.Fatalf("expected trimmed title/state, got %+v", prs[0])
	}
	if prs[0].MergeCommit == nil || prs[0].MergeCommit.OID != "def456" {
		t.Fatalf("expected trimmed merge commit, got %+v", prs[0].MergeCommit)
	}
}
