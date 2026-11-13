package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var errStateNotInitialized = errors.New("stack state not initialized; run git-stack init")

// Storage can now resolve these layouts, but CLI mutation must wait for the
// recovery and branch-ownership safeguards in #149.
func (r *repository) requireLegacyCommandLayout() error {
	if r.worktreeGitDir != filepath.Join(r.worktreeRoot, ".git") {
		return errors.New("commands in linked worktrees or separate Git directories are not supported yet")
	}
	return nil
}

type templateScope string

const (
	templateScopeRepo templateScope = "repo"
	templateScopeUser templateScope = "user"
)

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

func requirePersistedTrackedState(state *State, persisted bool, command string) error {
	if !persisted {
		return fmt.Errorf("%s requires initialized stack state; track a branch first with git-stack new or git-stack new --adopt", command)
	}
	if len(state.Branches) == 0 {
		return fmt.Errorf("%s requires at least one tracked branch; track a branch first with git-stack new or git-stack new --adopt", command)
	}
	return nil
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
	state, err = inferStateDefaults(repoRoot)
	if err != nil {
		return "", nil, false, err
	}
	return repoRoot, state, false, nil
}

func repoRoot() (string, error) {
	repoRoot, err := gitOutputTrimmed("rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("not a git repository")
	}
	return repoRoot, nil
}

func inferStateDefaults(repoRoot string) (*State, error) {
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
		Branches: map[string]*BranchRef{},
		Archived: map[string]*ArchivedRef{},
	}

	maxIndex := 0
	for _, branch := range branches {
		if branch == trunk {
			continue
		}
		if idx := parseLeadingIndex(branch); idx > maxIndex {
			maxIndex = idx
		}
	}
	if maxIndex > 0 {
		state.Naming.NextIndex = maxIndex + 1
	}
	return state, nil
}

func inferStateGraph(repoRoot string) (*State, error) {
	state, err := inferStateDefaults(repoRoot)
	if err != nil {
		return nil, err
	}
	branches, err := listLocalBranches()
	if err != nil {
		return nil, err
	}
	for _, branch := range branches {
		if branch == state.Trunk {
			continue
		}
		parent, err := inferParent(branch, branches, state.Trunk)
		if err != nil {
			return nil, err
		}
		state.Branches[branch] = &BranchRef{Parent: parent, LineageParent: parent}
	}
	return state, nil
}

func userStackDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "git-stack"), nil
}

func userPRTemplatePath() (string, error) {
	configDir, err := userStackDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "PR_TEMPLATE.md"), nil
}

func normalizeTemplateScope(scope string) (templateScope, error) {
	switch strings.TrimSpace(scope) {
	case "", string(templateScopeRepo):
		return templateScopeRepo, nil
	case string(templateScopeUser):
		return templateScopeUser, nil
	default:
		return "", errors.New("--scope must be repo or user")
	}
}

func prTemplatePath(repoRoot string, scope templateScope) (string, error) {
	switch scope {
	case templateScopeRepo:
		if repoRoot == "" {
			return "", errors.New("repo root is required for repo template scope")
		}
		store, err := openMetadataStore(repoRoot)
		if err != nil {
			return "", err
		}
		return store.templatePath, nil
	case templateScopeUser:
		return userPRTemplatePath()
	default:
		return "", fmt.Errorf("unknown template scope: %s", scope)
	}
}

func loadPRTemplate(repoRoot string) (string, bool, error) {
	store, err := openMetadataStore(repoRoot)
	if err != nil {
		return "", false, err
	}
	if text, found, err := store.loadTemplate(); err != nil || found {
		return text, found, err
	}
	userTemplatePath, err := userPRTemplatePath()
	if err != nil {
		return "", false, err
	}
	userTemplate, err := os.ReadFile(userTemplatePath)
	if err == nil {
		return string(userTemplate), true, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	return "", false, nil
}

func loadState(repoRoot string) (*State, error) {
	store, err := openMetadataStore(repoRoot)
	if err != nil {
		return nil, err
	}
	return store.loadState()
}

func saveState(repoRoot string, state *State) error {
	store, err := openMetadataStore(repoRoot)
	if err != nil {
		return err
	}
	return store.saveState(state)
}

func loadOperation(repoRoot string) (*RestackOperation, error) {
	store, err := openMetadataStore(repoRoot)
	if err != nil {
		return nil, err
	}
	return store.loadOperation()
}

func saveOperation(repoRoot string, op *RestackOperation) error {
	store, err := openMetadataStore(repoRoot)
	if err != nil {
		return err
	}
	return store.saveOperation(op)
}

func removeOperation(repoRoot string) error {
	store, err := openMetadataStore(repoRoot)
	if err != nil {
		return err
	}
	return store.removeOperation()
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
