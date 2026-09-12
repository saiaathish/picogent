# v4 live-provider evidence at exact main `8b88662`

Status: the exact-head runtime-boundary matrix records `PASS` for live-provider
connectivity and the bounded `fixed-no-tool-v1` quality campaign at source
`8b88662661c8acea973163007a776d198a9e7fae`. This child belongs to [#710](https://github.com/saiaathish/picogent/issues/710)
under the broader runtime-boundary parent [#453](https://github.com/saiaathish/picogent/issues/453).
It is not release authorization or a v4-complete claim.

## Provenance

| Field | Value |
| --- | --- |
| Candidate and behavior SHA | `8b88662661c8acea973163007a776d198a9e7fae` |
| Head / tree | `PASS` / `CLEAN` |
| Behavior provenance | `EXACT_HEAD` |
| Provider selection | `codex` |
| Model selection | `gpt-5.6-luna` |
| Runtime | `go1.26.6 darwin/arm64` |
| Environment | `task-owned-disposable` |
| Connectivity observed | `2026-09-12T02:09:08Z` |
| Quality campaign observed | `2026-09-12T02:08:35Z` |

The binary was built from the clean exact-head checkout with
`GOMAXPROCS=2`, `GOFLAGS=-p=1`, and `GOTOOLCHAIN=local`. Connectivity and each
quality case used a disposable `PICOGENT_HOME`, an empty workspace, and a
task-owned copy of the existing Codex authentication file. The authentication
copy, raw provider responses, stderr, and disposable session state were
removed after digest generation. Only the secret-free artifacts listed below
remain outside the checkout.

## Connectivity

The fixed prompt was:

```text
Reply with exactly LIVE_PROVIDER_OK. Do not call tools, inspect files, or modify anything.
```

The process exited successfully, returned the exact requested token, emitted
zero stderr bytes, left the workspace empty, and recorded zero persisted
tool-call records. The retained result digest is
`05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7`.

The secret-free connectivity artifact has SHA-256
`7fd39949c0269f310c1ba2df2262c469f4a6a91e6e14abd1d1f9b2a05343a6a6`.

## Fixed no-tool quality campaign

Each case ran serially in its own fresh disposable home and empty workspace.
All three processes exited successfully with zero stderr bytes, zero workspace
entries, and zero persisted tool-call records. Every case stayed below the
declared 5000 ms budget.

| Case | Result digest | Latency | Direct check | Verdict |
| --- | --- | ---: | --- | --- |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 1927 ms | exact `LIVE_PROVIDER_QUALITY_OK` | `PASS` |
| `bounded-summary` | `094d80521b13a469a2e18bea4477c2fb546b25b7e69a763cd940d12af02cce78` | 4056 ms | one non-empty sentence | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 2567 ms | exact `local first agent` | `PASS` |

The secret-free quality artifact has SHA-256
`093df311537f4556a5812ec44de48efa0c04237c19f3158d696544c9f3fe078f`.

## Exact-head matrix

The retained matrix artifact has SHA-256
`1447ec53b37da233840360c29261c22ec2256eedc65dd74d667435fa005627ba` and
projects:

```text
HEAD=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
PASS: 2  UNVERIFIED: 10
live-provider-connectivity=PASS
live-provider-quality=PASS
```

The following rows remain explicitly `UNVERIFIED` because this campaign does
not exercise them:

- hostile child-environment sanitization;
- deterministic hostile filesystem behavior;
- broad same-UID filesystem TOCTOU;
- hostile parent-swap confinement;
- release authorization;
- rendered cross-platform, local, recovery, and long-horizon behavior; and
- restart, steering, undo, and recovery.

## Reproduction

The three retained artifacts are outside the checkout at:

```text
/private/tmp/picogent-live-710.75vrla/artifacts/live-provider-evidence.json
/private/tmp/picogent-live-710.75vrla/artifacts/live-provider-quality-evidence.json
/private/tmp/picogent-live-710.75vrla/artifacts/runtime-boundary-matrix.json
```

From a clean checkout at the exact candidate SHA, validate the matrix with:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-live-710.75vrla/artifacts/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-live-710.75vrla/artifacts/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 8b88662661c8acea973163007a776d198a9e7fae \
  --out /private/tmp/picogent-live-710.75vrla/artifacts/runtime-boundary-matrix.json
```

This is a bounded, self-reported observation. It does not independently attest
provider identity or raw result semantics, and it does not prove streaming,
tool-use quality, authentication refresh, sustained sessions, rendered UI,
cross-platform behavior, hostile-runtime safety, recovery, or release
authorization.
