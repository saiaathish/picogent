# V4 current-main residual packet at `e770897`

Status: **exact-head rendered-evidence refresh only**. This packet records the
fail-closed posture for parent issue [#453](https://github.com/saiaathishkarthik/picogent/issues/453)
after [#704](https://github.com/saiaathishkarthik/picogent/issues/704). It does
not authorize a release, sign the residual-acceptance record, close the parent,
or claim v4 completion.

## Exact-head binding

| Field | Value |
| --- | --- |
| Candidate SHA | `e770897891541c31d592220ae902c7fbc5b3db70` |
| Candidate state | clean `main`, `HEAD=PASS`, `tree=CLEAN` |
| Behavior provenance | `EXACT_HEAD` |
| Host | `darwin/arm64`, `go1.26.6` |
| Generated at | `2026-09-12T00:40:10Z` |
| Matrix artifact | `/private/tmp/picogent-v4-evidence-704/runtime-boundary-matrix-exact.json` |
| Matrix SHA-256 | `2cce22d26a54418e64f3bb7afead8441c40ab972065ea537ebad1584a639c199` |
| Matrix summary | `PASS 2 / UNVERIFIED 10` |
| Rendered aggregate SHA-256 | `6587cc334e340ea76e917be1f15f3724f54291e0c381627beb3f03f7aace57d9` |

The matrix was generated from a clean checkout at the candidate SHA with the
fresh Darwin platform artifact and the validated Darwin/Linux/Windows
aggregate supplied. No live-provider artifact, operator approval, or broad
hostile-filesystem claim was supplied.

## Current claim posture

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `rendered-cross-platform` | `PASS` | Fresh task-owned Darwin, Linux, and Windows recovery observations at the same exact candidate. |
| `rendered-platform-local` | `PASS` | Fresh Darwin/arm64 owned-browser observation; does not stand in for live-provider or release proof. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad same-UID races remain an explicit residual. |
| `live-provider-connectivity` | `UNVERIFIED` | No live-provider artifact was supplied. |
| `live-provider-quality` | `UNVERIFIED` | No live-provider quality artifact was supplied. |
| `release-authorization` | `UNVERIFIED` | Exact-head audit documentation is stale for this candidate and no operator approval is present. |
| `hostile-child-env-sanitization` | `UNVERIFIED` | Documentation-backed security evidence is stale for this candidate. |
| `hostile-filesystem-deterministic` | `UNVERIFIED` | Documentation-backed hostile-runtime evidence is stale for this candidate. |
| `hostile-parent-swap-confinement` | `UNVERIFIED` | Bounded parent-swap records are not rebound to this candidate. |
| `rendered-long-horizon-local` | `UNVERIFIED` | Existing long-horizon record is stale for this candidate. |
| `rendered-recovery-undo-reload` | `UNVERIFIED` | API-boundary documentation provenance is incomplete for this candidate. |
| `restart-steer-undo-recovery` | `UNVERIFIED` | Existing deterministic recovery documentation is stale for this candidate. |

The rendered PASS rows are independent of live-provider quality, arbitrary
same-UID filesystem TOCTOU, hostile-runtime documentation, and release
authorization. The matrix intentionally preserves those gaps.

## Operator retention

The source checkout contains documentation only. Digest-only platform records,
screenshots, manifests, the aggregate, and the matrix remain outside the
checkout at:

```text
/private/tmp/picogent-v4-evidence-704/darwin/darwin-rendered-evidence-isolated/
/private/tmp/picogent-v4-evidence-704/linux/picogent-linux-rendered-evidence/
/private/tmp/picogent-v4-evidence-704/windows/
/private/tmp/picogent-v4-evidence-704/rendered-cross-platform-evidence.json
/private/tmp/picogent-v4-evidence-704/runtime-boundary-matrix-exact.json
```

The initial Darwin shared-cache build failed its source-provenance check and
is deliberately excluded. The isolated-cache rebuild passed all fixture
checks; no failed attempt is promoted to evidence.

## Reproduction

From a clean checkout at the candidate SHA, with the retained artifacts placed
outside that checkout:

```sh
GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-v4-evidence-704/darwin/darwin-rendered-evidence-isolated/darwin-rendered-platform-evidence.json \
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/private/tmp/picogent-v4-evidence-704/rendered-cross-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha e770897891541c31d592220ae902c7fbc5b3db70 \
  --out /private/tmp/picogent-v4-evidence-704/runtime-boundary-matrix-exact.json
```

A later non-documentation behavior change requires fresh rendered collection.
This packet does not close #453, authorize publication, or make a v4-complete
claim.
