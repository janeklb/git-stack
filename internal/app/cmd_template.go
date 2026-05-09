package app

import (
	"fmt"
	"os"
	"path/filepath"
)

func (a *App) cmdTemplatePR(scopeArg string) error {
	scope, err := normalizeTemplateScope(scopeArg)
	if err != nil {
		return err
	}

	repoRootPath := ""
	if scope == templateScopeRepo {
		repoRootPath, err = repoRoot()
		if err != nil {
			return err
		}
	}

	path, err := ensurePRTemplateFile(repoRootPath, scope)
	if err != nil {
		return err
	}

	a.printlnf("opening %s PR template: %s", scope, path)
	return editFileInGitEditor(path, a.in, a.stdout, a.stderr)
}

func ensurePRTemplateFile(repoRoot string, scope templateScope) (string, error) {
	path, err := prTemplatePath(repoRoot, scope)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(defaultPRBodyTemplate), 0o600); err != nil {
		return "", fmt.Errorf("seed PR template: %w", err)
	}
	return path, nil
}
