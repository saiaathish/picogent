# v4 rendered cross-platform evidence at exact candidate 06e06a1

Status: `PASS` for the rendered cross-platform claim only. This is an exact
candidate evidence record, not a release approval.

The evidence is bound to candidate SHA
`06e06a154cfc7e1d4c3e3e72e4819497a0c9d5f8`, the clean `main` tip produced by
[#556](https://github.com/saiaathish/picogent/pull/556). `main` later advanced
to `dba10f6c8b068685165a8b889072e25dadb2b77b` when
[#557](https://github.com/saiaathish/picogent/pull/557) added the Linux
collection lane. The aggregate remains intentionally bound to its observed
candidate; it is not projected onto the later tip.

This record satisfies the retained-evidence and documentation portion of
[#522](https://github.com/saiaathish/picogent/issues/522). The parent ledger
[#507](https://github.com/saiaathish/picogent/issues/507) and the broader
release-readiness tracks [#450](https://github.com/saiaathish/picogent/issues/450)
and [#453](https://github.com/saiaathish/picogent/issues/453) remain separate.

## Provenance

- Candidate source: clean detached checkout at
  `06e06a154cfc7e1d4c3e3e72e4819497a0c9d5f8`.
- Every seed and reload manifest records
  `source_sha=06e06a154cfc7e1d4c3e3e72e4819497a0c9d5f8`,
  `source_sha_verified=true`, and `source_tree_modified=false`.
- Darwin: task-owned BrowserOS capture on `darwin/arm64`, observed at
  `2026-09-07T17:43:24.845674Z`.
- Linux: fresh hosted `main` workflow dispatch
  [run 34151544649](https://github.com/saiaathish/picogent/actions/runs/34151544649),
  with exact candidate checkout and real Playwright Chromium on
  `linux/amd64`.
- Windows: fresh hosted `main` workflow dispatch
  [run 34147863372](https://github.com/saiaathish/picogent/actions/runs/34147863372),
  with exact candidate checkout and real Chromium on `windows/amd64`.
- The platform records, aggregate, and runtime-matrix outputs remain outside
  the source checkout. No screenshots, DOM, URLs, credentials, or transcripts
  are committed here.

## Platform records

All three records use schema
`picogent.v4.rendered-platform-evidence.v1`, fixture `rendered-recovery`,
environment `task-owned-disposable`, and `verdict=PASS`.

| Platform | Browser | Observation SHA-256 | Screenshot SHA-256 | Platform record SHA-256 |
| --- | --- | --- | --- | --- |
| `darwin/arm64` | `browseros-neo` | `b6feb1aafe92728b152c6b2765bac7c3825cecef564db7e88f2c0ee4a6aaa46f` | `UNRECORDED` | `464bbd8914c7147fa46b400f57ec9fb60599c068941bcbf91edf694b6fef9b41` |
| `linux/amd64` | `playwright-chromium-headless-134.0.6998.35` | `646d87c0d8db1e4190b5bfc3a9f2fff4112b6af13f35dbeb4244f53cb6b0e772` | `5ef4b71cc4fc9344c9ab59729714e1396c399ae78f7aef2720a8500561a3a37c` | `27683f1fa2cbc60e342c53943c670ec32d428acfee072648d28b999e3c5353cd` |
| `windows/amd64` | `chrome-headless-151.0.7922.174` | `3837f3e066731a07579c1676562477467307fa7825aaa0916254d38c22f682a6` | `9f51491944b47e6053654947a052a47bdba29c1d1e55783eeb3774fe7f0210db` | `31496a6f1c0e3c047dcfcca4f6284674f52fdd69d0033e53a55be0f5b1ec043f` |

Operator retention paths:

```text
/private/tmp/picogent-rendered-darwin-06e06a1-evidence/darwin-rendered-platform.json
/private/tmp/picogent-linux-owned-browser-34151544649/rendered-linux-owned-browser-06e06a154cfc7e1d4c3e3e72e4819497a0c9d5f8/picogent-linux-rendered-evidence/linux-rendered-platform-evidence.json
/private/tmp/picogent-windows-owned-browser-06e06a1/windows-rendered-platform.json
```

The Darwin record deliberately retains `screenshot_sha256=UNRECORDED`; that
is a bounded limitation of that capture, not a synthesized screenshot digest.

## Aggregate

The three records were packaged from the clean candidate checkout with
`cmd/rendered-cross-platform-evidence`:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=06e06a154cfc7e1d4c3e3e72e4819497a0c9d5f8
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=29564305dc590e7069693fc69db18fc92413c1b7637eaa457e1dfa2244a29766
```

Retained aggregate:

```text
/private/tmp/picogent-rendered-cross-platform-06e06a1/aggregate-exact-candidate.json
```

The exact candidate runtime matrix was then run with the retained aggregate:

```sh
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/private/tmp/picogent-rendered-cross-platform-06e06a1/aggregate-exact-candidate.json \
  go run ./cmd/runtime-boundary-matrix \
    --workspace . \
    --candidate-sha 06e06a154cfc7e1d4c3e3e72e4819497a0c9d5f8 \
    --out /private/tmp/picogent-rendered-cross-platform-06e06a1/exact-candidate-runtime-boundary-matrix.json
```

The matrix reported:

```text
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
rendered-cross-platform=PASS
runtime-matrix-sha256=47eaa03376e4d542ad87121407d5b097031ecc9e0c24a27eda3f72bd1391dc8d
```

## Boundary

This closes the rendered cross-platform evidence gap at the exact candidate
only. It does not upgrade live-provider connectivity or quality, arbitrary
same-UID filesystem TOCTOU, or overall release authorization. Those remain
`UNVERIFIED` or `INCONCLUSIVE` in the runtime matrix and release audit.
