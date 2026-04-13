# git-stack

Project direction: see `MANIFEST.md`.

`git-stack` is a simple Git stacked-PR CLI focused on local branch stacks and GitHub PR automation.

## User Question Tracking

Track recurring user questions about using `git-stack` in GitHub issue #34:

- https://github.com/janeklb/git-stack/issues/34

AI agent instructions live in `docs/agent-instructions.md` (with `CLAUDE.md` and `AGENTS.md` symlinked to it).

Canonical invocation:

- `git-stack <command>`
- `git stack <command>` when `git-stack` is on your `PATH`

Optional convenience alias:

- `alias stack=git-stack`
- or `ln -s "$(command -v git-stack)" "$HOME/.local/bin/stack"`

## Dependencies

- Go 1.22+
- `git`
- `gh` (GitHub CLI), authenticated for `submit` and PR updates

The CLI intentionally uses shelling out to `git`/`gh` rather than API SDKs to keep behavior aligned with your local tools.

## Build

Build the CLI binary:

```bash
go build -o bin/git-stack ./cmd/git-stack
```

This produces `bin/git-stack`.

Or use Make targets:

```bash
make test
make test-linux
make test-linux-timings
make build
make install
```

`make test-linux` and `make test-linux-timings` run the suite inside a local Docker
container using Linux plus Go `1.22.12`, which is closer to the GitHub Actions CI
environment than running directly on macOS.

These targets were added because this repository's tests shell out to `git` and
work with temporary repositories heavily enough that the suite runs much slower
on macOS than in CI, even on fast local hardware. Running the same suite inside
local Linux closes most of that gap and gives a better apples-to-apples check
when diagnosing CI-vs-local behavior.

They also mount persistent Docker volumes for the Go build and module caches, so
repeat runs stay closer to the cached behavior in CI instead of redownloading and
rebuilding everything on each invocation.

`make install` installs `git-stack` with `go install ./cmd/git-stack` into your Go bin directory
(`GOBIN` if set, otherwise `$(go env GOPATH)/bin`). If you also want `stack ...`, add a
manual shell alias or symlink after install.

## Shell Completion

Canonical completion setup targets `git-stack` directly:

- Bash: `source <(git-stack completion bash)`
- Zsh: `source <(git-stack completion zsh)`
- Fish: `git-stack completion fish | source`
- PowerShell: `git-stack completion powershell | Out-String | Invoke-Expression`

`git stack <command>` is an equivalent runtime form, but completion for that wrapper path is
not bundled automatically. Shells generally attach completion either to `git-stack` itself or to
Git's own subcommand completion layer, so `git stack ...` needs extra shell-specific glue if you
want argument completion there too.

For now, the supported recommendation is:

- use `git-stack ...` when you want completion
- use `git stack ...` if you prefer the Git extension form and do not mind setting up shell-specific completion separately

## Testing Tiers

Use test-name conventions to run fast unit coverage separately from integration smoke coverage:

- Fast unit-focused loop (skips `IntegrationSmoke` tests):

```bash
go test ./... -count=1 -skip IntegrationSmoke
```

- Integration smoke wiring checks (`submit`, `cleanup`, `status`):

```bash
go test ./internal/app -count=1 -run IntegrationSmoke
```

- Full suite:

```bash
go test ./... -count=1
```

## Commands

`git-stack init` remains available as a repair/config migration command, but it is no longer intended to be part of the normal happy path. Normal mutating commands should auto-bootstrap stack state when that can be done unambiguously.

```text
git-stack init [--trunk <branch>] [--mode rebase|merge]
git-stack new <name> [--parent <branch>] [--template <template>] [--prefix-index]
git-stack status
git-stack restack [--mode rebase|merge] [--continue] [--abort]
git-stack submit [--all] [--next-on-cleanup <branch>] [branch]
git-stack reparent <branch> --parent <new-parent>
git-stack cleanup [--all] [--yes] [--include-squash] [--untracked]
git-stack advance [--next <branch>]
git-stack doctor
git-stack completion [bash|zsh|fish|powershell]
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
- On restack conflicts, it stops and resumes with `git-stack restack --continue`
- PR submit uses parent branch as PR base
- Existing PRs are updated safely with a managed body block
- Mutating commands require a clean worktree
- Single-branch clones are not supported (reclone without `--single-branch` or fetch all branches)
- `origin` remote is required and must expose `refs/remotes/origin/HEAD`
