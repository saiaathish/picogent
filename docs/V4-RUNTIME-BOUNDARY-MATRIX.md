# V4 runtime-boundary evidence matrix

Status: this matrix catalogs remaining release-readiness runtime claims with
explicit `PASS` / `FAIL` / `INCONCLUSIVE` / `UNVERIFIED` verdicts. It does not
authorize a release and never treats mocks or local builds as live-provider
proof.

## Command

```sh
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha <full-commit-id>
```

Optional exact-SHA retention outside the checkout:

```sh
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha <full-commit-id> \
  --out /absolute/path/outside/checkout/runtime-boundary-matrix.json
```

See [V4-RUNTIME-BOUNDARY-MATRIX-RETENTION.md](V4-RUNTIME-BOUNDARY-MATRIX-RETENTION.md).

The workspace must be clean and `HEAD` must equal `--candidate-sha`. The JSON
artifact uses schema `picogent.v4.runtime-boundary-matrix.v1`.

## Claim categories

| Category | What it covers |
| --- | --- |
| `live_provider` | Authenticated live-provider quality |
| `rendered_platform` | Owned-browser / rendered fixture evidence and cross-platform gaps |
| `hostile_runtime` | Child-env sanitization and broader filesystem TOCTOU |
| `recovery_undo` | Deterministic restart, steering, undo, and recovery contracts |
| `release_authorization` | Overall release-authorization posture |

Live-provider rows stay `UNVERIFIED` unless `PICOGENT_LIVE_PROVIDER_EVIDENCE=1`
and `PICOGENT_LIVE_PROVIDER_ARTIFACT` points at an existing evidence file. Even
then, this matrix does not auto-score provider quality as `PASS`.

## Explicit boundaries

- Credentials must never be inlined into the matrix artifact.
- Unsupported platforms and destructive hostile cases remain fail-closed.
- A `PASS` on one deterministic or local-rendered claim does not upgrade
  cross-platform, live-provider, or overall release-authorization rows.
- `rendered-recovery-undo-reload` is `PASS` only when the automated
  allow→undo→reload API-boundary evidence doc is present; the browser runbook
  alone is not treated as that proof. Browser DOM, live-provider, and
  unsupported-platform claims remain outside the row.
- Parent release-readiness remains evidence-bound: retained matrix artifacts and
  hosted CI do not authorize a release while live-provider, broader hostile
  TOCTOU, and overall authorization gaps stay `UNVERIFIED` / `INCONCLUSIVE`.
  Direct live/rendered/hostile observation continues under
  [#453](https://github.com/saiaathish/picogent/issues/453).
