---
name: git-stack-feature
description: Builds a feature as a sequence of small stacked pull requests using the git-stack CLI and GitHub PR submission flow. Use when the user wants a feature split into reviewable slices, asks for stacked PRs, or wants the agent to manage branch creation, restacking, and PR updates with git-stack. If setup validation is needed, load a separate git-stack setup skill only when the user asks for it or when setup-related errors appear.
---

# Git-Stack Feature

## Quick start

1. Split the feature into the smallest reviewable branch-sized slices.
2. For each slice, create a branch with `git-stack new`, implement only that slice, commit, test, and run `git-stack submit`.
3. Use `git-stack state` to verify the stack after each submission.
4. After a slice merges, use `git-stack forward` to clean up and advance descendants.

## Workflows

### 1. Plan the stack

- Restate the feature as an ordered list of PR-sized slices.
- Prefer slices that are independently reviewable and low-risk.
- Ask one (or, at most, a few) short clarifying question if the dependency order is unclear.
- Name branches for the slice purpose, not for the whole feature.

### 2. Build each slice

For each planned slice:

1. Create the branch with `git-stack new <branch>`. If some work has already been done, use `git-stack new --adopt` to adopt the current branch as a stack slice.
2. Implement only the code needed for that slice.
3. Commit normally once the slice is coherent.
4. Run the smallest relevant verification for that slice.
5. Submit with `git-stack submit` to push branches and create or update the PR.
6. Inspect `git-stack state` to confirm the stack looks correct.

### 3. Adjust the stack when adjustments are made to any stack slice

- Use `git-stack restack` after rewriting or when descendants need to move onto updated parents.
- If a restack stops on conflicts, resolve them and continue with `git-stack restack --continue`.
- Abort a broken restack with `git-stack restack --abort` only when abandoning that restack attempt.

### 4. Advance after merges

- Treat `git-stack forward` as the default post-merge path.
- Only use `forward` when a PR in the current stack has merged and you are advancing the remaining work.
- After `forward`, check `git-stack state` and run `git-stack submit` again if later PRs need updates.

## Guardrails

- Use `git-stack submit` for PR creation and updates; do not replace it with plain `git push`.
- Keep each branch focused on one reviewable idea.
- Prefer fixing stack shape with `reparent` or `restack` instead of ad hoc git commands.
- Only switch to the dedicated setup skill when the user asks for setup validation or when errors point to setup problems like missing `origin`, missing `origin/HEAD`, unauthenticated `gh`, unsupported clone layout, or failed stack health checks.
