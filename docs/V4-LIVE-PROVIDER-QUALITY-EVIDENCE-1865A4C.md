# v4 live-provider quality evidence at exact behavior head `1865a4c`

Status: the exact-head runtime-boundary matrix records `PASS` for both the
bounded live-provider connectivity observation and the fixed no-tool quality
campaign. This checkpoint belongs to [#643](https://github.com/saiaathish/picogent/issues/643)
under the broader runtime-boundary parent
[#453](https://github.com/saiaathish/picogent/issues/453). It is not release
authorization or v4 completion.

The observation is deliberately bounded and self-reported at the artifact
level. Provider identity and raw result semantics are not independently
attested. The candidate is the clean behavior head before this documentation
packet; later documentation-only descendants may use `--behavior-sha
1865a4c00356ac9b871b35b3a08f4e983f2af0e1` under the matrix continuity contract.

## Provenance

| Field | Value |
| --- | --- |
| Candidate SHA | `1865a4c00356ac9b871b35b3a08f4e983f2af0e1` |
| Behavior provenance | `EXACT_HEAD` |
| Head match / tree | `PASS` / `CLEAN` |
| Provider | `codex` |
| Campaign | `fixed-no-tool-v1` |
| Runtime | `go1.26.6 darwin/arm64` |
| Environment | `task-owned-disposable` |
| Quality observation | `2026-09-11T01:23:56Z` |
| Connectivity observation | `2026-09-11T01:26:12Z` |
| Matrix generated | `2026-09-11T01:26:38Z` |

The Picogent binary was built once from the clean checkout with
`GOMAXPROCS=2`, `GOTOOLCHAIN=local`, and `GOFLAGS=-p=1`. The three quality
cases and the separate connectivity prompt ran serially using disposable
Picogent homes and empty workspaces. Raw provider output, stderr, credentials,
and disposable session state are not part of the retained evidence records.

## Fixed quality campaign observations

The prompt identities and exact contracts are defined by the `fixed-no-tool-v1`
section of the [runtime-boundary matrix](V4-RUNTIME-BOUNDARY-MATRIX.md). The
retained quality artifact stores only canonical prompt digests, result digests,
bounded latency, and explicit no-tool/no-mutation assertions.

| Case | Result digest | Direct result check | Latency | Tools | Workspace mutation | Verdict |
| --- | --- | --- | ---: | --- | --- | --- |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | Exact `LIVE_PROVIDER_QUALITY_OK` | 2535 ms | none observed | none observed | `PASS` |
| `bounded-summary` | `1f538171e1b2e21e656bac35343942c68527ae78a291cdbdc542b12cb62b9cad` | One non-empty sentence | 3613 ms | none observed | none observed | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | Exact `local first agent` | 2053 ms | none observed | none observed | `PASS` |

All cases were below the declared 5000 ms per-case budget and exited
successfully. Disposable home event logs contained only turn start/end records;
no tool events were observed. Each disposable workspace remained empty.

The separate connectivity prompt returned the exact `LIVE_PROVIDER_OK` result
in 4124 ms, with result digest
`05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7`, zero
stderr bytes, zero workspace entries, and no tool events.

## Retained artifacts

The secret-free artifacts are retained outside the checkout:

| Artifact | SHA-256 |
| --- | --- |
| `/private/tmp/picogent-live-quality-1865a4c.json` | `2dcf42b5d3f696d2a08103b8e8cf104895781ac275a49de6ecaa3f9a4fb53b20` |
| `/private/tmp/picogent-live-evidence-1865a4c.json` | `bb636b09f2dd8cef56c44ccf1098b707ecb8ab7c314c6ad5755f2b56a251bfd0` |
| `/private/tmp/picogent-quality-1865a4c/runtime-matrix.json` | `512f8f2a404584c08b804e989ebe836fe44b0881646a77e12dae243ea2eb88db` |

The older connectivity record for `2166004` remains historical. This packet
uses the refreshed connectivity artifact bound to the same `1865a4c` candidate
as the quality artifact; separate artifacts are not unioned across behavior
SHAs.

## Exact-head matrix projection

The matrix was run from the clean exact-head checkout with both live artifacts
enabled:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-live-evidence-1865a4c.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-live-quality-1865a4c.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 1865a4c00356ac9b871b35b3a08f4e983f2af0e1
```

Observed provenance was `EXACT_HEAD`, with `head_match=PASS` and
`tree=CLEAN`. The matrix summary was `PASS:8`, `INCONCLUSIVE:1`,
`UNVERIFIED:3`:

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `live-provider-connectivity` | `PASS` | One direct fixed-prompt provider response completed without tools or mutation. |
| `live-provider-quality` | `PASS` | The self-reported fixed no-tool artifact passed canonical prompt-digest and bounded case checks. |
| `rendered-platform-local` | `UNVERIFIED` | No rendered-platform artifact was supplied by this campaign. |
| `rendered-cross-platform` | `UNVERIFIED` | One macOS provider run cannot represent other supported platforms. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | The provider campaign does not exercise hostile filesystem writers. |
| `release-authorization` | `INCONCLUSIVE` | A bounded live-provider PASS does not authorize a release. |

## Reproduction boundary

Reproduce the live observations only from a clean checkout at the exact
candidate SHA, with a task-owned disposable provider home and workspace. Once
this documentation packet is merged, use the matrix's docs-only continuity
mode with `--behavior-sha
1865a4c00356ac9b871b35b3a08f4e983f2af0e1` only when every intervening commit
is confined to `docs/`.

## Explicit limits

This record does not prove:

- provider or account identity beyond runtime selection and artifact self-report;
- streaming quality, tool-use quality, authentication refresh, or sustained sessions;
- restart, steering, undo, rendered GUI/TUI/headless behavior, or cross-platform behavior;
- arbitrary same-UID filesystem race resistance or production security; or
- v3-v4 comparative quality, release authorization, or v4 completion.

The parent runtime-boundary issue remains open for rendered-platform evidence,
the broad hostile-filesystem TOCTOU residual, and the human release decision.
