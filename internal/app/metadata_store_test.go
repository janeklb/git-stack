package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const validStoredOperation = `{"version":1,"type":"restack","mode":"rebase","originalBranch":"main","queue":["feature"],"index":0,"originalHeads":{"feature":"0123456789abcdef0123456789abcdef01234567"}}`

func metadataStoreForTest(t *testing.T) *metadataStore {
	t.Helper()
	r, err := discoverRepository(newTestRepo(t), repositoryOptions{stdout: io.Discard, stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return r.metadataStore()
}

func metadataFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "metadata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func storedOperationForTest(t *testing.T) *RestackOperation {
	t.Helper()
	var op RestackOperation
	if err := json.Unmarshal([]byte(validStoredOperation), &op); err != nil {
		t.Fatal(err)
	}
	return &op
}

func TestMetadataStateCompatibility(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"state-v1.json", "legacy-state.json"} {
		t.Run(fixture, func(t *testing.T) {
			store := metadataStoreForTest(t)
			data := metadataFixture(t, fixture)
			if err := store.files.replace(store.statePath, data); err != nil {
				t.Fatal(err)
			}
			state, err := store.loadState()
			if err != nil {
				t.Fatal(err)
			}
			if fixture == "state-v1.json" {
				var expected State
				if err := json.Unmarshal(data, &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(state, &expected) {
					t.Fatalf("existing metadata changed: got %+v, want %+v", state, expected)
				}
			} else if state.Version != stateVersion || state.RestackMode != "rebase" || state.Naming.Template != "{slug}" || state.Naming.NextIndex != 1 || state.Branches["feature"].LineageParent != "main" {
				t.Fatalf("lost historical defaults: %+v", state)
			}
			if err := store.saveState(state); err != nil {
				t.Fatal(err)
			}
			again, err := store.loadState()
			if err != nil || !reflect.DeepEqual(state, again) {
				t.Fatalf("round trip changed metadata: %+v, %v", again, err)
			}
		})
	}
}

func TestMetadataRejectsInvalidStateWithoutOverwritingIt(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		`{`, `null`, `[]`, `{"version":999,"trunk":"main"}`, `{"version":0,"trunk":"main"}`,
		`{"version":null,"trunk":"main"}`, `{"trunk":"main","restackMode":"invalid"}`,
		`{"trunk":"main","branches":[]}`, `{"trunk":"main","unknown":true}`, `{}`,
	} {
		t.Run(data, func(t *testing.T) {
			store := metadataStoreForTest(t)
			if err := store.files.replace(store.statePath, []byte(data)); err != nil {
				t.Fatal(err)
			}
			if _, err := store.loadState(); err == nil || errors.Is(err, errStateNotInitialized) {
				t.Fatalf("invalid state treated as valid/missing: %v", err)
			}
			if err := store.saveState(&State{Trunk: "main"}); err == nil {
				t.Fatal("overwrote invalid or unsupported state")
			}
			after, err := os.ReadFile(store.statePath)
			if err != nil || string(after) != data {
				t.Fatalf("invalid state was not preserved: %s, %v", after, err)
			}
		})
	}
}

