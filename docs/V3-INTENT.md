# Picogent v3 intent contract

Picogent keeps the user-facing interaction simple while carrying a small
internal contract for each task. The contract is inferred from the request and
stored with the durable task, so a resumed turn retains the intended outcome,
task class, completeness expectation, risk, and proof needs.

The contract also produces a bounded definition of done. Criteria become the
task's durable steps and are injected into the agent's internal task context;
they are not exposed as a planning mode or required command vocabulary.

Inference is deterministic and conservative. It marks research, rendered UI
inspection, tests, or approval as needs when the wording makes them relevant.
It does not authorize a risky tool call: the existing permission gate remains
the authority.

Compatibility remains additive. Existing v1 task files load without an intent
contract, while new tasks persist the contract and criteria under the same
task-state version.

Validation for this slice:

- `go test ./internal/taskstate ./internal/agent`
- Full repository checks remain required before merge.

