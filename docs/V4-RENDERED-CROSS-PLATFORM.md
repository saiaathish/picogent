# v4 rendered cross-platform evidence contract

Status: contract plus packaging helper. Digest-only aggregate `PASS` exists at
exact candidate SHA `cddb184cc90de13423749aeb021440f016194a33` (post-#541 tip;
digest `b9c4f7ad364c0f2c191c568ee7aea971a3a540a718785f84bb7a4583e6b5151b`) and
historically at `18dc4a1ca1137ab78dfd0102848eb99c417653cc` (issue
[#507](https://github.com/saiaathish/picogent/issues/507)). This record belongs to
[#500](https://github.com/saiaathish/picogent/issues/500) under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

A single darwin BrowserOS observation, the HTTP API-boundary fixture, and
hosted CI alone are not cross-platform owned-browser proof. The Windows row
below is a real task-owned Playwright Chromium observation, not an API-boundary
substitute.

Tip three-platform collection at behavior SHA
`cddb184cc90de13423749aeb021440f016194a33` (after #541):

- Darwin: Playwright Chromium owned-browser `PASS`
  ([Darwin](V4-RENDERED-RECOVERY-EVIDENCE-CDDB184.md)).
- Linux: task-owned Chromium `PASS`
  ([Linux](V4-RENDERED-LINUX-EVIDENCE-CDDB184.md)).
- Windows: hosted Playwright Chromium `PASS` via
  `rendered-windows-owned-browser` run
  [34086505365](https://github.com/saiaathish/picogent/actions/runs/34086505365)
  ([Windows](V4-RENDERED-WINDOWS-EVIDENCE-CDDB184.md)).

The three-input packager retained aggregate
`rendered-cross-platform-evidence.json` with `verdict=PASS` and digest
`b9c4f7ad364c0f2c191c568ee7aea971a3a540a718785f84bb7a4583e6b5151b`. Exact-SHA
matrix validation at a clean `cddb184…` checkout projected
`rendered-cross-platform=PASS` with tip matrix
`PASS:9 / INCONCLUSIVE:1 / UNVERIFIED:1` (hostile TOCTOU only remaining
UNVERIFIED; release-authorization stays INCONCLUSIVE). Later non-docs tip
merges (`#542`, `#544`, …) invalidate tip continuity until three platforms are
recollected at the new tip; this SHA remains the stable exact-candidate
aggregate.

Historical three-platform collection at behavior SHA
`18dc4a1ca1137ab78dfd0102848eb99c417653cc`:

- Darwin: direct BrowserOS neo observation `PASS`, recorded in
  [the Darwin evidence record](V4-RENDERED-RECOVERY-EVIDENCE-18DC4A1.md).
- Linux: direct task-owned Chromium observation `PASS`, recorded in
  [the Linux evidence record](V4-RENDERED-LINUX-EVIDENCE-18DC4A1.md).
- Windows: direct task-owned Playwright Chromium observation `PASS`, recorded in
  [the Windows evidence record](V4-RENDERED-WINDOWS-EVIDENCE-18DC4A1.md).

The three-input packager retained aggregate
`rendered-cross-platform-evidence.json` with
`verdict=PASS` and digest
`e519f22aa878c33479ee7e85282a8e00e4767d68560a9ff537601c1803a23ef5`. Exact-SHA
matrix validation at a clean `18dc4a1…` checkout projected
`rendered-cross-platform=PASS`. The row remains exact-candidate bound: tip
checkouts must still supply that aggregate against candidate SHA `18dc4a1…`
(or recollect all three platforms at a later tip). Non-docs tips `#524` and
`#527` broke live/local-rendered `--behavior-sha` continuity from `18dc4a1…`;
behavior tip `37d9206…` refreshed Darwin/Linux locally without tip Windows;
`cddb184…` then completed the three-platform tip aggregate above. Hostile
TOCTOU and release authorization stay unchanged.

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
authorize release. [#507](https://github.com/saiaathish/picogent/issues/507)
is closed on the `18dc4a1…` three-platform acceptance.

## PASS requirements

- `required_platforms` is exactly `darwin`, `linux`, and `windows`
- every required platform has one entry with `verdict=PASS`
- every entry shares the matrix candidate SHA, uses
  `environment=task-owned-disposable`, and stores digests only
- `source_tree_modified` is explicitly `false`
- incomplete coverage may remain `UNVERIFIED` / `INCONCLUSIVE`; malformed,
  dirty, stale, or contradictory evidence fails closed

## Explicit non-claims

- does not invent placeholder Windows/Linux BrowserOS observations
- does not treat `TestRenderedRecoveryFixtureAPIBoundary` as owned-browser proof
- does not upgrade live-provider, hostile-TOCTOU, or release-authorization rows
- the aggregate PASS at `18dc4a1…` does not by itself authorize a release
