# v4 live-provider continuity at merged main `8b022e8`

Status: the post-merge runtime-boundary matrix captured against clean `main`
candidate `8b022e8` validates the retained live-provider artifacts from
behavior SHA `b7b5c5c`. This is continuity evidence, not a new provider
session, release authorization, or v4 completion claim.

This checkpoint continues [#453](https://github.com/saiaathishkarthik/picogent/issues/453)
and the exact behavior observation in
[V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).

## Provenance

| Field | Value |
| --- | --- |
| Current candidate SHA | `8b022e82be4b2e8ade0a502c5f373eaf6dc61160` |
| Artifact behavior SHA | `b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5` |
| Behavior provenance | `DOCS_ONLY_DESCENDANT` |
| Head match / tree | `PASS` / `CLEAN` |
| Matrix generated | `2026-09-10T10:15:21Z` |
| Continuity matrix SHA-256 | `3260157d65e09aa43669bc565f76b0912a782fc7a0174fd3e28c3f0b96237ebd` |

Git verified that the behavior SHA is an ancestor of the current candidate
and that every intervening commit only touched `docs/`. The matrix therefore
retains the original artifact provenance instead of rebinding it to the
documentation descendant.

## Result

| Claim | Verdict |
| --- | --- |
| Live-provider connectivity | `PASS` |
| Live-provider quality | `PASS` |
| Rendered long-horizon local | `PASS` |
| Rendered local platform | `UNVERIFIED` |
| Rendered cross-platform | `UNVERIFIED` |
| Hostile filesystem TOCTOU | `UNVERIFIED` |
| Release authorization | `INCONCLUSIVE` |

The complete matrix summary is `PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3`.
The live rows passed by validating the secret-free retained artifacts from the
behavior checkpoint; no live provider credential or raw provider output was
rerun or copied into the repository.

The continuity matrix was retained outside the checkout at:

`/private/tmp/picogent-live-619.R6pSMq/runtime-boundary-matrix-8b022e8.json`

## Reproduction boundary

From a clean checkout at candidate `8b022e8`, with the original artifacts
kept outside the checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/absolute/path/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/absolute/path/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 8b022e82be4b2e8ade0a502c5f373eaf6dc61160 \
  --behavior-sha b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5 \
  --out /absolute/path/runtime-boundary-matrix-8b022e8.json
```

The rendered rows, broad hostile TOCTOU boundary, and release authorization
remain independently bounded. The rendered exact-candidate packet is
recorded in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md)
and must not be combined with this continuity record into a synthetic
release-authorizing matrix.
