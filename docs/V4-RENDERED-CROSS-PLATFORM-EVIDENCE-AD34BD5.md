# v4 rendered cross-platform evidence at exact candidate `ad34bd5`

Status: `PASS` for the rendered cross-platform claim only. This exact-candidate
record satisfies [#613](https://github.com/saiaathishkarthik/picogent/issues/613)
under the runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It does not
authorize a release, upgrade the hostile-filesystem residual, establish
live-provider behavior, or claim v4 completion.

## Candidate and common flow

All three observations used the exact clean behavior candidate
`ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c` and the same task-owned rendered
recovery fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

Each platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, fixture `rendered-recovery`,
environment `task-owned-disposable`, and `verdict=PASS`. The platform
observations retain digests only; no DOM, URL, credential, transcript, or
screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | [issue #609](https://github.com/saiaathishkarthik/picogent/issues/609) | `playwright-chromium-headless-151.0.7922.34` | `d5a52ac6638a3fdc1f1f80e5e01c842d483bbe0350b92d2bf9f38dfef1b93899` | `fd582e69e21cd61f4e1c8a275561be21d7eb63fd523da622a668818c8781e6ea` | `fc86cb56692d23cabcfbd9a26c814b20b0b08be28265e8db00556e0000c8d6b8` |
| `linux/amd64` | [hosted run 34444670203](https://github.com/saiaathishkarthik/picogent/actions/runs/34444670203) / [issue #607](https://github.com/saiaathishkarthik/picogent/issues/607) | `playwright-chromium-headless-134.0.6998.35` | `03accda99a641d3911fa1fa0755817e7877bf2c984b647b6883f3951e35c67f0` | `b9e8da2e695234bd1d9e1eae0a79f0d1f45d42af9bf28962a91a80e15bcb7644` | `7e58d5745bf179e78ab38500fa1c706179a763c628a2dd67e7d57eb867e3e069` |
| `windows/amd64` | [hosted run 34444672008](https://github.com/saiaathishkarthik/picogent/actions/runs/34444672008) / [issue #608](https://github.com/saiaathishkarthik/picogent/issues/608) | `chrome-headless-152.0.7977.83` | `847a0f53a0c6a34b40cdb84ca6160499d73b888011de7ff696db5921af5e9d53` | `0836d20e73d94e1ab2f91e492c6485cbcd3e2b694179fc789de88d19fdd99c7d` | `923804ee1c9ec44ae48cd6ce023ec10a34ed07e240e6b4a9f8dce73a54f77485` |

All three platform runs recorded `source_sha_verified=true` and
`source_tree_modified=false`; the seed and reload fixture processes exited
cleanly. The Darwin, Linux, and Windows collectors each returned a `PASS`
platform record at the same candidate SHA.

## Aggregate and exact-candidate matrix

The aggregate validator accepted exactly one valid record for each required
platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=f3def9dc069980125752abbe3d22b7c2ad6a3af8ad74d52091afc38974e35630
```

The aggregate was retained outside the checkout at
`/private/tmp/picogent-rendered-aggregate-ad34/rendered-cross-platform-evidence.json`.
Its observed platform rows were `darwin/arm64`, `linux/amd64`, and
`windows/amd64`, each with `verdict=PASS` and the common candidate SHA.

The exact-head runtime matrix was generated from a clean checkout at the same
candidate SHA with the aggregate supplied:

```text
candidate_sha=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c
behavior_sha=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
rendered-cross-platform=PASS
summary=PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4
runtime_matrix_sha256=0edb2195f34cd53af4aa4574f82f1f3cd864b319361d427a1d179df6b5c1f952
```

The platform-specific matrices separately reported
`rendered-platform-local=PASS` for Darwin/arm64, Linux/amd64, and Windows/amd64.
The aggregate matrix invocation deliberately retained its independent
`rendered-platform-local` row as `UNVERIFIED` because it was supplied the
aggregate artifact, not a local-platform artifact. This does not downgrade the
three platform records or their direct observations.

The same matrix deliberately retains these independent boundaries:

- `hostile-filesystem-toctou=UNVERIFIED`;
- `live-provider-connectivity=UNVERIFIED`;
- `live-provider-quality=UNVERIFIED`; and
- `release-authorization=INCONCLUSIVE`.

## Operator retention

The source checkout contains documentation only. The platform evidence and
aggregate artifacts were retained outside the checkout at:

```text
/private/tmp/picogent-rendered-darwin-609/evidence/
/private/tmp/picogent-rendered-linux-607/rendered-linux-owned-browser-ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c/picogent-linux-rendered-evidence/
/private/tmp/picogent-rendered-windows-608/rendered-windows-evidence-ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c/
/private/tmp/picogent-rendered-aggregate-ad34/rendered-cross-platform-evidence.json
/private/tmp/picogent-rendered-aggregate-ad34/runtime-boundary-matrix.json
```

This closes the rendered cross-platform evidence gap for the exact candidate
`ad34bd5` only. A later non-documentation change requires a fresh
three-platform collection. A later documentation-only descendant may retain
this record as an exact-candidate artifact, but must not project its `PASS`
onto a different candidate SHA. No release or v4-completion claim follows from
this record.
