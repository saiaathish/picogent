# V4 exact behavior-evidence packet: `3f5543d`

Status: **RENDERED CHECKPOINT COMPLETE / RELEASE NOT COMPLETE**. This packet
records exact post-#570 evidence for operator review, including the fresh
three-platform owned-browser checkpoint for
[#507](https://github.com/saiaathish/picogent/issues/507). It does not authorize a
release, does not claim `release-authorization=PASS`, and does not close
[#450](https://github.com/saiaathish/picogent/issues/450) or
[#453](https://github.com/saiaathish/picogent/issues/453).

## Candidate and hosted provenance

| Field | Value |
| --- | --- |
| Exact behavior candidate | `3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe` |
| Change | PR [#570](https://github.com/saiaathish/picogent/pull/570), squash merge |
| Collection checkout | clean exact-head checkout in `/private/tmp/picogent-rendered-darwin-3f-clone` |
| CI | [run 34231332200](https://github.com/saiaathish/picogent/actions/runs/34231332200), `push`, success |
| Release artifacts | [run 34231332411](https://github.com/saiaathish/picogent/actions/runs/34231332411), `push`, success |
| Rendered Linux collection | [run 34241951536](https://github.com/saiaathish/picogent/actions/runs/34241951536), success |
| Rendered Windows collection | [run 34241951172](https://github.com/saiaathish/picogent/actions/runs/34241951172), success |
| Retained release-evidence root | `/private/tmp/picogent-evidence-3f5543d/` |
| Retained rendered-evidence roots | `/private/tmp/picogent-rendered-*-3f-*` and hosted run artifacts |

The CI run passed macOS, Ubuntu, and Windows tests, security scanning, and the
dependent `release-evidence` job. Ubuntu also passed the race, hostile
parent-swap, hostile final-path, fuzz, build, and GUI-listening lanes. The
release-artifacts run produced the three supported binary/package/SBOM target
sets.

## Retained artifact identity

| Artifact | GitHub artifact digest or local subject digest |
| --- | --- |
| `verification-manifest-3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe` | `sha256:76bc02d1d8fbe3e9bb16bcd4c26593a9dabd0fbf1c4665211987a1d2e791d4b9` |
| `release-attestation-3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe` | `sha256:084e5917f65ca3d6cbee666b4e3f71ba69600a7da23525cee933bb3605b57554` |
| `hostile-parent-swap-linux-3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe` | `sha256:02b027725552ee0b0a5e65960ccdcb046bd4bf439df7e1b8775329b9a9561e91` |
| `release-artifacts-3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe` | `sha256:ec7bec9ad820c8843795ffbe03a5bf2270a02866e97590a6bd1aa77bc12ddc04` |
| `release-gates.json` | `8b72abdaa0c6ec6e32705285cd54cf2a7c6fdddf3d5033c98a8289cbb1fef158` |
| `verification-manifest.json` | `57e799e9415a6bceb87b047036abd7173e1b2d26f9846726477107fd122f2955` |
| `runtime-boundary-matrix.json` | `2926c24e7c818290ab57c1bfa6caf84a6d11d4a537f66c7e28c3cdc157a84e87` |
| `rendered-cross-platform-aggregate.json` | `be790e046ee209b74531dfbfc7c71a6604f6b6401c214a951f2c54932111f7e5` |
| `release-attestation-predicate.json` | `a12b34f0d528dafccce3c07713e3fd0fb2ebf0a9b7fa2328823164dfbeb1c307` |
| `release-artifact-manifest.json` | `101d6f67b4f210bc237d6a84a63ff7b0a137130fd6f43e6a3f30240d70b4a0da` |

The GitHub artifact digests are the hosted bundle identities returned by the
Actions API. The local subject digests are SHA-256 recomputations over the
downloaded files. `release-artifact.sha256` validated all ten binary, package,
and SBOM subjects; `tar -tzf` validated all three package archives.

## Exact-tip observations

| Claim | Verdict | Evidence boundary |
| --- | --- | --- |
| Verification manifest | **PASS** | Schema `picogent.verify.v1`; exact SHA and clean tree; two recorded checks. |
| Release gates | **PASS** | `test` and `security` ledger entries bind to the exact `push` SHA. |
| Production artifacts / SBOM | **PASS** | Schema `picogent.release-artifact.v1`; Darwin arm64, Linux amd64, and Windows amd64 outputs. |
| Attestation predicate | **PASS** | Hosted CI attestation binds candidate, run `34231332200`, main workflow, and subject digests. |
| `hostile-child-env-sanitization` | **PASS** | Bounded deterministic evidence doc and hosted test lane. |
| `hostile-filesystem-deterministic` | **PASS** | Bounded securefile/procenv/workspace evidence. |
| `hostile-parent-swap-confinement` | **PASS** | Bounded Darwin/Linux evidence; not universal TOCTOU proof. |
| `hostile-filesystem-toctou` | **UNVERIFIED** | Explicit broad same-UID residual remains open. |
| `live-provider-connectivity` | **UNVERIFIED** | No exact-tip live-provider artifact was supplied. |
| `live-provider-quality` | **UNVERIFIED** | No exact-tip fixed quality artifact was supplied. |
| `rendered-cross-platform` | **PASS** | Three task-owned Darwin/Linux/Windows browser observations and the exact-candidate aggregate are recorded in [the rendered evidence record](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md). |
| `rendered-platform-local` | **PASS** | Exact-candidate Darwin owned-browser observation; not a claim about every rendered surface. |
| `rendered-long-horizon-local` | **PASS** | Existing checked-in contract document is present; this is not live browser proof. |
| `rendered-recovery-undo-reload` | **PASS** | Deterministic API-boundary document is present; this is not browser-DOM proof. |
| `restart-steer-undo-recovery` | **PASS** | Deterministic restart/steer/undo contract is documented; live recovery remains outside. |
| `release-authorization` | **INCONCLUSIVE** | The matrix records the separate predicate posture; no operator approval is supplied. |

Matrix summary: **PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3** at exact behavior candidate
`3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe`, with
`behavior_provenance=EXACT_HEAD` and `head_match=PASS`.

## Why older live/browser evidence is not rebound

The prior exact behavior packet at `1ce2059` remains historical. PR #570
changed `internal/verify/release_authorization.go` and its tests, so the
docs-only descendant exception could not carry the prior live-provider or
rendered-browser observations onto this candidate. The fresh records in
[the rendered evidence record](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md)
close the rendered checkpoint at `3f5543d` without projecting stale evidence.

## Release posture

This is an evidence refresh, not release approval. The broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`; the exact-tip live
provider rows remain unverified; and no authenticated human operator approval
for scope `v4-release` was observed. Consequently the release-authorization
predicate remains **`INCONCLUSIVE` / `authorized: false`**.

Parents #450 and #453 remain open. No tag, publish, deploy, or goal-completion
claim follows from this packet.
