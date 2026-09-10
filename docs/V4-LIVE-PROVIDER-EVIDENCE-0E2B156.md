# v4 live-provider evidence at exact tip `0e2b156`

Status: the exact-head matrix records `PASS` for live-provider connectivity
and the bounded `fixed-no-tool-v1` quality campaign. This is a bounded,
self-reported observation; it does not authorize a release or claim v4
completion.

This checkpoint belongs to [#597](https://github.com/saiaathishkarthik/picogent/issues/597)
under runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).

## Provenance

| Field | Value |
| --- | --- |
| Behavior / candidate SHA | `0e2b15624b29c256c4b22364ecf6970890c90374` |
| Behavior provenance | `EXACT_HEAD` |
| Head match / tree | `PASS` / `CLEAN` |
| Provider / model | `codex` / `gpt-5.6-luna` |
| Runtime | `go1.26.6 darwin/arm64` |
| Environment | `task-owned-disposable` |
| Observation timestamp | `2026-09-10T03:59:05Z` |

The binary was built from the clean exact-tip checkout with
`PICOGENT_PROVIDER=codex`, `PICOGENT_MODEL=gpt-5.6-luna`, and
`PICOGENT_ROUTER=off`. Connectivity and each quality case ran in a fresh
process with disposable Picogent state and workspace. The temporary
credential copy, raw provider output, stderr, and session state were removed
after digest generation.

## Observations

| Observation | Result SHA-256 | Latency | Stderr | Workspace entries | Verdict |
| --- | --- | ---: | ---: | ---: | --- |
| Connectivity (`LIVE_PROVIDER_OK`) | `05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7` | 4346 ms | 0 bytes | 0 | `PASS` |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 2217 ms | 0 bytes | 0 | `PASS` |
| `bounded-summary` | `90ef1c2d5988519ca3924d56d7e13fd1ce2dea582e119d0ca36736e4af468c02` | 2592 ms | 0 bytes | 0 | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 3689 ms | 0 bytes | 0 | `PASS` |

The three quality cases stayed below the declared 5000 ms per-case budget.
The exact-token and three-word cases matched their canonical contracts. The
summary case produced one non-empty line ending in sentence punctuation.
`tools_used=false` and `mutation_observed=false` are explicit operator
assertions in the retained quality artifact; provider identity and raw result
semantics are not independently attested.

## Retained artifacts

The secret-free artifacts remain outside the checkout under the task-owned
directory `/private/tmp/picogent-live-0e2b156/`:

| Artifact | SHA-256 |
| --- | --- |
| `live-provider-evidence.json` | `c828289b94b98dc1a7018065b8b49ec893ea01764d9c14b7929f8f0c917746d0` |
| `live-provider-quality-evidence.json` | `d97daa457c5ab4c134b92fb7c47f35a81f54cf57cb716763c416b14e0ccea0cd` |
| `runtime-boundary-matrix.json` | `824e1072b4d7fc9acf9ef00b683b376632febf8556c1ded2282eb486510fe0a9` |

The matrix was generated against the same clean exact tip and recorded:

```text
candidate_sha=0e2b15624b29c256c4b22364ecf6970890c90374
behavior_provenance=EXACT_HEAD
head_match=PASS  tree=CLEAN
summary=PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-platform-local=UNVERIFIED
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

## Docs-only continuity refresh at `2dd8e1b`

After the rendered evidence landings and the split-audit documentation merge,
the matrix was rerun at the exact clean `main` tip
`2dd8e1bb6b395d7b14bca4d45bf2b4fc53f93fb0`, retaining this packet's behavior
SHA `0e2b15624b29c256c4b22364ecf6970890c90374`. Git proved
`behavior_provenance=DOCS_ONLY_DESCENDANT`; `head_match=PASS` and `tree=CLEAN`
were also recorded.

The retained current-main continuity matrix is
`/private/tmp/picogent-continuity-main-2dd8e1b/runtime-boundary-matrix.json`
with SHA-256
`764a940da63d5003be6a6f3d302752ffbaa230010ab9f23a859a26cf60e94235`. It
records **PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3**, with both live-provider
rows still `PASS` and both rendered rows still `UNVERIFIED`. This is a
continuity refresh for the live-provider claim family only. It does not rebind
the rendered `ad34bd5` aggregate and must not be unioned with that aggregate
to authorize a release.

## Reproduction boundary

Reproduce from a clean checkout at the exact candidate SHA with a disposable
provider home and workspace. Keep all referenced artifacts outside the
checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/absolute/path/outside/checkout/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/absolute/path/outside/checkout/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 0e2b15624b29c256c4b22364ecf6970890c90374 \
  --out /absolute/path/outside/checkout/runtime-boundary-matrix.json
```

The matrix flags are opt-in and independent: connectivity evidence does not
upgrade quality, and either live-provider row does not upgrade rendered,
hostile-runtime, recovery, or release-authorization claims.

## Explicit limits

This record does not prove:

- provider or account identity beyond runtime selection and artifact
  self-report;
- streaming, tool-use quality, authentication refresh, or sustained sessions;
- rendered local or cross-platform browser behavior;
- arbitrary same-UID filesystem race resistance; or
- release authorization or v4 completion.

The rendered rows remain `UNVERIFIED`, the broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`, and
`release-authorization` remains `INCONCLUSIVE` without the required operator
decision.
