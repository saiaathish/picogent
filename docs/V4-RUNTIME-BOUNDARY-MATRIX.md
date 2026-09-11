# V4 runtime-boundary evidence matrix

Status: this matrix catalogs remaining release-readiness runtime claims with
explicit `PASS` / `FAIL` / `INCONCLUSIVE` / `UNVERIFIED` verdicts. It does not
authorize a release and never treats mocks or local builds as live-provider
proof.

**Current exact-head checkpoint for this refresh:** `main` is clean at
`3535bda907dbe8cc44ca43d5563e4e607a96d266`, the PR #689 merge. The typed
browser screenshot admission remains separately tested; this packet adds
task-owned rendered recovery observations on Darwin, Linux, and Windows. The
fresh exact-head matrix records `HEAD=PASS`, `tree=CLEAN`, and summary
`PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1`. Its artifact SHA-256 is
`f9576b69a41c8d0eb3e036dc8d82b7057f1eb7c842d9ffd5086916408fada2cc`.
Live-provider connectivity and fixed quality plus rendered-platform-local and
rendered-cross-platform are `PASS` under their bounded artifact contracts;
broad hostile-filesystem TOCTOU remains `UNVERIFIED`, and release
authorization remains `INCONCLUSIVE`. The rendered packet is recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3535BDA.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3535BDA.md).
The packet is exact-candidate-bound; any later non-documentation behavior
change requires fresh collection, and later documentation-only descendants
must not silently rebind the observation.

**Prior behavior-bound checkpoint (behavior tip `9c1ca2e`; parent issue `#453`):**
The fresh live-provider observation is bound to behavior tip
`9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb` (PR #661). It was evaluated from
the clean docs candidate
`c3a388571b7c19e3c646ae8480ca23b275d77946` with the documented
`DOCS_ONLY_DESCENDANT` continuity rule. PRs #663, #665, #666, #668, and #670
are documentation-only descendants after that behavior tip.

The new behavior-bound packet is recorded in
[V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md](V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md).
Its digest-only artifacts and matrix are retained outside the checkout under
`/private/tmp/picogent-live-refresh-evidence-c3/`.

The retained three-platform rendered record is documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md).
Its aggregate digest is
`71bfe03c0afb889c9a548e9264cf0ab4509d80f69a85e346fb4501d85a626b05`. The
retained prior-candidate matrix digest is
`6048c4244818db694b96b5875f8893f1d84aaf731a4caebc4493bfbff4213817`, with
summary PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1. Those results apply only to
the retained candidate and its permitted documentation-only descendants.

A fresh exact-current Darwin rendered recovery observation was collected at
candidate `3935b4482f703662acff21f23b205f0763ee1758` and is recorded in
[V4-RENDERED-DARWIN-EVIDENCE-3935B44.md](V4-RENDERED-DARWIN-EVIDENCE-3935B44.md).
It establishes `rendered-platform-local=PASS` for `darwin/arm64`; it does not
upgrade the retained cross-platform aggregate or the broader hostile and
authorization rows.

A fresh exact-current three-platform rendered recovery packet was then
collected at candidate `eabf8d6e35322170f7ab19dbd30d3cfb576f422c` and is
recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md).
It establishes `rendered-platform-local=PASS` and
`rendered-cross-platform=PASS` for that exact candidate only; broad hostile
TOCTOU and release authorization remain independently bounded.

The fresh matrix digest is
`a49c2bbb74d13f7a2053f3cbd131c7fc8aaf6dc32a295ccea2c87e2c79126293`, with
summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3. Its behavior-bound
rows are:

| Current behavior-tip claim | Current result | Boundary |
| --- | --- | --- |
| `live-provider-connectivity` | `PASS` | Fresh digest-only connectivity evidence is bound to `9c1ca2e` and validated through docs-only continuity. |
| `live-provider-quality` | `PASS` | Fresh bounded fixed no-tool quality evidence is bound to `9c1ca2e`; provider identity and raw result semantics remain self-reported. |
| `rendered-platform-local` | `UNVERIFIED` | No fresh local rendered artifact is bound to `9c1ca2e`. |
| `rendered-cross-platform` | `UNVERIFIED` | The retained three-platform packet is bound to `592b07a`, not the current behavior tip. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad same-UID TOCTOU remains an explicit residual. |
| `release-authorization` | `INCONCLUSIVE` | Runtime evidence and hosted CI do not replace operator approval. |

The post-merge continuity matrix captured at candidate `db477d1` is recorded in
[V4-LIVE-PROVIDER-CONTINUITY-DB477D1.md](V4-LIVE-PROVIDER-CONTINUITY-DB477D1.md).
It validates the retained live-provider rows through the documented
`DOCS_ONLY_DESCENDANT` rule; it does not rebind or upgrade the remaining
rendered, hostile-TOCTOU, or release-authorization rows.

