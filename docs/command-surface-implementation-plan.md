# Command Surface Implementation Plan

## Goal

Reshape `stack` around a smaller, more intuitive workflow surface while preserving strict, deterministic behavior.

Primary outcomes:

- make post-merge advance a first-class command
- replace fragmented maintenance commands with a single `cleanup` command
- define "current stack" once and use it consistently
- auto-bootstrap persisted state on first mutating command when bootstrap is unambiguous

## Target Surface

Target top-level commands:

- `stack new`
- `stack status`
- `stack restack`
- `stack submit`
- `stack reparent`
- `stack cleanup`
- `stack advance`
- `stack doctor`
- `stack completion`

Commands to remove or fold away:

- remove `stack refresh`
- remove `stack prune-local`

`stack init` is no longer part of the normal happy path. It may remain temporarily during migration, but the intended workflow is that the tool manages state automatically.

## Canonical Current Stack Semantics

`Current stack` means the connected tracked component rooted at the topmost tracked ancestor of the current branch, including all tracked descendants beneath that root.

Implications:

- stacks may be tree-shaped, not just linear
- if the current branch has siblings under the same tracked root, those siblings are part of the same current stack
- if a merged parent is removed and its children are reparented to trunk, those children may become separate active stacks
- lineage history is preserved even when active parentage changes

This definition should be used consistently by:

- `status` default view
- `submit` default scope
- `cleanup` default scope
- any stack-body sync scope derived from the current branch

## Command Semantics

### `stack advance [--next <branch>]`

Intent:

- current branch merged remotely; move work forward

Default behavior:

1. fetch and prune remote refs
2. require existing persisted state
3. require current branch to be tracked
4. require current branch PR to be merged
5. require remote branch to be deleted
6. require branch to be safely deletable locally
7. clean up current branch from local repo and state
8. reparent children to the appropriate active parent while preserving `lineageParent`
9. choose checkout target:
   - no child: trunk
   - one child: that child
   - multiple children: require `--next` or prompt explicitly
10. switch to chosen target
11. restack remaining affected branches
12. submit the affected current stack

Failure rules:

- if safety checks fail, abort before mutating state
- if restack stops for conflicts, preserve operation state and return normal `restack --continue/--abort` guidance

Initial flag surface:

- `--next <branch>`

Do not add `--no-submit` or `--no-restack` unless real usage proves they are needed.

### `stack cleanup [--all] [--yes] [--include-squash]`

Intent:

- reconcile merged branches with the local repo and stack state

Default behavior:

- operate on the current stack only
- discover safely eligible merged branches in that scope
- delete eligible local branches
- update tracked stack state for tracked branches
- reparent children and preserve lineage where needed
- print a plan before applying unless `--yes` is set

Scope rules:

- default: current stack
- `--all`: scan all eligible local branches

Tracked vs untracked branches:

- tracked branches: delete locally when safe and update stack state
- untracked branches: delete locally when safe without requiring stack metadata

Merge detection policy:

- default: use the repo-level cleanup merge-detection policy
- `--include-squash`: command-level override that broadens cleanup eligibility beyond strict merge detection

Repo-level policy:

- store a default cleanup merge-detection policy in persisted config/state
- initial default should remain strict
- command-level flags may override that default for a single invocation

### `stack submit [branch]`

Intent remains the same, but default scope changes.

Target behavior:

- explicit branch argument: submit the current stack rooted around that branch's tracked component, or reject if the branch is not tracked
- no branch argument: submit the current stack for the current branch

This replaces the current default behavior of submitting only the ancestor chain of the current branch.

`--all` remains available for submitting all tracked branches.

### `stack status`

Keep the current default scope conceptually aligned with `current stack`.

### `stack init`

Transition plan:

- stop requiring `init` for normal usage
- keep only if needed as an explicit repair or reconfiguration command during migration
- eventually decide whether it should remain as a niche config command or be removed entirely

## State Management

Persisted state remains first-class internally.

State continues to hold:

- trunk
- restack mode
- naming config
- tracked branch parentage
- lineage parentage
- PR metadata
- archived lineage references
- in-progress restack operation state

