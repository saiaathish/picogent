# v4 rendered cross-platform evidence contract

Status: contract plus packaging helper. The current clean candidate
`f004f724a4336f948e765d91c6a985b01eb37505` has a direct hosted Linux
observation in [V4-RENDERED-LINUX-EVIDENCE-F004F72.md](V4-RENDERED-LINUX-EVIDENCE-F004F72.md)
from [#601](https://github.com/saiaathishkarthik/picogent/issues/601); its
`linux/amd64` rendered row is `PASS`. The latest retained Darwin observation is
[V4-RENDERED-RECOVERY-EVIDENCE-4854523.md](V4-RENDERED-RECOVERY-EVIDENCE-4854523.md),
bound to behavior candidate `48545233c0a453c419787a4ca8b06d21741cc31c` from
[#599](https://github.com/saiaathishkarthik/picogent/issues/599) and carried only
through the documented docs-only-descendant continuity rule. No Windows
observation was supplied at the current candidate, so the cross-platform row
remains `UNVERIFIED`. The last retained exact-candidate cross-platform record is [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md),
bound to historical behavior candidate SHA `3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe`.
Its darwin+linux+windows aggregate is `PASS`, with digest
`be790e046ee209b74531dfbfc7c71a6604f6b6401c214a951f2c54932111f7e5`; exact
candidate matrix validation reported `head_match=PASS`, `tree=CLEAN`, and
`rendered-cross-platform=PASS` for that candidate only. The exact-head local
matrix at `48545233c0a453c419787a4ca8b06d21741cc31c` records
`PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4`, with
`rendered-platform-local=PASS` and `rendered-cross-platform=UNVERIFIED`.
The live-provider evidence packet remains separately bound to behavior SHA
`0e2b15624b29c256c4b22364ecf6970890c90374`; it is not silently projected onto
the new candidate. This is evidence for the
rendered cross-platform claim, not release authorization. The record is
exact-candidate bound: documentation-only descendants may retain it, but it
must not be projected onto another candidate SHA. This checkpoint belongs to
[#507](https://github.com/saiaathish/picogent/issues/507) under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

The earlier tip digest-only aggregate PASS remains on record at
`f8a78c646877164009953401772acdb4d346175d` with digest
`5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd`. See
[V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md). `#551` invalidated
behavior-SHA retention from `423d047…`; that historical package re-bound
darwin+linux+windows at its tip. Historical tip digest-only aggregate PASS remains on record at
`423d0471c1864651473c2f1828b67066885f79bd`, plus a prior acceptance aggregate
PASS at `18dc4a1ca1137ab78dfd0102848eb99c417653cc` (the earlier #507
acceptance). The packaging contract remains tracked by
[#500](https://github.com/saiaathish/picogent/issues/500); the current
evidence record is tracked by #507 under #453.

A single darwin BrowserOS observation, the HTTP API-boundary fixture, and
hosted CI alone are not cross-platform owned-browser proof. The Windows row
below is a real task-owned Playwright Chromium observation, not an API-boundary
substitute.

Tip three-platform collection at behavior SHA
`f8a78c646877164009953401772acdb4d346175d`:

- Darwin: direct Playwright Chromium observation `PASS`, recorded in
  [the Darwin evidence record](V4-RENDERED-RECOVERY-EVIDENCE-F8A78C6.md).
- Linux: direct task-owned Chromium observation `PASS`, recorded in
  [the Linux evidence record](V4-RENDERED-LINUX-EVIDENCE-F8A78C6.md).
- Windows: direct task-owned Playwright Chromium observation `PASS`, recorded in
  [the Windows evidence record](V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md)
  (GHA Run 12
  [34112789982](https://github.com/saiaathish/picogent/actions/runs/34112789982)).

The three-input packager published tip aggregate
`rendered-cross-platform-evidence.json` with `verdict=PASS` and digest
`5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd`. Exact-SHA
matrix validation at a clean `f8a78c6…` checkout projected
`rendered-cross-platform=PASS` (PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3).
Docs-only descendants retain that aggregate with
`--behavior-sha f8a78c646877164009953401772acdb4d346175d`; later non-docs tips
must recollect all three platforms (or keep `UNVERIFIED`). Hostile TOCTOU and
release authorization stay unchanged.

Historical tip aggregate at behavior SHA
`423d0471c1864651473c2f1828b67066885f79bd` remains on record with digest
`d5e55136fa1af5c00b37b22d6f06d1b31752e66abe027626c23243ecc3b4702f`
([Darwin](V4-RENDERED-RECOVERY-EVIDENCE-423D047.md),
[Linux](V4-RENDERED-LINUX-EVIDENCE-423D047.md),
[Windows](V4-RENDERED-WINDOWS-EVIDENCE-423D047.md)).

Historical interim tip aggregate at behavior SHA
`cddb184cc90de13423749aeb021440f016194a33` remains on record with digest
`b9c4f7ad364c0f2c191c568ee7aea971a3a540a718785f84bb7a4583e6b5151b`
([Darwin](V4-RENDERED-RECOVERY-EVIDENCE-CDDB184.md),
[Linux](V4-RENDERED-LINUX-EVIDENCE-CDDB184.md),
[Windows](V4-RENDERED-WINDOWS-EVIDENCE-CDDB184.md)).

Historical three-platform collection at behavior SHA
`18dc4a1ca1137ab78dfd0102848eb99c417653cc` remains on record
([Darwin](V4-RENDERED-RECOVERY-EVIDENCE-18DC4A1.md),
[Linux](V4-RENDERED-LINUX-EVIDENCE-18DC4A1.md),
[Windows](V4-RENDERED-WINDOWS-EVIDENCE-18DC4A1.md)) with aggregate digest
`e519f22aa878c33479ee7e85282a8e00e4767d68560a9ff537601c1803a23ef5`.

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
authorize release. #507 remains open for the broader release-audit residuals
even after an exact-candidate rendered evidence rebind.

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
- an aggregate PASS at any candidate does not by itself authorize a release
