package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var errStateNotInitialized = errors.New("stack state not initialized; run stack init")

func ensurePersistedState(repoRoot string, state *State, persisted bool, out io.Writer) (bool, error) {
	if persisted {
		return false, nil
	}
	if out != nil {
		if _, err := fmt.Fprintf(out, "initialized stack state (trunk=%s, mode=%s)\n", state.Trunk, state.RestackMode); err != nil {
			return false, err
		}
	}
	if err := saveState(repoRoot, state); err != nil {
		return false, err
	}
	return true, nil
}

func loadStateFromRepo() (string, *State, error) {
	repoRoot, err := repoRoot()
	if err != nil {
		return "", nil, err
	}
	state, err := loadState(repoRoot)
	if err != nil {
		return "", nil, err
	}
	return repoRoot, state, nil
}

func loadStateFromRepoOrInfer() (string, *State, bool, error) {
	repoRoot, err := repoRoot()
	if err != nil {
		return "", nil, false, err
	}
	state, err := loadState(repoRoot)
	if err == nil {
		return repoRoot, state, true, nil
	}
	if !errors.Is(err, errStateNotInitialized) {
		return "", nil, false, err
	}
	state, err = inferState(repoRoot)
	if err != nil {
		return "", nil, false, err
	}
	return repoRoot, state, false, nil
}

func repoRoot() (string, error) {
	repoRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("not a git repository")
	}
	return strings.TrimSpace(repoRoot), nil
}

func inferState(repoRoot string) (*State, error) {
	trunk, err := detectTrunk()
	if err != nil {
		return nil, err
	}
	branches, err := listLocalBranches()
	if err != nil {
		return nil, err
	}
	state := &State{
		Version:     stateVersion,
		Trunk:       trunk,
		RestackMode: defaultRestackMode,
		Naming: NamingConfig{
			Template:  "{slug}",
			NextIndex: 1,
		},
		Cleanup: CleanupConfig{
			MergeDetection: cleanupMergeDetectionStrict,
		},
		Branches: map[string]*BranchRef{},
		Archived: map[string]*ArchivedRef{},
	}

	maxIndex := 0
	for _, branch := range branches {
		if branch == trunk {
			continue
		}
		parent, err := inferParent(branch, branches, trunk)
		if err != nil {
			return nil, err
		}
		state.Branches[branch] = &BranchRef{Parent: parent, LineageParent: parent}
		if idx := parseLeadingIndex(branch); idx > maxIndex {
			maxIndex = idx
		}
	}
	if maxIndex > 0 {
		state.Naming.NextIndex = maxIndex + 1
	}
	return state, nil
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
			return nil, errStateNotInitialized
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
	if state.Archived == nil {
		state.Archived = map[string]*ArchivedRef{}
	}
	for _, branch := range state.Branches {
		if branch == nil {
			continue
		}
		if strings.TrimSpace(branch.LineageParent) == "" {
			branch.LineageParent = branch.Parent
		}
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
	if state.Cleanup.MergeDetection == "" {
		state.Cleanup.MergeDetection = cleanupMergeDetectionStrict
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

func parseLeadingIndex(branch string) int {
	parts := strings.SplitN(branch, "-", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
