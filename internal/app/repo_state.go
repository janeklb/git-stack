package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func loadStateFromRepo() (string, *State, error) {
	repoRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, errors.New("not a git repository")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	state, err := loadState(repoRoot)
	if err != nil {
		return "", nil, err
	}
	return repoRoot, state, nil
}

func statePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".git", "stack", "state.json")
}

func opPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".git", "stack", "operation.json")
}

func loadState(repoRoot string) (*State, error) {
	path := statePath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("stack state not initialized; run stack init")
		}
		return nil, err
	}
	state := &State{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	if state.Branches == nil {
		state.Branches = map[string]*BranchRef{}
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
	return state, nil
}

func saveState(repoRoot string, state *State) error {
	path := statePath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state.Version = stateVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadOperation(repoRoot string) (*RestackOperation, error) {
	path := opPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	op := &RestackOperation{}
	if err := json.Unmarshal(data, op); err != nil {
		return nil, err
	}
	return op, nil
}

func saveOperation(repoRoot string, op *RestackOperation) error {
	path := opPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func removeOperation(repoRoot string) error {
	path := opPath(repoRoot)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
