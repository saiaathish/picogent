# Picogent v3 causal verification memory

V2 already learned small habits and playbooks. V3 adds two bounded records for
the cases where a generic note is not enough:

- A failure stores a short trigger, consequence, scrubbed evidence, confidence,
  recency, and failure/resolution counts.
- A passing verification stores the task class, safe path-like targets, and
  observed stages.

These records are advisory hypotheses. The current workspace, permission gate,
and live verifier remain authoritative. Learned routes never supply shell
commands; they only help choose context and conservative verification targets.

Curator limits keep at most six failures and six routes, discard stale resolved
facts, cap evidence, and redact sensitive assignment-style values before save.
Prompt injection is capped with the existing memory budget and only one causal
record plus one route can enter a turn.

Validation for this slice:

- `go test ./internal/evolve ./internal/agent`
- Full repository checks remain required before merge.
