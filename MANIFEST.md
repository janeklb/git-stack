# stack manifest

`stack` exists to support one workflow really well: personal stacked PR development, especially the high-frequency loop of advancing a stack after merges.

## Purpose

- Explore and refine a stacked-PR workflow that works for the maintainer.
- Build a strongly opinionated, low-config CLI optimized for individual contributors.
- Optimize for daily reliability and flow speed with near-zero surprises.
- Make the post-merge "move me forward" workflow the highest-confidence path in the tool.

## Core posture

- Deterministic behavior over flexibility.
- If assumptions are not met, refuse clearly instead of guessing.
- Persist and maintain stack state automatically when the workflow is unambiguous.
- Start with strict constraints; loosen only when there is proven need.

## Current assumptions

- Full clone workflow (not single-branch clone).
- Remote `origin` exists and is the canonical remote.
- `refs/remotes/origin/HEAD` is available for trunk detection.
- Local git state is clean before mutating commands.
- Persisted stack state is an internal implementation detail managed by the tool, not a setup step users should need to think about during normal use.
- "Current stack" should be interpreted consistently across commands as the connected tracked component rooted at the topmost tracked ancestor of the current branch, including all tracked descendants beneath that root.

## Non-goals (for now)

- Team/process orchestration (policy engines, approval flows, org workflows).
- GUI/TUI dashboards.
- Multi-forge abstraction as a first-class goal.
- Broad edge-case handling outside this opinionated workflow.
- Exposing internal state-management steps as part of the normal happy path unless they materially improve clarity.

## Workflow shape

- Core daily commands should map directly to user intent: create work, inspect stack state, restack history, submit PRs, clean up merged work, and advance after merges.
- Post-merge advance is a first-class workflow and should have a dedicated command rather than being hidden behind flags on a broader maintenance command.
- Cleanup is the reconciliation primitive beneath advance, but should also stand on its own as a direct command:
  - `advance` handles the strict "current branch merged, move me forward" flow and includes targeted cleanup of the current merged branch as one step in that workflow.
  - `cleanup` handles merged-branch reconciliation directly for the current stack by default, with broader scopes available explicitly, including branches not tracked in stack state when they are safely eligible for removal.
- Tree-shaped stacks are allowed. When a merged parent is removed, its children may become separate active stacks while retaining lineage history.
- Read-only commands may infer or inspect state freely; mutating commands should transparently bootstrap and persist state when they can do so without ambiguity.

## Evolution

This is a working document. Assumptions are intentionally tight now and may be relaxed over time when real usage justifies it.
