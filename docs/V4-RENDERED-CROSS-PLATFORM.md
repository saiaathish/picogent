# v4 rendered cross-platform evidence contract

Status: contract only. The runtime-boundary row `rendered-cross-platform`
stays `UNVERIFIED` until a digest-only aggregation artifact proves PASS for
darwin, linux, and windows at one exact candidate SHA. This record belongs to
[#500](https://github.com/saiaathish/picogent/issues/500) under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

A single darwin BrowserOS observation, the HTTP API-boundary fixture, and
hosted CI alone are not cross-platform owned-browser proof.

## Matrix projection

```sh
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/absolute/path/outside/checkout/rendered-cross-platform-evidence.json \
  go run ./cmd/runtime-boundary-matrix \
    --workspace . \
    --candidate-sha <exact-clean-candidate-sha>
```

The artifact schema is `picogent.v4.rendered-cross-platform-evidence.v1`.

## PASS requirements

- `required_platforms` is exactly `darwin`, `linux`, and `windows`
- every required platform has one entry with `verdict=PASS`
- every entry shares the matrix candidate SHA, uses
  `environment=task-owned-disposable`, and stores digests only
- `source_tree_modified` is explicitly `false`
- incomplete coverage may remain `UNVERIFIED` / `INCONCLUSIVE`; malformed,
  dirty, stale, or contradictory evidence fails closed

## Explicit non-claims

- does not invent Windows/Linux BrowserOS observations
- does not treat `TestRenderedRecoveryFixtureAPIBoundary` as owned-browser proof
- does not upgrade live-provider, hostile-TOCTOU, or release-authorization rows
