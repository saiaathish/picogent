# v4 bounded live-provider quality evidence

Status: the exact-head matrix recorded `PASS` for one fixed no-tool quality
campaign observed through a Picogent run configured for Codex. This record
belongs to [#488](https://github.com/saiaathishkarthik/picogent/issues/488)
under the broader runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). The artifact
records provider selection and digest-only results; it is not an independent
attestation of provider identity. It is not a streaming, tool-use, recovery,
cross-platform, security, or release-readiness claim.

## Provenance

```text
repository: github.com/saiaathishkarthik/picogent
source:     67802ddaf761b31d6aec50f289cec474cf0522cc
provider:   codex
campaign:   fixed-no-tool-v1
runtime:    go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:   2026-09-06T09:18:19Z
```

The source checkout was clean and `HEAD` matched the recorded full commit ID.
The command selected `PICOGENT_PROVIDER=codex` and used the host's existing
Codex login from a disposable `PICOGENT_HOME` and workspace. The schema and
artifact do not cryptographically attest the provider process or account
identity. The canonical matrix publishes the fixed prompt text as the test
contract; the external observation directory was cleaned to retain only the
digest-only JSON artifact, with raw responses, credentials, and provider session
data excluded.

The digest-only artifact was retained outside the checkout at:

```text
/private/tmp/picogent-live-quality-observation.dnYuTL/live-provider-quality-evidence.json
```

Artifact SHA-256:

```text
036c8fd8368a592979c36d48f16c20e6b5e617ec81cbfa0e92d91d363fa4e807
```

## Fixed campaign observations

The prompt identities and their exact contracts are defined by the
`fixed-no-tool-v1` section of the
[runtime-boundary matrix](V4-RUNTIME-BOUNDARY-MATRIX.md). The artifact stores
only lowercase SHA-256 digests and bounded metadata.

| Case | Direct result check | Latency | Tools | Workspace mutation | Verdict |
| --- | --- | ---: | --- | --- | --- |
| `exact-token` | Exact one-token response matched `LIVE_PROVIDER_QUALITY_OK` | 3094 ms | none observed | none observed | `PASS` |
| `bounded-summary` | One non-empty line with one sentence | 2459 ms | none observed | none observed | `PASS` |
| `constraint-following` | Exact three-word response matched `local first agent` | 2110 ms | none observed | none observed | `PASS` |

All three cases were below the declared 5000 ms per-case budget. The command
exited successfully for each case, emitted no tool-start markers, and left the
disposable workspace empty.

## Exact-head matrix projection

The matrix was run with the quality artifact enabled against the same clean
source SHA:

```sh
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-live-quality-observation.dnYuTL/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 67802ddaf761b31d6aec50f289cec474cf0522cc
```

The observed provenance was `HEAD=PASS` and `tree=CLEAN`. The matrix summary
was `PASS:6`, `INCONCLUSIVE:1`, `UNVERIFIED:4`, with these relevant rows:

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `live-provider-quality` | `PASS` | The three fixed no-tool cases passed with exact-head, digest-only evidence for the Codex-selected run; provider identity is not independently attested. |
| `live-provider-connectivity` | `UNVERIFIED` | Connectivity is a separate artifact and was not inferred from quality evidence. |
| `rendered-platform-local` | `UNVERIFIED` | No rendered-platform artifact was supplied by this campaign. |
| `rendered-cross-platform` | `UNVERIFIED` | One macOS provider run cannot represent other supported platforms. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | The provider campaign does not exercise hostile filesystem writers. |
| `release-authorization` | `INCONCLUSIVE` | A bounded quality PASS does not authorize a release. |

## Explicit limits

This record does not prove:

- streaming quality, tool-use quality, authentication refresh, or sustained sessions;
- provider process or account identity beyond the runtime selection and artifact self-report;
- restart, steering, undo, rendered GUI/TUI/headless behavior, or cross-platform behavior;
- arbitrary same-UID filesystem race resistance or production security;
- v3-v4 comparative quality or release authorization.

The parent runtime-boundary issue and release-readiness issue remain open until
their larger evidence requirements are independently satisfied.