The workflow change is user-facing, not conceptual:

- users should not need to think about state bootstrapping during normal use
- the tool should create and maintain state automatically when the required defaults can be derived unambiguously

### Auto-Bootstrap Rules

Mutating commands should auto-bootstrap persisted state when state is missing and all required defaults are available without guessy behavior.

Initial mutating commands in scope:

- `new`
- `reparent`
- `restack`
- `submit`
- `cleanup`

`advance` is intentionally excluded from auto-bootstrap. It should require existing persisted state because it depends on tracked current-branch metadata and strict post-merge workflow guarantees.

Bootstrap inputs:

- trunk from explicit flag if present, otherwise `origin/HEAD`
- restack mode from default
- naming config from default
- tracked branch relationships inferred from git ancestry

Refuse bootstrap when:

- trunk cannot be determined reliably
- repository assumptions in the manifest are not met
- a command needs persisted information that cannot be safely inferred for the requested action

Read-only commands may still inspect inferred state, but should prefer the same current-stack semantics as persisted-state flows.

## Tree-Shaped Stack Rules

Branch trees are valid.

When removing a merged tracked branch:

- children are reparented to the replacement active parent
- `Parent` changes to the new active parent
- `lineageParent` stays unchanged unless the operation is an explicit user-directed reparent
- PR base metadata updates with the new active parent where needed

This preserves historical lineage without forcing the active graph to remain linear.

## Migration Steps

1. Define and centralize current-stack selection helpers.
2. Switch `submit` default scope to current-stack semantics.
3. Introduce `advance` as a top-level command backed by the existing strict post-merge flow.
4. Introduce `cleanup` as the merged-branch reconciliation command.
5. Move shared merged-branch cleanup logic behind reusable helpers used by both `cleanup` and `advance`.
6. Remove `refresh` once `advance` and `cleanup` cover its intended workflows.
7. Remove `prune-local` once `cleanup` covers its intended workflows.
8. Add auto-bootstrap helpers for mutating commands.
9. Decide whether `init` remains as a niche repair/config command or is removed.

## Testing Plan

Add or update coverage for:

- current-stack selection with linear and tree-shaped stacks
- `submit` default behavior on forked stacks
- `advance` success path with zero, one, and multiple children
- `advance` early failure when branch is not safely deletable
- `advance` interaction with restack conflict state
- `cleanup` default current-stack scope
- `cleanup --all`
- `cleanup` handling for tracked and untracked merged branches
- `cleanup --include-squash`
- auto-bootstrap on first mutating command
- migration coverage for command removal and replacement flows

## Decisions Made

- `advance` should be a top-level command and not remain a flag on `refresh`
- `advance` should require existing persisted state and a tracked current branch
- `advance` should start with `stack advance [--next <branch>]`
- `cleanup` is the reconciliation primitive beneath `advance`, but also stands on its own as a direct command
- `cleanup` defaults to operating on the current stack
- `cleanup` should be able to remove safely eligible merged branches that are not tracked in stack state
- `current stack` should use one canonical tree-aware definition across `status`, `submit`, and `cleanup`
- auto-bootstrap should apply to normal mutating commands when bootstrap is unambiguous, but not to `advance`
- cleanup merge detection should support both a repo-level default policy and command-level overrides such as `--include-squash`

## Deferred Questions

- whether `init` remains as an explicit repair/config command during or after migration
  answer: after migration we can replace `init` with a `config` command for managing `stack` tool configuration
- exact discovery rules for untracked branches under default `cleanup` scope
  answer: by default, the `cleanup` command should scan all tracked branches to see if any can be safely deleted (ie. have been fully merged to trunk). an `--untracked` flag should instruct the command to also check untracked local branches.
- whether multi-child `advance` should always prompt when `--next` is absent or refuse in non-interactive contexts
  answer: the `advance` command should only be available in interactive contexts
- whether to separate stable config from dynamic stack state later while keeping the same user-facing workflow
  answer: (TO DO)

You can answer deferred questions directly in this document underneath each item as decisions solidify.
