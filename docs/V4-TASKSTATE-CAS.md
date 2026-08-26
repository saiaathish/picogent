# v4 task-state compare-and-swap experiment

Status: bounded persistence hardening used by durable evidence binding on the
`codex/v4-evidence-binding` branch.

## Contract

Every persisted task now carries a monotonic `Revision`. `Store.Save` and
`Store.SaveIfRevision` lock the store across processes, reread the current
file, and write only when the caller's expected revision still matches. A
successful write advances the revision by one; a stale writer returns
`ErrRevisionConflict` without mutating its in-memory task.

The lock is an OS file lock (`flock` on Unix and `LockFileEx` on Windows),
combined with a process mutex for multiple store values in one process. State
files remain private and atomically replaced. A legacy task with no revision
is treated as generation zero and is claimed once by the first successful
save.

## Why this is separate from freshness

Revisions prevent stale callbacks and concurrent writers from overwriting a
newer persisted state. They do not prove that workspace bytes are unchanged,
do not detect an external A→B→A edit, and do not authorize completion. The
workspace observation and a later evidence-binding slice must supply those
checks.

Tests cover same-process stale saves, legacy generation-zero migration, two
real child processes that load the same revision and race to save it, and
agent publication that keeps a conflicting task revision out of memory.
