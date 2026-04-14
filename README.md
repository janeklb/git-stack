# git-stack

[![CI](https://github.com/janeklb/git-stack/actions/workflows/ci.yml/badge.svg)](https://github.com/janeklb/git-stack/actions/workflows/ci.yml)

`git-stack` is a CLI for managing stacked pull requests on GitHub.

**Stacked PRs** let you break a large feature into a sequence of small, reviewable slices — each PR builds on the previous one. `git-stack` manages the local branch graph, handles rebases when the stack changes, and keeps GitHub PR metadata (base branches, descriptions) in sync automatically.

The tool is intentionally opinionated and low-configuration: it targets one workflow well rather than trying to cover every possible setup. See [`MANIFEST.md`](MANIFEST.md) for the design rationale.

## Requirements

- Go 1.22+
- `git`
- `gh` (GitHub CLI), authenticated — required for `submit` and PR updates

## Installation

```bash
go install github.com/janeklb/git-stack/cmd/git-stack@latest
```

This installs `git-stack` to your Go bin directory (`GOBIN` if set, otherwise `$(go env GOPATH)/bin`). Make sure that directory is on your `PATH`.

Once installed, `git-stack` also works as a Git extension — `git stack <command>` is equivalent when `git-stack` is on your `PATH`.

Optional convenience alias or symlink:

```bash
alias stack=git-stack
# or:
ln -s "$(command -v git-stack)" "$HOME/.local/bin/stack"
```

## Typical workflow

```bash
# Start a new branch in the stack (branching from the current branch)
git-stack new my-feature-part-1

# Do your work and commit normally
git add . && git commit -m "..."

# Push branches and create/update GitHub PRs
git-stack submit

# Check the current stack graph
git-stack state

# Start the next slice
git-stack new my-feature-part-2
# ... repeat

# After a PR is merged, advance local state and restack descendants
git-stack advance
```

`submit` is the step that actually creates or updates GitHub PRs. PRs are not created by a plain `git push`.

## Commands

```text
git-stack new <name> [--parent <branch>] [--template <template>] [--prefix-index]
git-stack state
git-stack submit [--all] [--next-on-clean <branch>] [branch]
git-stack restack [--mode rebase|merge] [--continue] [--abort]
git-stack advance [--next <branch>]
git-stack clean [--all] [--yes] [--include-squash] [--untracked]
git-stack reparent <branch> --parent <new-parent>
git-stack check
git-stack init [--trunk <branch>] [--mode rebase|merge]
git-stack completion [bash|zsh|fish|powershell]
```

`init` is available as a repair/config migration command but is not part of the normal workflow. Mutating commands auto-bootstrap stack state when they can do so unambiguously.

## Shell completion

```bash
# Bash
source <(git-stack completion bash)

# Zsh
source <(git-stack completion zsh)

# Fish
git-stack completion fish | source

# PowerShell
git-stack completion powershell | Out-String | Invoke-Expression
```

Completion targets `git-stack` directly. The `git stack ...` extension form needs separate shell-specific setup if you want argument completion there too.

## State

Stack state is local-only:

- `.git/stack/state.json` — persisted branch graph and metadata
- `.git/stack/operation.json` — present only while a `restack` is in progress
- `.git/stack/PR_TEMPLATE.md` — optional per-repo PR body template for `submit`

If `.git/stack/PR_TEMPLATE.md` exists, `submit` renders it as a Go `text/template` and uses the result as the PR body verbatim. `submit` does not prepend or append anything around a custom template.

Template data:

- `.commits` — list of first-line commit subjects included in the PR
- `.stackedPRsSection` — managed `## Stacked PRs` block

If a custom template does not reference `.stackedPRsSection`, the PR body will not include the stacked-PR section.

If `.git/stack/PR_TEMPLATE.md` exists but is empty, `submit` uses that empty template as-is rather than falling back to the default.

When `.git/stack/PR_TEMPLATE.md` is absent, `submit` uses this default template:

```md
## Summary
{{- range .commits }}
- {{ . }}
{{- end }}

{{ .stackedPRsSection }}
```

## Behavior notes

- Stack unit is branch → PR
- Parent branch is inferred initially and persisted in local state
- Trunk defaults from `origin/HEAD` when available
- Stack operations infer graph from git when state is missing (stateless-first)
- `restack` defaults to rebase mode; merge mode is available via `--mode merge`
- On restack conflicts, the operation pauses; resume with `git-stack restack --continue`
- `submit` sets each PR's base to the parent branch and updates a managed block in the PR description
- Existing PRs are updated safely — only the managed block is touched
- Mutating commands require a clean worktree
- Single-branch clones are not supported (reclone without `--single-branch` or fetch all branches)
- `origin` remote is required and must expose `refs/remotes/origin/HEAD`

## Building from source

```bash
go build -o bin/git-stack ./cmd/git-stack
```

Or use the Makefile:

```bash
make build    # produces bin/git-stack
make install  # go install into GOBIN / GOPATH/bin
make test     # run the full test suite
```

### Running tests locally on macOS

The test suite shells out to `git` heavily. It runs much faster inside a Linux container:

```bash
make test-linux          # mirrors the CI environment
make test-linux-timings  # same, with per-test timing output
```

These targets use Docker with persistent Go build and module caches, so repeat runs stay close to CI caching behavior.

To skip the slower integration tests during development:

```bash
go test ./... -count=1 -skip IntegrationSmoke
```
