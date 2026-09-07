# v4 rendered cross-platform evidence contract

Status: contract plus packaging helper. The runtime-boundary row
`rendered-cross-platform` stays `UNVERIFIED` until a digest-only aggregation
artifact proves PASS for darwin, linux, and windows at one exact candidate SHA.
This record belongs to
[#500](https://github.com/saiaathish/picogent/issues/500) under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

A single darwin BrowserOS observation, the HTTP API-boundary fixture, and
hosted CI alone are not cross-platform owned-browser proof.

Current collection status at behavior SHA
`d1298998cc000c40ad2fdaf71a24e17649e09e19`:

- Darwin: direct BrowserOS neo observation `PASS`, recorded in
  [the Darwin evidence record](V4-RENDERED-RECOVERY-EVIDENCE-D129899.md).
- Linux: direct task-owned Chromium observation `PASS`, recorded in
  [the Linux evidence record](V4-RENDERED-LINUX-EVIDENCE-D129899.md).
- Windows: `UNVERIFIED`; no owned-browser observation exists.

Consequently, no three-platform aggregate exists and the runtime boundary
remains `UNVERIFIED`.

## Matrix projection

```sh
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/absolute/path/outside/checkout/rendered-cross-platform-evidence.json \
  go run ./cmd/runtime-boundary-matrix \
    --workspace . \
    --candidate-sha <exact-clean-candidate-sha>
```

The artifact schema is `picogent.v4.rendered-cross-platform-evidence.v1`.

## Produce the aggregate artifact

Collect one `picogent.v4.rendered-platform-evidence.v1` record per platform
only after the direct owned-browser flow in
[the recovery fixture runbook](V4-RENDERED-RECOVERY-FIXTURE.md) completes on
that platform. Keep each record outside the checkout. The producer must bind
the record to the exact clean candidate SHA, task-owned disposable environment,
platform/architecture, browser/fixture identity, UTC observation time, and
observation/screenshot digests. Do not copy DOM, screenshots, URLs,
credentials, or browser transcripts into the record.

After all three observations exist, run the packaging command from a clean
checkout at the same candidate SHA:

```sh
candidate_sha="$(git rev-parse HEAD)"
go run ./cmd/rendered-cross-platform-evidence \
  --workspace "$PWD" \
  --candidate-sha "$candidate_sha" \
  --darwin /absolute/path/outside/checkout/darwin-rendered-platform.json \
  --linux /absolute/path/outside/checkout/linux-rendered-platform.json \
  --windows /absolute/path/outside/checkout/windows-rendered-platform.json \
  --out /absolute/path/outside/checkout/rendered-cross-platform-evidence.json
```

The command validates every input, rejects stale/malformed/dirty or
contradictory records, and publishes the aggregate exclusively. It packages
producer-declared architecture values but does not attest that a platform run
actually happened; that provenance belongs to each real task-owned browser
observation. Reusing one platform record under another platform flag fails
closed.

Validate the aggregate through the exact-SHA matrix:

```sh
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/absolute/path/outside/checkout/rendered-cross-platform-evidence.json \
  go run ./cmd/runtime-boundary-matrix \
    --workspace . \
    --candidate-sha "$candidate_sha" \
    --out /absolute/path/outside/checkout/runtime-boundary-matrix.json
```

If any platform observation is missing, use the matrix without the
cross-platform flag and retain `UNVERIFIED`; never synthesize a placeholder
artifact to make the aggregate pass. A successful aggregate still does not
authorize release or close
[#507](https://github.com/saiaathishkarthik/picogent/issues/507).

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