func TestMetadataPreservesUnknownAndLegacyRecoveryRecords(t *testing.T) {
	t.Parallel()
	cases := []string{
		string(metadataFixture(t, "legacy-operation.json")), `{`, `null`,
		strings.Replace(validStoredOperation, `"version":1`, `"version":999`, 1),
		strings.Replace(validStoredOperation, `"version":1`, `"version":null`, 1),
		strings.Replace(validStoredOperation, `"mode":"rebase"`, `"mode":"invalid"`, 1),
		strings.Replace(validStoredOperation, `"type":"restack"`, `"type":"foreign"`, 1),
		strings.Replace(validStoredOperation, `"index":0`, `"index":-1`, 1),
		strings.Replace(validStoredOperation, `"index":0`, `"index":2`, 1),
		strings.Replace(validStoredOperation, `"queue":["feature"]`, `"queue":["feature","feature"]`, 1),
		strings.Replace(validStoredOperation, `"originalHeads"`, `"foreignHeads"`, 1),
		strings.Replace(validStoredOperation, `"originalHeads":{"feature":"0123456789abcdef0123456789abcdef01234567"}`, `"originalHeads":{}`, 1),
		strings.Replace(validStoredOperation, `"originalBranch":"main"`, `"originalBranch":""`, 1),
		strings.Replace(validStoredOperation, `"queue":["feature"]`, `"queue":[]`, 1),
	}
	for i, data := range cases {
		store := metadataStoreForTest(t)
		if err := store.files.replace(store.operationPath, []byte(data)); err != nil {
			t.Fatal(err)
		}
		_, err := store.loadOperation()
		if err == nil || !strings.Contains(err.Error(), store.operationPath) {
			t.Fatalf("case %d: no actionable recovery error: %v", i, err)
		}
		if i == 0 && !strings.Contains(err.Error(), "legacy unversioned recovery record") {
			t.Fatalf("legacy record was not conservatively diagnosed: %v", err)
		}
		if err := store.saveOperation(storedOperationForTest(t)); err == nil {
			t.Fatalf("case %d: overwrote unsafe recovery record", i)
		}
		if err := store.removeOperation(); err == nil {
			t.Fatalf("case %d: removed unsafe recovery record", i)
		}
		after, err := os.ReadFile(store.operationPath)
		if err != nil || string(after) != data {
			t.Fatalf("case %d: recovery record was not preserved: %s, %v", i, after, err)
		}
	}
}

func TestMetadataValidationDoesNotPreventGraphInspection(t *testing.T) {
	t.Parallel()
	store := metadataStoreForTest(t)
	state := &State{Trunk: "main", Branches: map[string]*BranchRef{
		"self": {Parent: "self"}, "cycle-a": {Parent: "cycle-b"}, "cycle-b": {Parent: "cycle-a"},
		"metadata-missing": nil,
	}}
	if err := store.saveState(state); err != nil {
		t.Fatalf("storage performed mutation graph validation: %v", err)
	}
	if _, err := store.loadState(); err != nil {
		t.Fatalf("cannot inspect invalid graph: %v", err)
	}
}

