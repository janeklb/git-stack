package app

import (
	"fmt"
	"os"
)

func (a *App) cmdTemplatePR(scopeArg string) error {
	scope, err := normalizeTemplateScope(scopeArg)
	if err != nil {
		return err
	}

	repoRootPath := ""
	if scope == templateScopeRepo {
		repo, err := discoverRepository(".", repositoryOptions{in: a.in, stdout: a.stdout, stderr: a.stderr})
		if err != nil {
			return err
		}
		if err := repo.requireLegacyCommandLayout(); err != nil {
			return err
		}
		repoRootPath = repo.worktreeRoot
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
	if err := realMetadataFiles().replace(path, []byte(defaultPRBodyTemplate)); err != nil {
		return "", fmt.Errorf("seed PR template: %w", err)
	}
	return path, nil
}