See the [behavior-bound live-provider evidence record](V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md),
the [retained prior-candidate rendered cross-platform evidence record](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md),
the [behavior-bound Darwin rendered recovery evidence record](V4-RENDERED-RECOVERY-EVIDENCE-1865A4C.md),
the [fresh exact-current Darwin rendered recovery evidence record](V4-RENDERED-DARWIN-EVIDENCE-3935B44.md),
the [fresh exact-current cross-platform rendered recovery evidence record](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md),
the [historical Darwin rendered evidence record](V4-RENDERED-DARWIN-EVIDENCE-AD34BD5.md),
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

The historical retained three-platform observation is recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md)
for [#646](https://github.com/saiaathishkarthik/picogent/issues/646). Darwin,
Linux, and Windows each passed the same allow→undo→fresh-process reload flow
at exact candidate `592b07a633d9683354c4abeee09b767bc071ab35`; the aggregate
digest is `71bfe03c0afb889c9a548e9264cf0ab4509d80f69a85e346fb4501d85a626b05`.
Exact-SHA matrix projection is `rendered-cross-platform=PASS` only for that
historical candidate aggregate. It is not projected onto the current tip.
The older Darwin, Linux, Windows, and aggregate records remain historical and
are not silently projected onto this candidate. With the Darwin/Linux
parent-swap docs present, `hostile-parent-swap-confinement` projects `PASS`;
residual `hostile-filesystem-toctou` stays `UNVERIFIED` and
`release-authorization` stays `INCONCLUSIVE` without operator approval. The
tip-bound final release + hostile residual packet and operator decision
checklist are
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) and
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md); formal
TOCTOU residual acceptance is
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md). They do
not authorize a release.

The exact-current local refresh at `3935b4482f703662acff21f23b205f0763ee1758`
is the [Darwin rendered recovery record](V4-RENDERED-DARWIN-EVIDENCE-3935B44.md).
Its task-owned BrowserOS neo observation passed the Safe permission, contained
Allow, Undo, and fresh-process reload flow. The retained exact-head matrix for
that run has digest
`0313eb667c1cffbcb9783dbeb2b18f5998157da02f204e1f7cc3bc65b724bc95` and
summary `PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4`; the run supplied no
live-provider artifacts, so those rows remain fail-closed in that snapshot.
The local rendered row is therefore `PASS` only for Darwin/arm64, while
`rendered-cross-platform` remains `UNVERIFIED`.

The fresh exact-current Darwin refresh at
`9a1171942e9041a9e25a75e30e6d9e5dc7d7ad5e` is recorded in
[V4-RENDERED-DARWIN-EVIDENCE-9A11719.md](V4-RENDERED-DARWIN-EVIDENCE-9A11719.md).
Its task-owned BrowserOS neo observation passed the Safe permission, contained
Allow, Undo, and fresh-process reload flow. The platform artifact digest is
`517ebe333f3b80ade0471f94100acabb1b7a4b96e17719255706072fd6bdb94c`; the
exact-head matrix digest is
`c2e11de481c22087d9ac7ca9c8c7db1b40c61b2739d5eccfd9ac6f6d00c1baba` with
summary `PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4`. This rebinds only the local
Darwin rendered row; live-provider, cross-platform, broad TOCTOU, and release
authorization rows remain independently bounded.

The prior exact-current three-platform refresh at
`eabf8d6e35322170f7ab19dbd30d3cfb576f422c` is recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md).
Darwin/arm64, Linux/amd64, and Windows/amd64 each passed the same owned-browser
allow → undo → fresh-process reload flow. The validated aggregate digest is
`e59d54ebd6f75403974744432b03e28ba2725654c4a11e264a868d4842a0f681`, and the
exact-head matrix with local and aggregate artifacts supplied reports
`rendered-platform-local=PASS`, `rendered-cross-platform=PASS`, and summary
`PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3`. The remaining three rows are the two
live-provider claims and broad same-UID TOCTOU; release authorization remains
`INCONCLUSIVE`.

The latest exact-current three-platform refresh at
`3535bda907dbe8cc44ca43d5563e4e607a96d266` is recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3535BDA.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3535BDA.md).
Darwin/arm64, Linux/amd64, and Windows/amd64 each passed the same owned-browser
allow → undo → fresh-process reload flow. The validated aggregate digest is
`d01672f51c5b3121e5a6b740b7ec56dc89f516adf47e5398888869c2ed876d8d`, and the
exact-head matrix with local, aggregate, and exact-current live-provider
artifacts supplied reports `rendered-platform-local=PASS`,
`rendered-cross-platform=PASS`, `live-provider-connectivity=PASS`,
`live-provider-quality=PASS`, and summary `PASS 10 / INCONCLUSIVE 1 /
UNVERIFIED 1`. Broad same-UID TOCTOU remains `UNVERIFIED` and release
authorization remains `INCONCLUSIVE`.

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
