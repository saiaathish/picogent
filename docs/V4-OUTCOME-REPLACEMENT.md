# Explicit outcome replacement

Ordinary follow-ups keep the durable outcome and definition of done. A small
set of direct commands replaces that contract:

- `replace the current goal with ...`
- `replace the current task with ...`
- `cancel the previous task; instead ...`

The payload must infer a concrete task. Questions, quoted instructions,
hypotheticals, negations, and bare `instead` are not replacement commands.
Recognized commands with invalid task payloads stop before provider dispatch.
Combining replacement with retention of the current workspace goal is rejected
because the two contracts would conflict.

Replacement retains task/session identity, cumulative attempts, changed files,
constraints, turn history, and useful historical evidence. It advances intent
revision even for identical outcome text. Before new criteria are assigned,
all old evidence bindings and verification authority are retired. Historical
check results are retained, not rewritten as failures. Fresh proof is required.

An active workspace goal is retired only by its exact admitted text/revision.
The replacement task is saved first with a pending retirement marker, then the
old goal is conditionally cleared, then the exact marker is acknowledged via
task CAS. Restart retries both crash windows. A newer goal, even with identical
text, cannot be cleared; persistence failure prevents provider dispatch.
Acknowledged markers are removed so restoring the session does not reject a
later independently published workspace goal. Automatic goal inference does
not publish replacement commands before this transaction.

The new outcome remains in the existing durable task model; no second new goal
is introduced. This slice does not claim durable user-exclusion enforcement or
intermediate stale-turn mutation protection. Those remain separate work under
issue #760. Local regression tests and hosted CI do not establish live-provider
quality, long-horizon outcome success, or v4 release readiness.
