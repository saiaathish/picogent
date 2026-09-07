# v4 live-provider evidence at behavior `f8a78c6`

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at behavior source
`f8a78c646877164009953401772acdb4d346175d` (`#551`). Docs tip
`71c31cadf0538610bd1637621a028f9b4cbb4bda` (`#552`) is a docs-only descendant —
retain with `--behavior-sha f8a78c646877164009953401772acdb4d346175d`.

Belongs to [#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
behavior:    f8a78c646877164009953401772acdb4d346175d (#551)
docs tip:    71c31cadf0538610bd1637621a028f9b4cbb4bda (#552; docs-only)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-07T11:48:09Z
```

The behavior worktree was clean at `f8a78c6…`. The binary was built from that
worktree and run with `PICOGENT_PROVIDER=codex`, `PICOGENT_MODEL=gpt-5.6-luna`,
and `PICOGENT_ROUTER=off`. Each observation used a disposable `PICOGENT_HOME`
and workspace plus a task-owned copy of Codex credentials. The copy, raw
responses, stderr, and provider session state were removed after digest
generation.

## Digests

```text
connectivity df823bd92b76f0016e336cdb9e10262504653ca40ea6311bc73f9a099d4d26f7
quality      2123344da2716d8207c69ca38ccf67ca6d835fe61db5d0210aca4d289fc687cc
exact-head matrix (live+cross)
             f0b4bca1ac626fd559bcfacb4a6e414d70c3eecef8b51dd056dd6f82fa574710
docs-tip retention (nocross, --behavior-sha f8a78c6)
             4a59c0daba15b088b1e19e5a38eed5243561feb4cd14d0a51613843e2b421a4d
```

Host-local artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-f8a78c6/live-provider-evidence.json
/private/tmp/picogent-evidence-f8a78c6/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-f8a78c6/aggregate/runtime-boundary-matrix-full.json
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 3366 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 1903 ms | `PASS` |
| `bounded-summary` | `e72d528b40f080ec10730b96b59acb7a9053d1f39454e045d49253cf68d8d5de` | 2919 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 1971 ms | `PASS` |

All quality cases stayed below the declared 5000 ms budget and recorded
`tools_used=false` and `mutation_observed=false`.

## Exact-head matrix validation

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/darwin/darwin-rendered-platform-evidence.json \
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/aggregate/rendered-cross-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha f8a78c646877164009953401772acdb4d346175d \
  --out /private/tmp/picogent-evidence-f8a78c6/aggregate/runtime-boundary-matrix-full.json
```

Summary at behavior exact head:

```text
PASS: 10  INCONCLUSIVE: 1  UNVERIFIED: 1
behavior_provenance=EXACT_HEAD
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

## Docs-only tip retention

From clean docs tip `71c31ca…` without projecting the exact-candidate aggregate:

```sh
candidate_sha="$(git rev-parse HEAD)"  # 71c31ca…
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-f8a78c6/darwin/darwin-rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$candidate_sha" \
  --behavior-sha f8a78c646877164009953401772acdb4d346175d \
  --out /private/tmp/picogent-evidence-f8a78c6/aggregate/runtime-boundary-matrix-retention-71c31ca-nocross.json
```

```text
PASS: 9  INCONCLUSIVE: 1  UNVERIFIED: 2
behavior_provenance=DOCS_ONLY_DESCENDANT
live + local rendered PASS
rendered-cross-platform=UNVERIFIED  # exact-candidate claim deferred to behavior SHA
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Pointing the `f8a78c6` aggregate at docs-tip candidate fails
`rendered-cross-platform` by design (aggregate `candidate_sha` must match).

## Explicitly unchanged

- `hostile-filesystem-toctou=UNVERIFIED`
- `release-authorization=INCONCLUSIVE` (no operator approval)
- Streaming / tool-use / multi-hour live recovery remain outside PASS rows
- [#450](https://github.com/saiaathish/picogent/issues/450) /
  [#453](https://github.com/saiaathish/picogent/issues/453) stay open
