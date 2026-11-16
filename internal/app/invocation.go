package app

import "os"

// Only command entry discovers layout. Workflows reuse the saved repository;
// user template editing and shell completion may run without clone preflight.
func (a *App) discoverRepository() (*repository, error) {
	if a.repo != nil {
		return a.repo, nil
	}
	dir := a.dir
	if dir == "" {
		dir = "."
	}
	r, err := discoverRepository(dir, repositoryOptions{
		runner: a.runner, env: a.env, in: a.in, stdout: a.stdout, stderr: a.stderr,
	})
	if err != nil {
		return nil, err
	}
	a.repo = r
	return r, nil
}

func (a *App) editorExecution() (commandRunner, commandRunOptions) {
	if a.repo != nil {
		return a.repo.run, a.repo.options
	}
	// User-level template editing intentionally works outside a repository.
	// Keep it on the same runner/streams without requiring clone discovery.
	runner := a.runner
	if runner == nil {
		runner = runCommand
	}
	env := a.env
	if env == nil {
		env = os.Environ()
	}
	return runner, commandRunOptions{
		dir: a.dir, stdin: a.in, stdout: a.stdout, stderr: a.stderr, baseEnv: env,
	}
}
