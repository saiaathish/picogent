# v4 live-provider connectivity evidence at exact tip `2166004`

Status: the exact-head runtime-boundary matrix records `PASS` for the single
bounded live-provider connectivity observation. This is not evidence of
provider quality, rendered behavior, hostile-filesystem safety, release
authorization, or v4 completion.

This checkpoint belongs to [#641](https://github.com/saiaathishkarthik/picogent/issues/641)
under runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).

## Provenance

| Field | Value |
| --- | --- |
| Candidate SHA | `216600450a68c4603f8e2460279688cc56f08c0b` |
| Behavior provenance | `EXACT_HEAD` |
| Head match / tree | `PASS` / `CLEAN` |
| Provider | `codex` |
| Runtime | `go1.26.6 darwin/arm64` |
| Environment | `task-owned-disposable` |
| Observation time | `2026-09-11T00:41:08Z` |
| Matrix generated | `2026-09-11T00:41:42Z` |

The probe ran from the clean exact-tip checkout with a disposable Picogent home
and workspace. It used the existing local Codex login without copying or
retaining credentials. Raw provider output, stderr, and disposable state are
not retained in this repository.

## Observation

| Observation | Prompt SHA-256 | Result SHA-256 | Command wall time | Stderr | Workspace entries | Verdict |
| --- | --- | --- | ---: | ---: | ---: | --- |
| `LIVE_PROVIDER_OK` | `0c4e68894f41a3ce3194b17b931f22128600a4511ab42cfd69f22c17411c5887` | `05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7` | 5755 ms | 0 bytes | 0 | `PASS` |

The result digest covers the observed `LIVE_PROVIDER_OK` line including its
terminal newline. `tools_used=false` and `mutation_observed=false` are
explicit artifact assertions supported by the fixed no-tool prompt, zero
workspace entries, and zero captured stderr bytes.

## Retained artifacts

Secret-free artifacts are retained outside the checkout under the task-owned
directory `/private/tmp/picogent-live-2166004/`:

| Artifact | SHA-256 |
| --- | --- |
| `live-provider-evidence.json` | `ba689e31f561f8398597d92f996aa6a77d620a2878406b3a0e0fdf09507dae3d` |
| `runtime-boundary-matrix.json` | `1e17c01e8d9fe015e9530974dddbf04065f07bfa3950cb240b1f576307710c0d` |

The exact-head matrix recorded:

```text
candidate_sha=216600450a68c4603f8e2460279688cc56f08c0b
behavior_provenance=EXACT_HEAD
head_match=PASS  tree=CLEAN
summary=PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4
live-provider-connectivity=PASS
live-provider-quality=UNVERIFIED
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

## Reproduction boundary

Reproduce from a clean checkout at the exact candidate SHA with a disposable
provider home and workspace. Retain only digest-only JSON outside the checkout.
The matrix flags are opt-in and independent: connectivity evidence does not
upgrade quality, rendered, hostile-runtime, recovery, or release-authorization
claims.

## Explicit limits

This record does not prove:

- provider or account identity beyond runtime selection and artifact
  self-report;
- live-provider quality, streaming, tool use, authentication refresh, or
  sustained sessions;
- rendered local or cross-platform browser behavior;
- arbitrary same-UID filesystem race resistance; or
- release authorization or v4 completion.

The quality and rendered rows remain `UNVERIFIED`, the broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`, and
`release-authorization` remains `INCONCLUSIVE` without the required evidence
and human operator decision.
