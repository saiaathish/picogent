# V4 exact-tip evidence at f901e8f

Status: **NOT COMPLETE**. This packet records a bounded exact-head runtime
observation. It does not authorize a release or claim that Picogent v4 is
complete.

## Provenance

| Field | Value |
| --- | --- |
| Candidate SHA | `f901e8fa1a7de08ef8c5f4f9db5246b7b7d64a25` |
| Behavior SHA | `f901e8fa1a7de08ef8c5f4f9db5246b7b7d64a25` |
| Behavior provenance | `EXACT_HEAD` |
| Head match | `PASS` |
| Worktree | `CLEAN` |
| Retained artifact | `/private/tmp/picogent-evidence-f901e8f/runtime-boundary-matrix.json` |
| Artifact SHA-256 | `6e7cc3a996bc69b7cdcab7d7d735c2f6d359bf1a8920d112aafab59e4a877774` |

The artifact was collected with the current clean checkout using:

```sh
GOMAXPROCS=2 go run ./cmd/runtime-boundary-matrix \
  --workspace /private/tmp/picogent-v4-next \
  --candidate-sha f901e8fa1a7de08ef8c5f4f9db5246b7b7d64a25 \
  --out /private/tmp/picogent-evidence-f901e8f/runtime-boundary-matrix.json
```

## Matrix result

Summary: **PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5**.

| Claim | Verdict |
| --- | --- |
| `hostile-child-env-sanitization` | `PASS` |
| `hostile-filesystem-deterministic` | `PASS` |
| `hostile-filesystem-toctou` | `UNVERIFIED` |
| `hostile-parent-swap-confinement` | `PASS` |
| `live-provider-connectivity` | `UNVERIFIED` |
| `live-provider-quality` | `UNVERIFIED` |
| `release-authorization` | `INCONCLUSIVE` |
| `rendered-long-horizon-local` | `PASS` |
| `rendered-platform-local` | `UNVERIFIED` |
| `rendered-recovery-undo-reload` | `PASS` |
| `restart-steer-undo-recovery` | `PASS` |

## Evidence boundary

This exact-head collector did not receive live-provider or rendered-platform
artifacts. The broad hostile-filesystem TOCTOU claim remains `UNVERIFIED`, and
release authorization remains `INCONCLUSIVE` without the required operator
decision. The deterministic adaptive-depth quality result from PR #594 is
benchmark evidence only; it does not upgrade any runtime matrix row or permit
a release claim.

The prior `492595f` packet is historical and is not projected onto this tip.
