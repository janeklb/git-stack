package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const operationVersion = 1

type metadataStore struct {
	statePath     string
	operationPath string
	templatePath  string
	files         metadataFiles
}

func (r *repository) metadataStore() *metadataStore {
	return &metadataStore{
		statePath:     filepath.Join(r.commonGitDir, "stack", "state.json"),
		operationPath: filepath.Join(r.worktreeGitDir, "stack", "operation.json"),
		templatePath:  filepath.Join(r.commonGitDir, "stack", "PR_TEMPLATE.md"),
		files:         realMetadataFiles(),
	}
}

// Temporary adapter for workflows that still carry a worktree-root string.
// New storage/recovery callers use repository.metadataStore directly.
func openMetadataStore(worktreeRoot string) (*metadataStore, error) {
	r, err := discoverRepository(worktreeRoot, repositoryOptions{})
	if err != nil {
		return nil, err
	}
	return r.metadataStore(), nil
}

func decodeMetadata(data []byte, target any, version int, legacyState bool) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("expected a metadata JSON object")
	}
	encodedVersion, present := fields["version"]
	if !present {
		if !legacyState {
			return errors.New("legacy unversioned recovery record lacks evidence for safe automatic recovery; back up this file, run git status, and finish or abort the Git operation manually; reconcile stack metadata before retiring the record")
		}
	} else {
		var storedVersion int
		if bytes.Equal(bytes.TrimSpace(encodedVersion), []byte("null")) {
			return errors.New("metadata version must be an integer")
		}
		if err := json.Unmarshal(encodedVersion, &storedVersion); err != nil {
			return fmt.Errorf("invalid metadata version: %w", err)
		}
		if storedVersion != version {
			return fmt.Errorf("unsupported metadata version %d (supported: %d); file preserved", storedVersion, version)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func validateStateDocument(state *State) error {
	if state == nil {
		return errors.New("state is required")
	}
	if state.Version != 0 && state.Version != stateVersion {
		return fmt.Errorf("unsupported state version %d", state.Version)
	}
	normalizeState(state)
	state.Version = stateVersion
	if state.Trunk == "" {
		return errors.New("state trunk is required")
	}
	if state.Naming.Template == "" {
		state.Naming.Template = "{slug}"
	}
	if state.Naming.NextIndex <= 0 {
		state.Naming.NextIndex = 1
	}
	if state.RestackMode == "" {
		state.RestackMode = defaultRestackMode
	}
	if err := validateStoredRestackMode(state.RestackMode); err != nil {
		return err
	}
	// Parent cycles, self-parenting, missing branch metadata, and untracked
	// parents are graph concerns. Inspection must still be able to diagnose them.
	return nil
}

func validateStoredRestackMode(mode string) error {
	if mode != "rebase" && mode != "merge" {
		return fmt.Errorf("invalid restack mode %q; expected rebase or merge", mode)
	}
	return nil
}

func validateOperationDocument(op *RestackOperation) error {
	if op == nil {
		return errors.New("operation is required")
	}
	if op.Version != 0 && op.Version != operationVersion {
		return fmt.Errorf("unsupported operation version %d", op.Version)
	}
	normalizeOperation(op)
	op.Version = operationVersion
	if op.Type != "restack" || op.OriginalBranch == "" {
		return errors.New("operation requires type restack and an original branch")
	}
	if err := validateStoredRestackMode(op.Mode); err != nil {
		return err
	}
	if len(op.Queue) == 0 || op.Index < 0 || op.Index > len(op.Queue) {
		return errors.New("operation requires a nonempty queue and an index within it (or immediately after it)")
	}
	seen := map[string]bool{}
	for _, branch := range op.Queue {
		if branch == "" || seen[branch] || op.OriginalHeads[branch] == "" {
			return errors.New("operation queue requires unique nonempty branches with original heads")
		}
		seen[branch] = true
	}
	return nil
}

func (s *metadataStore) loadState() (*State, error) {
	data, err := os.ReadFile(s.statePath)
	if os.IsNotExist(err) {
		return nil, errStateNotInitialized
	}
	if err != nil {
		return nil, err
	}
	state := &State{}
	if err := decodeMetadata(data, state, stateVersion, true); err != nil {
		return nil, fmt.Errorf("read state %s: %w", s.statePath, err)
	}
	if err := validateStateDocument(state); err != nil {
		return nil, fmt.Errorf("invalid state %s: %w", s.statePath, err)
	}
	return state, nil
}

func (s *metadataStore) saveState(state *State) error {
	if _, err := s.loadState(); err != nil && !errors.Is(err, errStateNotInitialized) {
		return err
	}
	if err := validateStateDocument(state); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return s.files.replace(s.statePath, data)
}

func (s *metadataStore) loadOperation() (*RestackOperation, error) {
	data, err := os.ReadFile(s.operationPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	op := &RestackOperation{}
	if err := decodeMetadata(data, op, operationVersion, false); err != nil {
		return nil, fmt.Errorf("read recovery record %s: %w", s.operationPath, err)
	}
	if err := validateOperationDocument(op); err != nil {
		return nil, fmt.Errorf("invalid recovery record %s: %w", s.operationPath, err)
	}
	return op, nil
}

func (s *metadataStore) saveOperation(op *RestackOperation) error {
	if _, err := s.loadOperation(); err != nil {
		return err
	}
	if err := validateOperationDocument(op); err != nil {
		return err
	}
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	return s.files.replace(s.operationPath, data)
}

func (s *metadataStore) removeOperation() error {
	op, err := s.loadOperation()
	if err != nil || op == nil {
		return err
	}
	if err := s.files.remove(s.operationPath); err != nil {
		return fmt.Errorf("remove recovery record %s: %w", s.operationPath, err)
	}
	return s.files.syncDir(filepath.Dir(s.operationPath))
}

func (s *metadataStore) loadTemplate() (string, bool, error) {
	data, err := os.ReadFile(s.templatePath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return string(data), err == nil, err
}
