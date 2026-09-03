# v4 headless direct-process-kill recovery evidence

Status: local fresh-process evidence and hosted platform matrix recorded.

This is the bounded headless slice of [#374](https://github.com/saiaathish/picogent/issues/374),
under the broader run-lock recovery parent [#311](https://github.com/saiaathish/picogent/issues/311)
and outcome parent [#246](https://github.com/saiaathish/picogent/issues/246).

## Source identity

The executable fixture was validated at:

```text
repository: github.com/saiaathish/picogent
source:     1dae1ebbc084da5139215d97ee17a94acc5330ab (fix(test): use Windows executable path in headless fixture)
runtime:    go1.26.6 darwin/arm64
host:       Apple M3 arm64 macOS
```

The lifecycle contract was introduced in the preceding commit
`00462e7bbfe90a4d28084e4c99f47aabbfade8bb` (`test(lifecycle): define headless process-kill contract`).

## Fixture

`TestHeadlessFreshProcessKillRecoversInterruptedTurn` builds the real headless
binary and runs one prompt against a deterministic loopback provider. The
provider barrier is reached only after the active turn is durably admitted.
The test then:

1. confirms the task is working with an active turn;
2. terminates the headless child with `Process.Kill`;
3. starts a fresh test process using the normal application context and the
   same prompt-keyed task session; and
4. checks that recovery records an interrupted turn with `recover` routing,
   `process_restart` stop metadata, `UNVERIFIED` evidence, and a fail-closed
   completion projection.

The test-server cleanup has an explicit release path because an abruptly
terminated client does not guarantee prompt request-context closure on every
local runtime.

## Validation

```text
go test ./cmd/picogent -run '^TestHeadlessFreshProcessKillRecoversInterruptedTurn$' -count=1 -timeout=180s -v  PASS (1.40s)
go test ./cmd/picogent ./internal/lifecycle -count=1 -timeout=180s          PASS
git diff --check                                                         PASS
```

The hosted matrix completed successfully from the exact PR source commit
`1dae1ebbc084da5139215d97ee17a94acc5330ab`:

```text
run:             33719240748 (https://github.com/saiaathish/picogent/actions/runs/33719240748)
test (ubuntu):   PASS
test (windows):  PASS
test (macos):    PASS
security:        PASS
release-evidence: PASS
```

These hosted results are recorded from GitHub's completed check runs; no
hosted result is inferred from the local run.

## Evidence boundary

This proves direct process termination and durable headless recovery through
the existing taskstate/application seams. It does not prove Windows
console-control or `SIGINT` semantics, arbitrary crash timing between
application operations, hostile or uncooperative same-UID filesystem writers,
pathname/TOCTOU race resistance, live-provider quality, rendered behavior, or
release readiness.
