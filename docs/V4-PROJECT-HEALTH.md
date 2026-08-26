# V4 project health diagnosis

`project_health` is a small internal capability for broad requests such as
“make this ready to launch” or “get this repo healthy.” It uses one fresh
`repomap` snapshot and returns a bounded JSON report with:

- recognized project shape and inferred commands;
- Git head and dirty-path provenance when available;
- fixed health dimensions such as build, tests, runtime, security, performance,
  and release;
- ranked observations with evidence and a suggested next action.

The capability is read-only. It does not run build, test, lint, package-manager,
browser, deployment, or model commands. It does not persist a project index,
start a watcher, or claim that an inferred command passes. `UNKNOWN` and
`UNVERIFIED` are deliberately retained so the existing verifier and live tools
remain the completion authority.

The priority number is only a deterministic ordering aid. It combines the
importance of the observation and the cost of leaving it unresolved; it is not
a probability, health percentage, or security rating. Repository filenames and
manifest-derived values remain untrusted data and are redacted before the
bounded tool result is returned.

## Acceptance contract

The slice is acceptable only when:

1. a fresh snapshot is used for each diagnosis;
2. no project command is executed by the diagnosis path;
3. unavailable Git or incomplete inventory evidence stays unknown/attention;
4. findings are bounded and sorted deterministically;
5. the tool does not expose a new user-facing mode or dashboard;
6. the result remains separate from task completion and verification proof.

## Evidence boundary

Local package tests prove report construction, ordering, redaction, bounds, and
tool registration. They do not prove live provider quality, browser behavior,
cross-platform runtime behavior, or that a project is launch-ready. Those
claims still require the existing targeted/broader verification and the final
v4 release campaigns.
