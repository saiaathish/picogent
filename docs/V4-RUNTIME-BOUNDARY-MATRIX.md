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
| `live_provider` | Authenticated connectivity plus a bounded fixed no-tool quality campaign |
| `rendered_platform` | Owned-browser / rendered fixture evidence and cross-platform gaps |
| `hostile_runtime` | Child-env sanitization and broader filesystem TOCTOU |
| `recovery_undo` | Deterministic restart, steering, undo, and recovery contracts |
| `release_authorization` | Overall release-authorization posture |

The connectivity row stays `UNVERIFIED` unless
`PICOGENT_LIVE_PROVIDER_EVIDENCE=1` and
`PICOGENT_LIVE_PROVIDER_ARTIFACT` points at a valid evidence file. The quality
row has a separate opt-in contract:
`PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1` and
`PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT` must point at a valid quality
evidence file. Connectivity evidence never upgrades the quality row.

The latest tip-bound Codex observation at exact merged `main`
`e7c5f04d227c12eede0456d11a5ddcbb6636cee4` is recorded in
[V4-LIVE-PROVIDER-EVIDENCE-E7C5F04.md](V4-LIVE-PROVIDER-EVIDENCE-E7C5F04.md)
for [#497](https://github.com/saiaathish/picogent/issues/497). With the matching
digest-only artifacts supplied, it projects both
`live-provider-connectivity=PASS` and `live-provider-quality=PASS` for the
fixed no-tool campaign after canonical prompt-digest and bounded-shape checks;
provider identity and raw result semantics are not independently attested. The
matching rendered allow→undo→reload observation is
[V4-RENDERED-RECOVERY-EVIDENCE-E7C5F04.md](V4-RENDERED-RECOVERY-EVIDENCE-E7C5F04.md).
Historical quality-only records such as
[V4-LIVE-PROVIDER-QUALITY-EVIDENCE.md](V4-LIVE-PROVIDER-QUALITY-EVIDENCE.md)
remain unchanged. Hostile-TOCTOU, cross-platform rendered, and
release-authorization rows remain independently evidence-bound.

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
- `live-provider-quality` remains `UNVERIFIED` when only this connectivity
  artifact is supplied. One successful response does not measure answer
  quality, streaming, tool use, authentication refresh, recovery, or sustained
  behavior.

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

## Fixed live-provider quality evidence

The quality row measures a small fixed no-tool campaign separately from the
connectivity row. Its artifact uses schema
`picogent.v4.live-provider-quality-evidence.v1` and campaign
`fixed-no-tool-v1`. The campaign has three stable case identities:

| Case | Prompt used for the digest-only observation |
| --- | --- |
| `exact-token` | `Reply with exactly LIVE_PROVIDER_QUALITY_OK. Do not call tools, inspect files, or modify anything.` |
| `bounded-summary` | `In exactly one sentence, explain what Picogent is for. Do not call tools, inspect files, or modify anything.` |
| `constraint-following` | `Return exactly three words: local first agent. Do not call tools, inspect files, or modify anything.` |

The prompt text and provider result are not retained in the artifact. The
loader binds each prompt digest to the canonical prompt text above; result
digests, provider identity, and the provider's semantic answer remain
self-reported because raw output and credentials are intentionally not retained.
Each case retains only lowercase SHA-256 digests, a bounded latency measurement,
its verdict, and explicit `tools_used` / `mutation_observed` assertions. A
complete passing artifact has this shape:

```json
{
  "schema": "picogent.v4.live-provider-quality-evidence.v1",
  "campaign": "fixed-no-tool-v1",
  "candidate_sha": "<full lowercase commit SHA>",
  "provider": "codex",
  "environment": "task-owned-disposable",
  "latency_budget_ms": 5000,
  "latency_max_ms": 300,
  "observed_at": "2026-09-06T12:00:00Z",
  "verdict": "PASS",
  "cases": [
    {
      "id": "exact-token",
      "prompt_sha256": "<64 lowercase hex characters>",
      "result_sha256": "<64 lowercase hex characters>",
      "latency_ms": 100,
      "verdict": "PASS",
      "tools_used": false,
      "mutation_observed": false
    },
    {
      "id": "bounded-summary",
      "prompt_sha256": "<64 lowercase hex characters>",
      "result_sha256": "<64 lowercase hex characters>",
      "latency_ms": 300,
      "verdict": "PASS",
      "tools_used": false,
      "mutation_observed": false
    },
    {
      "id": "constraint-following",
      "prompt_sha256": "<64 lowercase hex characters>",
      "result_sha256": "<64 lowercase hex characters>",
      "latency_ms": 200,
      "verdict": "PASS",
      "tools_used": false,
      "mutation_observed": false
    }
  ]
}
```

Run the campaign only with a task-owned disposable home and workspace, then
retain the artifact outside the checkout:

```sh
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-live-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)"
```

`PASS` requires all three fixed cases exactly once, canonical prompt digests,
valid result digests, every case within the declared latency budget, every case
marked `PASS`, and explicit false no-tool/no-mutation assertions. A provider
outage or incomplete campaign may remain `INCONCLUSIVE` or `UNVERIFIED`; the
matrix does not infer either state as `PASS`. Malformed, stale-SHA,
secret-shaped, trailing, oversized, or workspace-contained artifacts fail
closed. The PASS is an artifact-level self-reported observation, not independent
provider or result attestation. This campaign does not prove
streaming quality, authentication refresh, tool behavior, recovery, sustained
sessions, rendered behavior, cross-platform behavior, or release readiness.

## Rendered-platform evidence

The rendered rows are also split by claim size:

- `rendered-platform-local` can reflect one valid task-owned observation on the
  current OS and architecture.
- `rendered-cross-platform` remains `UNVERIFIED` unless
  `PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1` and
  `PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT` supply a valid aggregation for
  darwin, linux, and windows at the exact candidate SHA. See
  [V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md). One local
  platform record cannot stand in for the other supported platforms.

The latest direct current-main local observation is recorded in
[V4-RENDERED-RECOVERY-CURRENT-MAIN-EVIDENCE.md](V4-RENDERED-RECOVERY-CURRENT-MAIN-EVIDENCE.md)
for [#492](https://github.com/saiaathish/picogent/issues/492). It proves only
the `darwin/arm64` rendered recovery path; cross-platform behavior remains
`UNVERIFIED`.

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
