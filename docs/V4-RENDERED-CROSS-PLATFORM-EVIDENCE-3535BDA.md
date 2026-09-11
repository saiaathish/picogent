# v4 rendered cross-platform evidence at exact candidate `3535bda`

Status: `PASS` for the rendered cross-platform recovery claim only. This
record belongs to [#690](https://github.com/saiaathishkarthik/picogent/issues/690)
under [#453](https://github.com/saiaathishkarthik/picogent/issues/453). It
does not authorize a release, upgrade the broad hostile-filesystem residual,
or claim v4 completion.

## Candidate and common flow

All three observations used the exact clean candidate
`3535bda907dbe8cc44ca43d5563e4e607a96d266`, the merge of PR #689, and the
same task-owned `rendered-recovery` fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the one-file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

Each platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, environment
`task-owned-disposable`, and verdict `PASS`. The retained records contain
bounded identity and SHA-256 values; no DOM, URL, credential, transcript, or
screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `2927638ca0565044ac0bde0c3ba2a5646bebb9ded87f7264494981a0ff43bd02` | `e544f24658f0d50c8ffe0b6679f8071daabe63d3138d39474660b7c91fe7968f` | `c2d800b69127ee3213ed4c7be9c5d3fe04308f9131977d6e6eae310c60f63dc9` |
| `linux/amd64` | [hosted run 34636874454](https://github.com/saiaathishkarthik/picogent/actions/runs/34636874454) | `playwright-chromium-headless-134.0.6998.35` | `e3232d4e77fc47d7869f4790728e04235c08a3f6477cc6d24d58965ec9791a50` | `a39c4d51121142207804d909196c0498d3db5c5a3b9f2a71690e310681d2571f` | `eb4d300ccbbef373987d7b40c4d4eddd86716e3ea8365c128a3af9e620253283` |
| `windows/amd64` | [hosted run 34636876660](https://github.com/saiaathishkarthik/picogent/actions/runs/34636876660) | `chrome-headless-152.0.7977.83` | `b3c63ded8ef40ada2fefe8665f10bebaf6364502b1cfff8c1580b478f13d2877` | `90c1b715768c3c11b475796e2ef9712c5b62285de9db5b44f7cab2d507ff3d4f` | `f526271e7f562f4383838d8a33c735b00d32f9758a113cceefc2def32af83c86` |

All three platform records report `source_tree_modified=false` and the common
candidate SHA. The hosted Linux and Windows collectors completed the same
allow → undo → fresh-process reload flow and uploaded digest-only records plus
screenshots. The Darwin collection used the same fixture contract on the
task-owned local host.

## Aggregate and exact-candidate matrix

The aggregate validator accepted exactly one valid record for each required
platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=3535bda907dbe8cc44ca43d5563e4e607a96d266
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=d01672f51c5b3121e5a6b740b7ec56dc89f516adf47e5398888869c2ed876d8d
```

The exact-head runtime matrix was generated from a clean checkout at the same
candidate with the Darwin local artifact, the cross-platform aggregate, and
the exact-current digest-only live-provider artifacts supplied:

```text
candidate_sha=3535bda907dbe8cc44ca43d5563e4e607a96d266
behavior_sha=3535bda907dbe8cc44ca43d5563e4e607a96d266
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-platform-local=PASS
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
summary=PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1
runtime_matrix_sha256=f9576b69a41c8d0eb3e036dc8d82b7057f1eb7c842d9ffd5086916408fada2cc
```

The live-provider connectivity artifact SHA-256 is
`0ad46528eb82b6ec266da28a3c7b178b8570d0d777e874b29fa8a9520550229a`; the
fixed quality artifact SHA-256 is
`71ce8a4a65d5c14ad386d610efc4b4745481c6b4d4bc968c3124ed8746e785f1`.
The quality campaign observed `2194 ms`, `2624 ms`, and `2431 ms` for its
three cases against the `5000 ms` per-case budget. These rows remain bounded
digest-only observations: provider identity and raw result semantics are not
independently attested.

## Operator retention

The platform records, aggregate, live-provider artifacts, and matrix remain
outside the checkout at:

```text
/private/tmp/picogent-rendered-darwin-690-evidence-final/darwin-rendered-platform-evidence.json
/private/tmp/picogent-rendered-linux-690-artifacts/rendered-linux-owned-browser-3535bda907dbe8cc44ca43d5563e4e607a96d266/picogent-linux-rendered-evidence/linux-rendered-platform-evidence.json
/private/tmp/picogent-rendered-windows-690-artifacts/rendered-windows-evidence-3535bda907dbe8cc44ca43d5563e4e607a96d266/windows-rendered-platform.json
/private/tmp/picogent-rendered-cross-platform-690-aggregate/rendered-cross-platform-evidence.json
/private/tmp/picogent-rendered-cross-platform-690-aggregate/runtime-boundary-matrix-exact.json
/private/tmp/picogent-live-690-final/live-provider-evidence.json
/private/tmp/picogent-live-690-final/live-provider-quality-evidence.json
```

No screenshots, browser transcripts, credentials, raw provider responses, or
scratch checkouts are part of the repository. The failed pre-correction matrix
attempt is retained separately as an audit record and is not used as evidence.

This closes the rendered cross-platform evidence gap for exact candidate
`3535bda` only. A later non-documentation behavior change requires a fresh
three-platform collection. Broad same-UID filesystem TOCTOU remains
`UNVERIFIED`, release authorization remains `INCONCLUSIVE`, and no v4
completion or release claim follows from this record.
