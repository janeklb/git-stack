---
name: git-stack-setup
description: Verifies whether a repository is ready for git-stack workflows by checking clone layout, remotes, GitHub CLI auth, worktree cleanliness, and stack health. Use when the user explicitly asks to validate git-stack setup or when another git-stack skill hits setup-related errors.
---

# Git-Stack Setup

## Command forms

- Use `git-stack check` as the default git-stack readiness check.
- Do not run `git-stack <command> --help` as a routine probe. Only use `--help` when you need to confirm a non-routine flag or the observed behavior does not match this skill.

## Quick start

1. Check the working tree and index state.
2. Confirm the repo has canonical `origin` and `origin/HEAD`.
3. Confirm `gh` is authenticated if PR submission is part of the workflow.
4. Run `git-stack check` when stack metadata health is relevant.
5. Report only actionable setup issues and stop once the repo is ready.

## When to use this skill

- The user explicitly asks whether a repo is ready for `git-stack`.
- Another `git-stack` workflow fails with a setup-looking error.

## Workflow

### 1. Check prerequisites

- Confirm the repository is a full clone suitable for `git-stack`.
- Confirm `origin` exists and `refs/remotes/origin/HEAD` is available.
- Confirm the worktree is clean before recommending mutating `git-stack` commands.
- Confirm `gh` authentication when PR creation or updates are expected.

### 2. Check git-stack health

- Run `git-stack check` when stack metadata or tracked-branch health may be involved.
- If the repo is simply uninitialized but otherwise normal, note that mutating commands may auto-bootstrap state when unambiguous.
- Distinguish setup failures from ordinary branching or merge-conflict workflow issues.

### 3. Report narrowly

- Report the specific failing assumption.
- Give the smallest corrective action.
- Stop once setup is healthy.

## Guardrails

- Do not create or restack branches unless the user asks for follow-up action.
- Do not rerun the same checks repeatedly in a stable repo unless the user asks.
- Keep the output focused on setup readiness.
