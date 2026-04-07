# stack assumptions

This file records repo-level workflow assumptions and command preconditions that shape `stack` behavior.

Use it when changing command semantics, relaxing constraints, or adding new workflows. If an assumption changes, audit every command that depends on it.

## Repo-level assumptions

| Assumption | Why it exists | Commands that depend on it |
| --- | --- | --- |
| Full-clone workflow with canonical remote `origin` | The tool resolves branches and trunk from `origin`, not from arbitrary remotes or single-branch clones. | All commands except root help/completion via `ensureSupportedCloneLayout()` |
| `refs/remotes/origin/HEAD` exists | Trunk detection depends on a stable remote default branch. | `init` when `--trunk` is omitted; any flow that auto-infers state from trunk |
| Clean worktree before mutation | Mutating commands should not have to reason about unrelated local edits, index state, or partially-applied history rewrites. | `init`, `new`, `restack` (new run), `submit`, `reparent`, `advance`, `cleanup` |
| Persisted stack state is internal tool state | Commands may auto-bootstrap or persist state without treating state management as a user-facing workflow. | `new`, `restack`, `status`, `reparent`, `submit`, `cleanup` |
| Single-writer tracked-stack model | A tracked stack is expected to be managed from one clone at a time. `stack` does not try to negotiate concurrent writers for the same tracked branches. | Primarily `submit`; indirectly `advance` after restacks/repairs |
| Remote tracked branches are mirrors of local tracked branches | For tracked branches, the local branch plus stack state is the source of truth. Remote branch history may be replaced when the local stack is rewritten. | `submit`, `advance` |

## Command-specific assumptions

### `submit`

- Tracked branch publication is a synchronization step, not a negotiation step.
- `submit` always pushes tracked branches with `git push --force-with-lease -u origin <branch>:<branch>`.
- This assumes tracked branches may be rebased or reparented locally as part of normal stack maintenance.
- A `--force-with-lease` rejection is treated as an unexpected remote mutation, not as a cue to retry differently.

Why this matters:
- Changing submit push semantics affects `advance`, branch-repair flows, and any assumption that tracked branch history is tool-managed.

### `advance`

- The current branch must be tracked.
- The current branch must have PR metadata and that PR must already be merged.
- The remote branch must already be deleted.
- Local branch commits must already be integrated into the PR base before local cleanup proceeds.

Why this matters:
- `advance` assumes a strict post-merge flow: cleanup the merged branch, move to a surviving branch, restack descendants, then submit them.

### `cleanup`

- `origin` is the canonical source for whether remote branches still exist.
- These commands rely on a fresh `git fetch --prune origin` before deciding which local branches are eligible for cleanup.

Why this matters:
- If remote-state interpretation changes, merged-branch cleanup rules and state reconciliation may become unsafe.

### `new`, `reparent`

- Parent branches must already exist locally or on `origin`.
- Reparenting is implemented as a history rewrite, not metadata-only relinking.

Why this matters:
- Any future “virtual” or metadata-only parent model would need a separate audit of submit/restack assumptions.

### `restack`

- A fresh restack run requires a clean worktree.
- `--continue` and `--abort` are recovery paths for an already-persisted restack operation.

Why this matters:
- Restack is the main history-rewriting primitive beneath several other workflows.

## Audit checklist

When changing a foundational assumption, check at least:

- clone/remote detection in `ensureSupportedCloneLayout()`
- clean-worktree enforcement in mutating commands
- submit push semantics and lease-failure behavior
- advance eligibility checks
- cleanup decisions that rely on fetched remote state
- state bootstrap/persistence behavior in commands that call `loadStateFromRepoOrInfer()`
