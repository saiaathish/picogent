# v4 workspace freshness experiment

Status: bounded primitive on the `codex/v4-workspace-freshness` branch; not
yet wired into durable task completion.

## Contract

`internal/workspace.Capture` records a fresh observation of one workspace and
at most 128 explicitly named regular files. It processes at most 512 caller
path inputs; exceeding that input bound is also recorded as truncation. It
stores only:

- the canonical root path and platform filesystem identity;
- each file's existence, identity, size, and SHA-256 digest when the file is
  at most 1 MiB;
- explicit `Known` and truncation state.

Missing files are known observations. Unsafe paths, symlinked file components,
non-regular files, and invalid roots fail closed. Large files and captures
that change while being read remain unknown.

`workspace.Compare` returns `Fresh` only when both captures are complete and
known, the workspace identity is unchanged, every tracked path matches, and
the bounded file digests match. A changed identity or digest is `Changed`;
unknown or truncated state is `Unknown`. A matching digest is evidence about
the observed bytes only; it is not an ABA or authorization mechanism.

## Deliberate limits

This slice does not add a watcher, cache, recursive tree hash, task-state
schema, or completion behavior. The next slice must bind observations to
persisted task revisions and reject stale cross-process writers before any
durable diagnosis or evidence can authorize completion.

The tests cover same-path external rewrites, no-op writes, file creation,
workspace replacement, truncation, oversized files, and unsafe paths. Live
Windows reparse-point behavior and long-running concurrent mutation remain
hosted verification work.
