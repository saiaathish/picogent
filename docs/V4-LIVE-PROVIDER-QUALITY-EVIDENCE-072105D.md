# v4 live-provider quality evidence at current main

Status: the exact-head matrix recorded `PASS` for one bounded fixed no-tool
quality artifact observed through a Picogent run configured for Codex. This
record belongs to [#494](https://github.com/saiaathishkarthik/picogent/issues/494)
under the broader runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). The prior
historical record in
[`V4-LIVE-PROVIDER-QUALITY-EVIDENCE.md`](V4-LIVE-PROVIDER-QUALITY-EVIDENCE.md)
remains unchanged and is not rebound to this source.

This is a bounded self-reported quality observation. It is not an independent
attestation of provider identity or account identity, and it is not a
streaming, tool-use, recovery, cross-platform, security, or release-readiness
claim.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      072105d0c885bf4161291f5eeccd3a3c37a39b0b
source_ref:  origin/main (verified exact match before observation)
provider:    codex
campaign:    fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-06T11:31:26Z
```

The source checkout was clean, `HEAD` matched the recorded full commit ID, and
the authoritative `origin/main` ref matched the same SHA. The run built the
Picogent binary from that checkout and used `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off` with a separate
disposable `PICOGENT_HOME`, disposable per-case workspaces, and a copied
task-owned Codex home. The copied credential was used only for the observation;
the operator removed that temporary credential and the provider session data
after artifact generation. Cleanup is operator-reported and is not an
independent credential-attestation claim.

The retained digest-only artifact is outside the checkout at:

```text
/private/tmp/picogent-live-quality-current.RG6JwK/live-provider-quality-evidence.json
```

Artifact SHA-256:

```text
b3f6fa51e613ac0ac2a638e422736f92e89b00f31af661c39d4af960c9f96f73
```

The artifact path is host-local and is not a portable repository link. A
digest-only copy of the artifact and the matrix summary is recorded in the
conversation for PR #495 so the result can be reviewed after this disposable
workspace is gone. Raw provider responses, stderr captures, credentials, and
provider session data were intentionally not retained. The artifact contains
only canonical prompt digests, result digests, bounded metadata, and explicit
tool/mutation assertions.

## Fixed campaign observations

The prompt identities and exact contracts are defined by the `fixed-no-tool-v1`
section of the runtime-boundary matrix. Every case ran in a fresh process with
`picogent run --yes --dir <disposable-workspace> <prompt>`.

| Case | Result digest | Latency | Tools | Workspace mutation | Verdict |
| --- | --- | ---: | --- | --- | --- |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 3278 ms | none observed | none observed | `PASS` |
| `bounded-summary` | `86aef4adc191119fd6c9216691fda9a9828bfdb005dbd12dc3a27ce08ca48d27` | 2973 ms | none observed | none observed | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 2218 ms | none observed | none observed | `PASS` |

All three cases were below the declared 5000 ms per-case budget. The exact
token and three-word cases matched their canonical output contracts. The
bounded-summary case produced one non-empty sentence. The no-tool and empty
workspace observations are operator-reported from the disposable runs and are
represented by the artifact's self-reported boolean assertions; the discarded
stderr and workspace snapshots are not independently replayable from this PR.

## Exact-head matrix projection

The matrix loader accepted the retained artifact against the same clean source
SHA:

```sh
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-live-quality-current.RG6JwK/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 072105d0c885bf4161291f5eeccd3a3c37a39b0b
```

Observed provenance was `HEAD=PASS` and `tree=CLEAN`. The matrix summary was
`PASS:6`, `INCONCLUSIVE:1`, `UNVERIFIED:4`. The relevant rows were:

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `live-provider-quality` | `PASS` | The self-reported fixed no-tool artifact passed canonical prompt-digest and bounded case checks; provider identity and raw result semantics are not independently attested. |
| `live-provider-connectivity` | `UNVERIFIED` | Connectivity is a separate artifact and was not inferred from quality evidence. |
| `rendered-platform-local` | `UNVERIFIED` | No rendered-platform artifact was supplied by this campaign. |
| `rendered-cross-platform` | `UNVERIFIED` | One macOS provider run cannot represent other supported platforms. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | The provider campaign does not exercise hostile filesystem writers. |
| `release-authorization` | `INCONCLUSIVE` | A bounded quality PASS does not authorize a release. |

## Explicit limits

This record does not prove:

- provider or account identity beyond runtime selection and artifact self-report;
- streaming quality, tool-use quality, authentication refresh, or sustained sessions;
- restart, steering, undo, rendered GUI/TUI/headless behavior, or cross-platform behavior;
- arbitrary same-UID filesystem race resistance or production security;
- v3-v4 comparative quality or release authorization.

The artifact is bound to the pre-PR behavior SHA `072105d0c885bf4161291f5eeccd3a3c37a39b0b`.
It must not be silently rebound to a later documentation or merge SHA. The
parent runtime-boundary issue and release-readiness issue remain open for the
other unresolved evidence boundaries.
