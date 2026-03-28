# stack

`stack` is a simple Git stacked-PR CLI focused on local branch stacks and GitHub PR automation.

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
go build -o stack ./cmd/stack
```

Create the git extension name as a symlink:

```bash
ln -sf stack git-stack
```

To make both forms work, ensure both `stack` and `git-stack` are available on your `PATH`
(`git-stack` can be a symlink to `stack`).

Or use Make targets:

```bash
make test
make build
```

## Commands

```text
stack init [--trunk <branch>] [--mode rebase|merge]
stack new <name> [--parent <branch>] [--template <template>] [--prefix-index]
stack status
stack restack [--mode rebase|merge] [--continue] [--abort]
stack submit [--all] [branch]
stack reparent <branch> --parent <new-parent>
stack repair
```

## State

State is local-only and stored in:

- `.git/stack/state.json`
- `.git/stack/operation.json` (only while `restack` is in progress)

## MVP behavior notes

- Stack unit is branch -> PR
- Parent is inferred initially and persisted in local state
- Trunk defaults from `origin/HEAD` when available
- `restack` defaults to rebase, supports merge mode
- On restack conflicts, it stops and resumes with `stack restack --continue`
- PR submit uses parent branch as PR base
- Existing PRs are updated safely with a managed body block
- Mutating commands require a clean worktree
