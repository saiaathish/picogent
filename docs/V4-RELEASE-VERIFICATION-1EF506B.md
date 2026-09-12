# v4 exact-main release verification packet

Status: verification `PASS` for the exact clean main candidate below. Release
authorization remains `INCONCLUSIVE`: this packet does not record operator
approval, does not close the broad hostile-runtime residual, and does not
claim v4 completion.

## Candidate and artifacts

```text
repository:       github.com/saiaathish/picogent
candidate-sha:     1ef506be13d0347d820aa0f2b857e627e928f58a
head-match:        PASS
tree:              CLEAN
runtime:           go1.26.6 darwin/arm64
resource-policy:   GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local
schema:            picogent.verify.v1
```

The exact manifest was emitted outside the checkout:

```text
/private/tmp/picogent-release-evidence-708.Pe3KbM/verification-manifest.json
sha256=d8b8e283a41bfa80914ba989665bf7cf9c4f8ce669e4daa6a1a88f4a44e1fdba

/private/tmp/picogent-release-evidence-708.Pe3KbM/verification-coverage.out
sha256=2af8555cc4627e3ee41f27ca79233f1f1fe8bf31e3c136ea75a0ecd5493e86df
```

## Observed result

| Stage | Result | Observation |
| --- | --- | --- |
| Targeted `internal/verify` | `PASS` | 1 package; Go coverage `78.85939036381514%` |
| Broader `go test ./...` | `PASS` | 49 packages; `130.094415792s`; no kill or truncated result |
| Manifest | `PASS` | Exact SHA matched and worktree was clean |

The broader result supersedes the older 90-second killed observations in the
historical audit. The benchmark package remains expensive (about 102 seconds
in the low-resource full-suite run); the longer bounded manifest timeout is
therefore intentional and remains fail-closed if exceeded.

## Remaining release boundaries

This packet is verification evidence, not an authorization predicate. The
current hostile-runtime packet remains behavior-bound to `e770897` as a
documentation-only descendant and records matrix `PASS 3 / UNVERIFIED 9` with
matrix SHA-256
`948bdce8dc39fba7c9d7330b7e33d4d3a66fae990f35b71cda2a66257ee06a02`.

The following remain explicitly unresolved for release authorization:

```text
hostile-filesystem-toctou
live-provider-connectivity
live-provider-quality
rendered-cross-platform
rendered-long-horizon-local
rendered-platform-local
rendered-recovery-undo-reload
restart-steer-undo-recovery
release-authorization (operator approval absent)
```

Historical live-provider and rendered packets are not silently unioned with
this current verification manifest. Any claim that needs those boundaries
requires a separately supplied artifact bound to its behavior SHA.

## Reproduction

```sh
GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local \
  go run ./cmd/verify-manifest \
    --workspace . \
    --expected-sha 1ef506be13d0347d820aa0f2b857e627e928f58a \
    --target internal/verify \
    --coverprofile /private/tmp/picogent-release-evidence-708.Pe3KbM/verification-coverage.out \
    --targeted-timeout 45s \
    --timeout 15m
```

No release, v4-complete, or parent-issue closure claim follows from this
packet. The operator checklist and the formal hostile-TOCTOU residual package
remain the governing human gates.
