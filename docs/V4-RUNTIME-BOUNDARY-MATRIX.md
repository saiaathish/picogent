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
| `live_provider` | Authenticated live-provider connectivity and quality gap |
| `rendered_platform` | Owned-browser / rendered fixture evidence and cross-platform gaps |
| `hostile_runtime` | Child-env sanitization and broader filesystem TOCTOU |
| `recovery_undo` | Deterministic restart, steering, undo, and recovery contracts |
| `release_authorization` | Overall release-authorization posture |

The connectivity row stays `UNVERIFIED` unless
`PICOGENT_LIVE_PROVIDER_EVIDENCE=1` and
`PICOGENT_LIVE_PROVIDER_ARTIFACT` points at a valid evidence file. The quality
row remains `UNVERIFIED`; this matrix never auto-scores provider quality as
`PASS`.

## Live-provider connectivity evidence

The matrix separates one narrow claim from the broader quality claim:

- `live-provider-connectivity` can become `PASS` when a real provider answers a
  fixed, no-tool prompt in a task-owned disposable home and workspace.
- `live-provider-quality` remains `UNVERIFIED`. One successful response does
  not measure answer quality, streaming, tool use, authentication refresh,
  recovery, or sustained behavior.

Run the probe with disposable paths and retain the record outside the checkout.
For example, the provider probe may be run as:

```sh
probe_home="$(mktemp -d /private/tmp/picogent-live-home.XXXXXX)"
probe_workspace="$(mktemp -d /private/tmp/picogent-live-workspace.XXXXXX)"
PICOGENT_HOME="$probe_home" PICOGENT_PROVIDER=codex \
  go run ./cmd/picogent run \
  "Reply with exactly LIVE_PROVIDER_OK. Do not call tools, inspect files, or modify anything." \
  --dir "$probe_workspace" --yes
```

Create a secret-free JSON record from that observation. The prompt and exact
provider result are represented only by lowercase SHA-256 digests:

```json
{
  "schema": "picogent.v4.live-provider-evidence.v1",
  "candidate_sha": "<full lowercase commit SHA>",
  "provider": "codex",
  "environment": "task-owned-disposable",
  "prompt_sha256": "<64 lowercase hex characters>",
  "result_sha256": "<64 lowercase hex characters>",
  "observed_at": "2026-09-06T12:00:00Z",
  "status": "PASS",
  "tools_used": false,
  "mutation_observed": false
}
```

The matrix accepts the record only when the schema, exact candidate SHA,
provider identifier, disposable-environment marker, digests, timestamp, and
no-tool/no-mutation assertions validate. Unknown fields, trailing JSON,
oversized records, workspace-contained paths, and symlinked artifact paths fail
closed. The artifact must be readable from a real parent directory outside the
checkout and must remain available until the matrix command finishes.

Run the matrix against the same clean checkout SHA:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-live-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)"
```

This record proves only direct connectivity for the one fixed prompt on the
observed host. It is not evidence of provider quality, authorization refresh,
tool behavior, rendered UI behavior, cross-platform behavior, recovery, or
release readiness.

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
