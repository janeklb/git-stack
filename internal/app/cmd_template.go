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

	if scope == templateScopeRepo {
		repo, err := a.discoverRepository()
		if err != nil {
			return err
		}
		if err := repo.requireSupportedCommandLayout(); err != nil {
			return err
		}
	}

	path, err := a.ensurePRTemplateFile(scope)
	if err != nil {
		return err
	}

	a.printlnf("opening %s PR template: %s", scope, path)
	runner, opts := a.editorExecution()
	return editFileInGitEditor(path, runner, opts)
}

func (a *App) ensurePRTemplateFile(scope templateScope) (string, error) {
	path, err := a.prTemplatePath(scope)
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
