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

## Hostile-runtime evidence

The hostile rows separate deterministic controls from the broader race claim:

- `hostile-filesystem-deterministic` can become `PASS` when the exact-head
  [bounded hostile-runtime evidence](V4-HOSTILE-RUNTIME-EVIDENCE.md) is present.
  It covers the deterministic securefile, procenv, and workspace test families
  named by that record.
- `hostile-filesystem-toctou` remains `UNVERIFIED`. Descriptor/handle-anchored
  operations and bounded ancestor-swap tests do not prove arbitrary same-UID
  writers cannot race every cross-surface pathname boundary.

The deterministic row is a narrower observation and does not authorize a
release, upgrade the live-provider or rendered rows, or change the explicit
limits in the evidence record.

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

## Rendered-platform evidence

The rendered rows are also split by claim size:

- `rendered-platform-local` can reflect one valid task-owned observation on the
  current OS and architecture.
- `rendered-cross-platform` remains `UNVERIFIED`; one macOS, Windows, or Linux
  record cannot stand in for the other supported platforms.

Enable the narrow local row with an evidence artifact outside the checkout:

```sh
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-rendered-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)"
```

The artifact uses schema `picogent.v4.rendered-platform-evidence.v1` and stores
only bounded identity and digests:

```json
{
  "schema": "picogent.v4.rendered-platform-evidence.v1",
  "candidate_sha": "<full lowercase commit SHA>",
  "platform": "darwin",
  "architecture": "arm64",
  "environment": "task-owned-disposable",
  "browser": "browseros-neo",
  "fixture": "rendered-recovery",
  "observation_sha256": "<64 lowercase hex characters>",
  "screenshot_sha256": "UNRECORDED",
  "observed_at": "2026-09-06T12:00:00Z",
  "verdict": "PASS",
  "source_tree_modified": false
}
```

The loader requires the exact candidate SHA and the current matrix host's
platform/architecture, a task-owned disposable environment, valid fixture and
browser identifiers, digest-only observation and screenshot references, an
explicit clean-source assertion, and one of the bounded verdicts. Unknown or
trailing fields, oversized/malformed records, workspace-contained or symlinked
artifact paths, dirty-source assertions, and records from another host fail
closed. A valid `FAIL`, `INCONCLUSIVE`, or `UNVERIFIED` record remains that
verdict in the local row; the loader never infers `PASS` from artifact presence
alone.

The record is evidence of the named fixture on the named host only. It does
not store or prove DOM output, screenshots, URLs, credentials, provider
quality, unsupported-platform behavior, hostile filesystem races, recovery, or
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
