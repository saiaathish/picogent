# v4 live-provider continuity at merged main `db477d1`

Status: the post-merge runtime-boundary matrix validates the retained
live-provider artifacts from behavior SHA `b7b5c5c` against current clean
`main` `db477d1`. This is continuity evidence, not a new provider session,
release authorization, or v4 completion claim.

This checkpoint continues [#453](https://github.com/saiaathishkarthik/picogent/issues/453)
and the exact behavior observation in
[V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).

## Provenance

| Field | Value |
| --- | --- |
| Current candidate SHA | `db477d17ab0f374e5cc9ae473a0613744023743e` |
| Artifact behavior SHA | `b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5` |
| Behavior provenance | `DOCS_ONLY_DESCENDANT` |
| Head match / tree | `PASS` / `CLEAN` |
| Matrix generated | `2026-09-10T09:27:00Z` |
| Post-merge matrix SHA-256 | `2412100f7fd20e288f0130e813c1efef5c72a5d9fd98886b2e4a378fab2f011c` |

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

Retained artifact digests remain documented in the behavior record. The
post-merge matrix was retained outside the checkout at:

`/private/tmp/picogent-live-619.R6pSMq/runtime-boundary-matrix-post-merge.json`

## Reproduction boundary

From a clean checkout at current `main`, with the original artifacts kept
outside the checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/absolute/path/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/absolute/path/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha db477d17ab0f374e5cc9ae473a0613744023743e \
  --behavior-sha b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5 \
  --out /absolute/path/runtime-boundary-matrix-post-merge.json
```

The rendered rows, broad hostile TOCTOU boundary, and release authorization
remain independently bounded. This record does not upgrade any of them.
