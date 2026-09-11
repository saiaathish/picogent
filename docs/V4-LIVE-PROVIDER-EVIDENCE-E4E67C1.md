# v4 live-provider evidence at tip e4e67c1

Status: exact-head matrix `PASS` for direct live-provider connectivity and
the bounded `fixed-no-tool-v1` quality campaign at source
`e4e67c15dea4ad5fe87b3349da3bc4dc230cf0a3`. This checkpoint belongs to
[#688](https://github.com/saiaathishkarthik/picogent/issues/688) under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).

This is a bounded task-owned observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile same-UID TOCTOU resistance, or release
authorization.

## Provenance

```text
repository:  github.com/saiaathishkarthik/picogent
source:      e4e67c15dea4ad5fe87b3349da3bc4dc230cf0a3
source_ref:  main after PR #687
provider:    codex
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-11T18:20:59Z
```

The source checkout was clean and `HEAD` matched the recorded full commit ID.
The binary was built from that checkout with the low-resource settings
`GOMAXPROCS=2 GOFLAGS=-p=1`. The observation used a disposable Picogent home,
Codex home, workspace, and a task-owned copy of the existing Codex login.
Credentials and provider session state were removed after collection. Raw
provider responses were not retained.

Digest-only artifacts were retained outside the checkout:

```text
/private/tmp/picogent-live-688-final/live-provider-evidence.json
/private/tmp/picogent-live-688-final/live-provider-quality-evidence.json
/private/tmp/picogent-live-688-final/runtime-boundary-matrix.json
```

Artifact SHA-256 values:

```text
connectivity 8c401d30898ef6fd171cf07579907110bd2a87c36a488c42681b5a58178cfa67
quality      198e0a23e77f1425d989077eac7b5dcd75041511d76f7a98fe9d83e75f27bd51
matrix       6be4f28d3a68322e6d6a525fc625a3f4269a46b056672c899fbc74169f828339
```

## Observations

The fixed connectivity prompt returned the required token with result digest
`c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` in
`1803 ms`. The command exited successfully, emitted no stderr, used no tools,
and left the disposable workspace empty.

The fixed quality campaign used a `5000 ms` per-case budget:

| Case | Result SHA-256 | Latency | Tools | Workspace mutation | Verdict |
| --- | --- | ---: | --- | --- | --- |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 3002 ms | none observed | none observed | `PASS` |
| `bounded-summary` | `990b5257e4a65fd9cf4710c89c32c0eaf0825ee547cf2b64e2702228eae5af04` | 2744 ms | none observed | none observed | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2772 ms | none observed | none observed | `PASS` |

All cases were one line, the bounded-summary case contained one sentence, all
three stayed below the budget, emitted no stderr, and left zero workspace
entries. Provider identity and result semantics remain self-reported by the
digest-only artifact.

## Exact-head matrix

The matching artifacts were supplied to the matrix from the clean checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-live-688-final/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-live-688-final/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha e4e67c15dea4ad5fe87b3349da3bc4dc230cf0a3 \
  --out /private/tmp/picogent-live-688-final/runtime-boundary-matrix.json
```

The matrix recorded:

```text
HEAD=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
PASS: 8  INCONCLUSIVE: 1  UNVERIFIED: 3
```

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `live-provider-connectivity` | `PASS` | Direct fixed-prompt response from the task-owned Codex run, with no tools or workspace mutation observed. |
| `live-provider-quality` | `PASS` | Complete canonical fixed no-tool campaign; provider identity and raw result semantics are not independently attested. |
| `rendered-platform-local` | `UNVERIFIED` | No current rendered-platform artifact was supplied by this campaign. |
| `rendered-cross-platform` | `UNVERIFIED` | No current three-platform aggregate was supplied by this campaign. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad same-UID TOCTOU remains an explicit residual outside bounded parent-swap evidence. |
| `release-authorization` | `INCONCLUSIVE` | Green hosted gates and bounded provider evidence do not replace the remaining evidence or explicit operator approval. |

This record does not authorize a release or close the parent issue.