func TestMetadataRejectsInvalidWritesWithoutChangingValidDocuments(t *testing.T) {
	t.Parallel()
	store := metadataStoreForTest(t)
	if err := store.saveState(&State{Trunk: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := store.saveOperation(storedOperationForTest(t)); err != nil {
		t.Fatal(err)
	}
	stateBefore, err := os.ReadFile(store.statePath)
	if err != nil {
		t.Fatal(err)
	}
	opBefore, err := os.ReadFile(store.operationPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []*State{nil, {Trunk: "main", RestackMode: "invalid"}, {Version: 999, Trunk: "main"}} {
		if err := store.saveState(state); err == nil {
			t.Fatal("saved an invalid state document")
		}
	}
	for _, change := range []func(*RestackOperation){
		func(op *RestackOperation) { op.Version = 999 },
		func(op *RestackOperation) { op.Mode = "invalid" },
		func(op *RestackOperation) { op.Index = -1 },
	} {
		op := storedOperationForTest(t)
		change(op)
		if err := store.saveOperation(op); err == nil {
			t.Fatal("saved an invalid operation document")
		}
	}
	stateAfter, err := os.ReadFile(store.statePath)
	if err != nil || !bytes.Equal(stateBefore, stateAfter) {
		t.Fatalf("invalid write damaged valid state: %s, %v", stateAfter, err)
	}
	opAfter, err := os.ReadFile(store.operationPath)
	if err != nil || !bytes.Equal(opBefore, opAfter) {
		t.Fatalf("invalid write damaged valid operation: %s, %v", opAfter, err)
	}
}

func TestMetadataMissingAndFailedDeletion(t *testing.T) {
	t.Parallel()
	store := metadataStoreForTest(t)
	if _, err := store.loadState(); !errors.Is(err, errStateNotInitialized) {
		t.Fatalf("missing state = %v", err)
	}
	if op, err := store.loadOperation(); op != nil || err != nil {
		t.Fatalf("missing operation = %+v, %v", op, err)
	}
	if err := store.removeOperation(); err != nil {
		t.Fatalf("missing operation removal = %v", err)
	}
	if err := store.saveOperation(storedOperationForTest(t)); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.operationPath)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("injected removal failure")
	store.files.remove = func(string) error { return cause }
	if err := store.removeOperation(); !errors.Is(err, cause) {
		t.Fatalf("lost removal error: %v", err)
	}
	after, err := os.ReadFile(store.operationPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed removal damaged journal: %s, %v", after, err)
	}
	store.files.remove = os.Remove
	store.files.syncDir = func(string) error { return cause }
	if err := store.removeOperation(); !errors.Is(err, cause) {
		t.Fatalf("lost removal durability error: %v", err)
	}
	if op, err := store.loadOperation(); err != nil || op != nil {
		t.Fatalf("post-unlink sync failure unexpectedly restored a journal: %+v, %v", op, err)
	}
}

func TestMetadataLayoutSharesStateAndTemplatesButNotRecovery(t *testing.T) {
	t.Parallel()
	repo, err := filepath.EvalSymlinks(newTestRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "linked with spaces")
	mustGit(t, repo, "worktree", "add", "-b", "feature", linked)
	mainStore, err := openMetadataStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	linkedStore, err := openMetadataStore(linked)
	if err != nil {
		t.Fatal(err)
	}
	if mainStore.statePath != linkedStore.statePath || mainStore.templatePath != linkedStore.templatePath || mainStore.operationPath == linkedStore.operationPath {
		t.Fatalf("incorrect metadata ownership: main=%+v linked=%+v", mainStore, linkedStore)
	}
	if mainStore.statePath != filepath.Join(repo, ".git", "stack", "state.json") || mainStore.operationPath != filepath.Join(repo, ".git", "stack", "operation.json") {
		t.Fatal("ordinary clone paths changed")
	}
	if err := mainStore.saveState(&State{Trunk: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := linkedStore.loadState(); err != nil {
		t.Fatalf("linked worktree cannot read shared state: %v", err)
	}
	if err := mainStore.files.replace(mainStore.templatePath, []byte("shared template\n")); err != nil {
		t.Fatal(err)
	}
	if template, found, err := linkedStore.loadTemplate(); err != nil || !found || template != "shared template\n" {
		t.Fatalf("linked template not shared: %q, %v, %v", template, found, err)
	}
	mainOp, linkedOp := storedOperationForTest(t), storedOperationForTest(t)
	linkedOp.Mode = "merge"
	if err := mainStore.saveOperation(mainOp); err != nil {
		t.Fatal(err)
	}
	if op, err := linkedStore.loadOperation(); op != nil || err != nil {
		t.Fatalf("linked worktree read another worktree's journal: %+v, %v", op, err)
	}
	if err := linkedStore.saveOperation(linkedOp); err != nil {
		t.Fatal(err)
	}
	op, err := mainStore.loadOperation()
	if err != nil || op == nil || op.Mode != "rebase" {
		t.Fatalf("linked journal overwrote main journal: %+v, %v", op, err)
	}
	if err := linkedStore.removeOperation(); err != nil {
		t.Fatal(err)
	}
	if op, err := mainStore.loadOperation(); err != nil || op == nil {
		t.Fatalf("linked removal removed main journal: %+v, %v", op, err)
	}
}

func TestCLITemplateDoesNotEnableLinkedWorktreeMutationYet(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	mustGit(t, repo, "worktree", "add", "-b", "feature", linked)
	out, code := runCLIInRepoAndCaptureWithEnv(t, linked, mergeEnvironment(os.Environ(), []string{"GIT_EDITOR=true"}), []string{"template", "pr"})
	if code == 0 || !strings.Contains(out, "not supported yet") {
		t.Fatalf("expected linked template refusal, got exit %d: %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "stack", "PR_TEMPLATE.md")); !os.IsNotExist(err) {
		t.Fatalf("template was created before linked-worktree policy: %v", err)
	}
}
