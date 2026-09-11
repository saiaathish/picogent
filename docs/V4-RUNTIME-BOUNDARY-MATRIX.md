# V4 runtime-boundary evidence matrix

Status: this matrix catalogs remaining release-readiness runtime claims with
explicit `PASS` / `FAIL` / `INCONCLUSIVE` / `UNVERIFIED` verdicts. It does not
authorize a release and never treats mocks or local builds as live-provider
proof.

**Current main behavior-evidence note (candidate `2166004` / issue `#641`):**
The exact current main tip is
`216600450a68c4603f8e2460279688cc56f08c0b`. Post-merge CI run
`34546564353` and release-artifacts run `34546564358` both passed at that
SHA. The last non-documentation behavior candidate is
`eccebd293e58ec48c6553e9228ff4b201a74c77a`; the retained bounded goal-state
records from that candidate are documented in
[V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md) and remain valid for
the current documentation-only descendants.

A fresh exact-head no-tool connectivity observation at this candidate is
documented in
[V4-LIVE-PROVIDER-EVIDENCE-2166004.md](V4-LIVE-PROVIDER-EVIDENCE-2166004.md).
Its exact-head matrix digest is
`1e17c01e8d9fe015e9530974dddbf04065f07bfa3950cb240b1f576307710c0d`, with
summary PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4. It is limited to
`live-provider-connectivity` and does not supply quality or rendered evidence.

No fresh live-provider or rendered-platform observation is bound to the
current behavior candidate after the goal-state hardening changes beyond this
single connectivity probe. The older live-provider packets and rendered
aggregates below remain historical and must not be unioned or projected onto
this candidate. Accordingly, the current posture is
`live-provider-connectivity=PASS`,
`live-provider-quality=UNVERIFIED`, `rendered-platform-local=UNVERIFIED`,
`rendered-cross-platform=UNVERIFIED`, broad
`hostile-filesystem-toctou=UNVERIFIED`, and
`release-authorization=INCONCLUSIVE`. The bounded goal-state PASS is a named
subsystem observation and does not upgrade the broader hostile-runtime row.

The post-merge continuity matrix captured at candidate `db477d1` is recorded in
[V4-LIVE-PROVIDER-CONTINUITY-DB477D1.md](V4-LIVE-PROVIDER-CONTINUITY-DB477D1.md).
It validates the retained live-provider rows through the documented
`DOCS_ONLY_DESCENDANT` rule; it does not rebind or upgrade the remaining
rendered, hostile-TOCTOU, or release-authorization rows.

See the [historical Darwin rendered evidence record](V4-RENDERED-DARWIN-EVIDENCE-AD34BD5.md),
the [historical Windows rendered evidence record](V4-RENDERED-WINDOWS-EVIDENCE-AD34BD5.md),
the [historical Linux rendered evidence record](V4-RENDERED-LINUX-EVIDENCE-AD34BD5.md),
and the [exact-candidate aggregate record](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md)
and the [historical exact-tip live-provider evidence record](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).
The prior `f901e8f`, `492595f`, `1531850`, `97a3ec5`, `3f5543d`, and `1ce2059`
packets and their live/browser records remain historical and are not silently
projected onto this candidate.

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

## Behavior-SHA continuity across evidence docs

Live connectivity, live quality, and local rendered-platform artifacts may
remain bound to an earlier behavior revision when the current candidate is a
docs-only descendant:

```sh
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)" \
  --behavior-sha <artifact-source-full-commit-id>
```

The matrix still requires a clean worktree and exact
`HEAD == --candidate-sha`. It proves with Git that `--behavior-sha` is an
ancestor and that every path touched by every intervening commit is under
`docs/` (a later revert does not erase a non-docs touch). The report records both SHAs and
`behavior_provenance=DOCS_ONLY_DESCENDANT`; omitting `--behavior-sha` preserves
the exact-head contract and records `EXACT_HEAD`.

Any code, test, workflow, build, or other non-`docs/` path change after the
behavior SHA rejects collection. A missing/non-ancestor SHA, unavailable or
oversized Git diff, malformed artifact, artifact SHA mismatch, or dirty tree
also fails closed. The exception applies only to:

- `live-provider-connectivity`
- `live-provider-quality`
- `rendered-platform-local`

It does not rebind or upgrade `rendered-cross-platform`,
`hostile-filesystem-toctou`, the verification manifest, production artifacts,
release attestations, or `release-authorization`; those remain exact-candidate
or independently evidence-bound.

