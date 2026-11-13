# Metadata storage

The repository module discovers all directories through Git. The metadata store
uses those directories, not the contents or assumed shape of a `.git` path:

| Document | Location |
| --- | --- |
| `state.json` | `<common Git directory>/stack/state.json` |
| `PR_TEMPLATE.md` | `<common Git directory>/stack/PR_TEMPLATE.md` |
| `operation.json` | `<invoking worktree Git directory>/stack/operation.json` |

An ordinary clone retains its existing `.git/stack` paths. Storage-level layout
support does **not** enable linked-worktree CLI mutations: those remain refused
until recovery and branch-ownership safeguards are integrated.

## Compatibility

State remains version 1. Existing naming, PR, lineage, and pending-base metadata
are preserved. Historically defaulted missing fields remain compatible, including
the unversioned state shape. Graph cycles and other graph errors do not prevent
loading state for inspection; mutation planning validates those separately.

New restack records carry storage version 1. A storage version identifies a
schema, not proof that a Git step completed. Existing unversioned operation
records are retained and refused because their evidence is insufficient for
safe automatic recovery. Back up the indicated file, inspect `git status`, finish
or abort the Git operation manually, and reconcile refs and stack metadata before
retiring that record. Do not add a version field to bypass this refusal.

Malformed documents, unsupported versions, invalid modes, and unknown fields are
diagnosed without overwriting the existing document. Missing state and corrupt
state are distinct outcomes. Recovery records are validated before removal too.

## Replacement and durability

Writes use private, same-directory temporary files, file sync, close, atomic
rename, and directory sync. Newly created directories use mode `0700`, files use
`0600`, and creation of directory entries is synced. Existing directory
permissions are not changed. Handled pre-rename failures preserve the old copy
and remove temporary files; cleanup failures report the temporary path.

After rename, a failed directory sync returns an error: the new complete document
is already visible, but crash durability is unconfirmed. Removal similarly
returns directory-sync errors after unlink. Platforms or filesystems that cannot
perform directory sync return an error rather than silently claiming durability.
Guarantees ultimately depend on the underlying filesystem honoring sync/rename.

Atomic replacement protects individual documents, not a collection of files plus
Git refs. It does not serialize concurrent writers or infer recovery progress.
Those guarantees belong to the shared journal/recovery protocol, not this store.
