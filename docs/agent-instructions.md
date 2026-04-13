# Agent Instructions

This file is the source of truth for repository-specific AI agent instructions.

## Required Reading

- [MANIFEST.md](/MANIFEST.md)
- [ASSUMPTIONS.md](/ASSUMPTIONS.md)

## Other notes

- Integration tests are slow on MacOS seemingly due to subprocess spawning overhead; if running on MacOS and have access to docker then run tests using `make test-linux`.
