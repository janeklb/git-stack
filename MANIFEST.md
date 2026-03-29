# stack manifest

`stack` exists to support one workflow really well: personal stacked PR development.

## Purpose

- Explore and refine a stacked-PR workflow that works for the maintainer.
- Build a strongly opinionated, low-config CLI optimized for individual contributors.
- Optimize for daily reliability and flow speed with near-zero surprises.

## Core posture

- Deterministic behavior over flexibility.
- If assumptions are not met, refuse clearly instead of guessing.
- Start with strict constraints; loosen only when there is proven need.

## Current assumptions

- Full clone workflow (not single-branch clone).
- Remote `origin` exists and is the canonical remote.
- `refs/remotes/origin/HEAD` is available for trunk detection.
- Local git state is clean before mutating commands.

## Non-goals (for now)

- Team/process orchestration (policy engines, approval flows, org workflows).
- GUI/TUI dashboards.
- Multi-forge abstraction as a first-class goal.
- Broad edge-case handling outside this opinionated workflow.

## Evolution

This is a working document. Assumptions are intentionally tight now and may be relaxed over time when real usage justifies it.
