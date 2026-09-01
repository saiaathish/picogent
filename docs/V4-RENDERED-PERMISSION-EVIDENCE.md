# v4 rendered permission and side-effect evidence

Status: size:S contract and fixture slice for [#291](https://github.com/saiaathish/picogent/issues/291), under the authoritative Outcome Engine work in [#246](https://github.com/saiaathish/picogent/issues/246). This document does not close #291 and does not claim the medium permission-to-mutation or large undo/restart lanes are complete.

## Boundary

The rendered journey must reuse the existing Safe/Fast permission gate, native
workspace tools, taskstate outcome/evidence model, GUI SSE/state projection, and
checkpoint-backed undo. The S lane owns only the narrow contract and
deterministic fixture seam:

1. A task-owned rendered GUI shows the Safe-mode decision for `write_file`
   before the fixture performs any write.
2. A deny decision returns `perm.Deny` and leaves the contained probe path
   absent.
3. An allow decision returns `perm.Allow`; only then does the fixture write the
   exact expected bytes to the contained probe path.
4. The GUI permission endpoint clears the matching pending request after the
   decision and does not accept a cross-origin mutation request.
5. Verification, undo, and restart claims require their own direct observations
   and stay `UNVERIFIED` when the fixture cannot observe them.

The executable fixture is
`internal/gui/rendered_permission_contract_test.go`. It exercises the existing
`/api/permission` route through the loopback origin check and models the
side-effect boundary without adding a second planner, daemon, index, or
user-facing workflow.

## Bounded observation format

Future rendered runs should record one bounded object per decision. The values
below are the required fields, not permission to omit unknown observations:

```json
{
  "issue": "291",
  "parent_issue": "246",
  "surface": "rendered_gui",
  "fixture": {
    "browser": "task_owned_browser_tab",
    "provider": "local_deterministic_stub",
    "mode": "safe"
  },
  "request": {
    "tool": "write_file",
    "relative_path": "rendered-probe.txt",
    "expected_bytes": 27
  },
  "decision": "deny|allow",
  "workspace": {
    "exists_before": false,
    "exists_after": "false|true|UNKNOWN",
    "content_matches": "false|true|UNKNOWN"
  },
  "rendered": {
    "permission_visible": "true|false|UNKNOWN",
    "changed_files": "0|1|UNKNOWN",
    "verification": "PASS|FAIL|INCONCLUSIVE|UNVERIFIED",
    "undo_available": "true|false|UNKNOWN"
  },
  "provenance": {
    "source_sha": "<exact observed source SHA or UNRECORDED>",
    "runtime_binary": "<exact binary identity or UNRECORDED>",
    "observed_at": "<UTC timestamp>"
  },
  "status": "PASS|FAIL|UNVERIFIED"
}
```

`PASS` is permitted only when every required field for that case is directly
observed and the result is bound to the current contained workspace. An
unknown, missing, stale, or unrecorded field remains `UNVERIFIED`; a local
provider fixture is never evidence of live-provider quality.

## Candidate local rendered observation

On 2026-09-01, a disposable task-owned BrowserOS tab used Safe mode, a local
OpenAI-compatible provider stub, and a contained probe file. The exact source
SHA of the GUI binary was not recorded, so this is candidate runtime evidence,
not a clean-head release result.

| Case | Direct observation | Status |
| --- | --- | --- |
| Permission before mutation | The rendered page showed the Safe-mode permission controls for `write_file` before the probe write. | `CONFIRMED` |
| Deny | Denying the request left `rendered-probe.txt` absent. | `CONFIRMED` |
| Allow | Allowing the request produced `rendered-probe.txt` with exactly `rendered permission probe\n` (27 bytes). | `CONFIRMED` |
| Rendered mutation projection | The page represented the mutation as `Edited 1 file`. | `CONFIRMED` |
| Verification | The page reported `verify INCONCLUSIVE ... reason=no test runner found`; no verification `PASS` was claimed. | `CONFIRMED` |
| Undo | The durable workspace contained a sealed undo journal, but the rendered state reported `undo_available=false`, exposed a disabled Undo control, and returned no task projection. The undo mutation was not exercised. | `UNVERIFIED` |
| Restart recovery | No direct post-mutation restart-and-reload observation was captured. | `UNVERIFIED` |

The undo mismatch is intentionally retained as evidence for the M/L follow-up,
not papered over by treating a journal file as equivalent to a user-visible
undo action. Live-provider behavior, cross-platform rendered behavior,
arbitrary hostile filesystem races, and release readiness remain `UNVERIFIED`.

## Validation

The S-lane checkpoint was validated on branch
`codex/v4-rendered-permission-contract` at source commit
`e85e8d8`. The focused GUI package command passed after the
no-side-effect assertion was added:

```text
go test ./internal/gui -count=1
ok  github.com/saiaathish/picogent/internal/gui  15.529s
```

Fresh local validation at `e85e8d8` also passed:

```text
go test ./... -count=1
go test -race ./internal/gui -count=1
go vet ./...
git diff --check
```

The test proves the decision ordering and contained side-effect behavior at the
existing API boundary. It does not replace the BrowserOS observation required
by the medium lane or the undo/restart proof required by the large lane.
