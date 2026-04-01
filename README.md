# stack

Project direction: see `MANIFEST.md`.

`stack` is a simple Git stacked-PR CLI focused on local branch stacks and GitHub PR automation.

## User Question Tracking

Track recurring user questions about using `stack` in GitHub issue #34:

- https://github.com/janeklb/stack/issues/34

AI agent instructions live in `docs/agent-instructions.md` (with `CLAUDE.md` and `AGENTS.md` symlinked to it).

It supports both forms:

- `stack <command>`
- `git stack <command>` (by installing `git-stack` on your `PATH`)

## Dependencies

- Go 1.22+
- `git`
- `gh` (GitHub CLI), authenticated for `submit` and PR updates

The CLI intentionally uses shelling out to `git`/`gh` rather than API SDKs to keep behavior aligned with your local tools.

## Build

Build the CLI binary:

```bash
go build -o bin/stack ./cmd/stack
```

This produces `bin/stack`.

Or use Make targets:

```bash
make test
make build
make install
```

`make install` installs `stack` with `go install ./cmd/stack` into your Go bin directory
(`GOBIN` if set, otherwise `$(go env GOPATH)/bin`) and creates `git-stack` as a symlink
in the same location so both `stack ...` and `git stack ...` work.

## Testing Tiers

Use test-name conventions to run fast unit coverage separately from integration smoke coverage:

- Fast unit-focused loop (skips `IntegrationSmoke` tests):

```bash
go test ./... -count=1 -skip IntegrationSmoke
```

- Integration smoke wiring checks (`submit`, `prune-local`, `status`):

```bash
go test ./internal/app -count=1 -run IntegrationSmoke
```

- Full suite:

```bash
go test ./... -count=1
```

## Commands

```text
stack init [--trunk <branch>] [--mode rebase|merge]
stack new <name> [--parent <branch>] [--template <template>] [--prefix-index]
stack status
stack restack [--mode rebase|merge] [--continue] [--abort]
stack submit [--all] [branch]
stack reparent <branch> --parent <new-parent>
stack prune-local [--yes]
stack repair
stack completion [bash|zsh|fish|powershell]
```

## State

State is local-only and stored in:

- `.git/stack/state.json`
- `.git/stack/operation.json` (only while `restack` is in progress)

## MVP behavior notes

- Stack unit is branch -> PR
- Parent is inferred initially and persisted in local state
- Trunk defaults from `origin/HEAD` when available
- Stack operations infer graph from git when state is missing (stateless-first)
- `restack` defaults to rebase, supports merge mode
- On restack conflicts, it stops and resumes with `stack restack --continue`
- PR submit uses parent branch as PR base
- Existing PRs are updated safely with a managed body block
- Mutating commands require a clean worktree
- Single-branch clones are not supported (reclone without `--single-branch` or fetch all branches)
- `origin` remote is required and must expose `refs/remotes/origin/HEAD`
