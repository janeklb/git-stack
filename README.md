# git-stack

[![CI](https://github.com/janeklb/git-stack/actions/workflows/ci.yml/badge.svg)](https://github.com/janeklb/git-stack/actions/workflows/ci.yml)

`git-stack` is a CLI for managing stacked pull requests on GitHub.

**Stacked PRs** let you break a large feature into a sequence of small, reviewable slices — each PR builds on the previous one. `git-stack` manages the local branch graph, handles rebases when the stack changes, and keeps GitHub PR metadata (base branches, descriptions) in sync automatically.

The tool is intentionally opinionated and low-configuration: it targets one workflow well rather than trying to cover every possible setup. See [`MANIFEST.md`](MANIFEST.md) for the design rationale.

## Requirements

- `git`
- `gh` (GitHub CLI), authenticated — required for `submit` and PR updates

## Installation

### Homebrew

```bash
brew install janeklb/tap/git-stack
```

### Go

Requires Go 1.21+.

```bash
go install github.com/janeklb/git-stack/cmd/git-stack@latest
```

This installs `git-stack` to your Go bin directory (`GOBIN` if set, otherwise `$(go env GOPATH)/bin`). Make sure that directory is on your `PATH`.

### Binaries

Release binaries are also published on the GitHub Releases page for tagged versions.

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

# After a PR is merged, move local state forward and restack descendants
git-stack forward
```

`submit` is the step that actually creates or updates GitHub PRs. PRs are not created by a plain `git push`.

## Commands

```text
git-stack new <name> [--parent <branch>] [--template <template>] [--prefix-index]
git-stack state
git-stack submit [--all] [--next-on-clean <branch>] [branch]
git-stack restack [--mode rebase|merge] [--continue] [--abort]
git-stack forward [--next <branch>]
git-stack clean [--all] [--yes] [--untracked]
git-stack reparent [branch] --onto <new-parent>
git-stack template pr [--scope repo|user]
git-stack check
git-stack init [--trunk <branch>] [--mode rebase|merge]
git-stack version
git-stack completion [bash|zsh|fish|powershell]
```

`init` is available as a repair/config migration command but is not part of the normal workflow. Mutating commands auto-bootstrap stack state when they can do so unambiguously.

## Shell completion

Homebrew installs completion files with the package, but your shell still needs Homebrew's completion support enabled for them to load automatically.

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

For maintainer-facing release details, including the Homebrew handoff, see [`docs/releasing.md`](docs/releasing.md).

## State

Stack state is local-only:

- `.git/stack/state.json` — persisted branch graph and metadata
- `.git/stack/operation.json` — present only while a `restack` is in progress
- `.git/stack/PR_TEMPLATE.md` — optional per-repo PR body template for `submit`
- `<user config dir>/git-stack/PR_TEMPLATE.md` — optional user-level PR body template for `submit`

If `.git/stack/PR_TEMPLATE.md` exists, `submit` uses it first. Otherwise, if `<user config dir>/git-stack/PR_TEMPLATE.md` exists, `submit` uses that. Custom templates are rendered as Go `text/template` templates and used as the PR body verbatim. `submit` does not prepend or append anything around a custom template.

Template data:

- `.commits` — list of first-line commit subjects included in the PR
- `.stackedPRsSection` — managed `## Stacked PRs` block

If a custom template does not reference `.stackedPRsSection`, the PR body will not include the stacked-PR section.

If either custom template file exists but is empty, `submit` uses that empty template as-is rather than falling back to the default.

When neither custom template file exists, `submit` uses [the default template](./internal/app/default_pr_body.md.tmpl).

`<user config dir>` follows the platform default returned by Go's `os.UserConfigDir`, such as `~/.config` on Linux or `~/Library/Application Support` on macOS.

Use `git-stack template pr` to edit the repo template, or `git-stack template pr --scope user` to edit the user-level template. When the selected template file does not exist yet, `git-stack` seeds it with the built-in default template before opening your configured Git editor.

## Key constraints

- Mutating commands require a clean worktree.
- Full clones are required; single-branch clones are not supported.
- The repository must have an `origin` remote, and `refs/remotes/origin/HEAD` must be available for trunk detection.
- `submit` is the command that creates or updates PRs; a plain `git push` does not.
- `init` is mainly a repair or reconfiguration command; normal mutating workflows auto-bootstrap state when they can do so unambiguously.

## Building from source

Requires Go 1.21+.

```bash
go build -o bin/git-stack ./cmd/git-stack
```

Or use the Makefile:

```bash
make build    # produces bin/git-stack
make install  # go install into GOBIN / GOPATH/bin
make test     # run the full test suite
```

### Running tests on macOS

The test suite shells out to `git` heavily. It runs much faster inside a Linux container:

```bash
make test-linux          # runs tests in a local Linux container
```

This target uses the official Go Docker image with persistent Go build and module caches, so repeat runs stay fast.