The historical `264fbde2b45609e6e85e175efe216e8f63509ab8` artifacts cannot be
attached to the merge containing this contract because this contract itself
changes Go code and tests after that SHA. The clean continuation path is one
fresh observation at the merged contract SHA. Its evidence documentation may
then advance `main` without invalidating those artifacts, provided all
intervening commits remain confined to `docs/`.

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

The historical tip-bound Codex observation at exact behavior tip
`1ce2059fc352b5d32b5da3adbaeb1ecb48834db7` is recorded in
[V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md](V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md)
for [#453](https://github.com/saiaathish/picogent/issues/453). With the matching
digest-only artifacts supplied, it projects both
`live-provider-connectivity=PASS` and `live-provider-quality=PASS` for the
fixed no-tool campaign after canonical prompt-digest and bounded-shape checks;
provider identity and raw result semantics are not independently attested.
The matching Darwin/Linux/Windows rendered observation is recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md)
for [#507](https://github.com/saiaathish/picogent/issues/507). Hostile TOCTOU
and release-authorization rows remain independently evidence-bound and are not
upgraded by either aggregate.

Historical interim tip aggregate at `cddb184…` (post-#541; digest
`b9c4f7ad364c0f2c191c568ee7aea971a3a540a718785f84bb7a4583e6b5151b`) remains on
record ([live](V4-LIVE-PROVIDER-EVIDENCE-CDDB184.md),
[Darwin](V4-RENDERED-RECOVERY-EVIDENCE-CDDB184.md),
[Linux](V4-RENDERED-LINUX-EVIDENCE-CDDB184.md),
[Windows](V4-RENDERED-WINDOWS-EVIDENCE-CDDB184.md)); it is not the current tip.

## Hostile-runtime evidence

The hostile rows separate deterministic controls, bounded parent-swap
confinement, and the broader race residual:

- `hostile-filesystem-deterministic` can become `PASS` when the exact-head
  [bounded hostile-runtime evidence](V4-HOSTILE-RUNTIME-EVIDENCE.md) is present.
  It covers the deterministic securefile, procenv, and workspace test families
  named by that record.
- `hostile-parent-swap-confinement` can become `PASS` when both
  [Darwin](V4-HOSTILE-PARENT-SWAP-DARWIN.md) and
  [Linux](V4-HOSTILE-PARENT-SWAP-LINUX.md) same-UID parent-swap confinement
  evidence records are present. Partial single-platform evidence stays
  `INCONCLUSIVE`. This row is the hostile-runtime release gate for bounded
  confinement; it is not a universal TOCTOU proof.
- `hostile-filesystem-toctou` remains `UNVERIFIED` as an explicit residual.
  Descriptor/handle-anchored operations and bounded ancestor-swap tests do not
  prove arbitrary same-UID writers cannot race every cross-surface pathname
  boundary. The release-authorization predicate does not require this residual
  row to become `PASS`. Formal residual-acceptance packaging for operators is
  [V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md)
  ([acceptance stub](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md)); it does not
  upgrade this row to `PASS`.

The deterministic and parent-swap rows are narrower observations and do not
authorize a release, upgrade the live-provider or rendered rows, or change the
explicit limits in the evidence records.

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

The latest local rendered observation is recorded in
[V4-RENDERED-DARWIN-EVIDENCE-AD34BD5.md](V4-RENDERED-DARWIN-EVIDENCE-AD34BD5.md)
at `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`; Darwin/arm64 is evidenced
there.
The last retained exact-candidate cross-platform observation is recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md)
for [#507](https://github.com/saiaathish/picogent/issues/507). Darwin, Linux,
and Windows each passed the same allow→undo→fresh-process reload flow at the
exact candidate `3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe`. Exact-SHA matrix
projection is `rendered-cross-platform=PASS` when that candidate aggregate is
supplied; later docs-only descendants must not project it onto a different
candidate SHA. The exact candidate `ad34bd5` now has a fresh Darwin local row
and a separate darwin/linux/windows aggregate with `verdict=PASS`, recorded in
[V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md). The aggregate
remains bound to `ad34bd5`; a later non-docs candidate requires recollection.
With the
Darwin/Linux parent-swap docs present, `hostile-parent-swap-confinement`
projects `PASS`; residual `hostile-filesystem-toctou` stays `UNVERIFIED` and
`release-authorization` stays `INCONCLUSIVE` without operator approval. The
tip-bound final release + hostile residual packet and operator decision
checklist are
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) and
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md); formal
TOCTOU residual acceptance is
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md). They do
not authorize a release.

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
