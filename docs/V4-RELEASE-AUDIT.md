# V4 independent release-evidence audit

Status: `INCONCLUSIVE` for release authorization. This is an independently
rechecked evidence report, not a release approval or a supply-chain
certification.

Audit snapshot: 2026-09-02 UTC
Issue: [#321](https://github.com/saiaathish/picogent/issues/321)
Parent: [#316](https://github.com/saiaathish/picogent/issues/316)
Broader parent: [#246](https://github.com/saiaathish/picogent/issues/246)

“Independent” here means a fresh repository-side recheck of downloaded
artifacts and the exact Git history. It does not mean a third-party audit.

## Executive verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| The M workflow published hosted attestations for the two release-evidence JSON subjects | `CONFIRMED` | Directly observed in PR run `33581494767` and post-merge main run `33581839408`. |
| The subjects, candidate commit, predicate, repository, and signer workflow match the recorded main run | `CONFIRMED` | Recomputed digests and local `gh attestation verify` both passed. |
| Required `test` and `security` job results were bound to the main candidate | `CONFIRMED` | `release-gates.json` validates as `PASS` for both jobs at merge SHA `3900ce2…`. |
| The verification manifest proves a clean, fully covered release candidate | `UNVERIFIED` | The hosted manifest reports `tree: DIRTY`, skips targeted coverage, and remains `UNVERIFIED`. |
| A production SBOM and signed production binary exist | `UNVERIFIED` | No SBOM or production binary artifact was present in the inspected run. |
| Picogent is release-ready across providers, hostile runtimes, and rendered platforms | `UNVERIFIED` | Those boundaries remain outside the observed evidence. |

The evidence is strong for the bounded hosted-attestation workflow path. It is
not sufficient to promote the repository to release-ready status.

## Candidate and workflow provenance

The audited candidate is the M lane merged by
[PR #320](https://github.com/saiaathish/picogent/pull/320):

- M source head: `57572188ce49fd6496581fb09214e9bf5b750893`.
- Merge commit: `3900ce229440bd1ece64c9d2cf15960aa471bdcd`.
- Merge parents: `17b7a5b352916a47aa663faf6e0a310acd98543f` and the M source
  head. The source is an ancestor of the merge commit.
- Remote `main` resolved to the merge commit at audit time.
- The workflow checks out the PR source head rather than GitHub’s synthetic
  merge ref for PR evidence, and checks out the pushed SHA for `main`
  ([`.github/workflows/ci.yml:98-103`](../.github/workflows/ci.yml)).
- The release-evidence job runs after `test` and `security`, uses
  `always()`, and validates the required gate ledger before generating the
  manifest ([`.github/workflows/ci.yml:88-162`](../.github/workflows/ci.yml)).
- The attestation publisher is pinned to
  `actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6`; the predicate type
  is
  `https://github.com/saiaathishkarthik/picogent/attestation/release-evidence/v1`
  ([`.github/workflows/ci.yml:163-211`](../.github/workflows/ci.yml)).
- Each subject is verified with the explicit repository, predicate type, and
  exact signer workflow ([`.github/workflows/ci.yml:212-237`](../.github/workflows/ci.yml)).

## Hosted run evidence

Both runs completed with success for security, Ubuntu, Windows, macOS, and the
dependent `release-evidence` job.

### Pull request run

[Run 33581494767](https://github.com/saiaathish/picogent/actions/runs/33581494767)
attested the PR source head `57572188ce49fd6496581fb09214e9bf5b750893`.

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100096558757` | `PASS` |
| `test (ubuntu-latest)` | `100096558886` | `PASS` |
| `test (windows-latest)` | `100096558937` | `PASS` |
| `test (macos-latest)` | `100096558992` | `PASS` |
| `release-evidence` | `100097217254` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-57572188ce49fd6496581fb09214e9bf5b750893` | `9828630729` | 1,000 bytes | present, unexpired |
| `release-attestation-57572188ce49fd6496581fb09214e9bf5b750893` | `9828631109` | 16,575 bytes | present, unexpired |

The PR predicate recorded event `pull_request`, run `33581494767`, and signer
workflow
`saiaathish/picogent/.github/workflows/ci.yml@refs/pull/320/merge`. Its
attestation certificate identified the pull-request workflow execution; this
is distinct from the source candidate SHA by design.

### Post-merge main run

[Run 33581839408](https://github.com/saiaathish/picogent/actions/runs/33581839408)
ran at the exact merge SHA `3900ce229440bd1ece64c9d2cf15960aa471bdcd`.

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100097559715` | `PASS` |
| `test (windows-latest)` | `100097559810` | `PASS` |
| `test (macos-latest)` | `100097559830` | `PASS` |
| `test (ubuntu-latest)` | `100097559911` | `PASS` |
| `release-evidence` | `100098192611` | `PASS` |

| Artifact | Artifact ID | Size | Contents |
| --- | ---: | ---: | --- |
| [`verification-manifest-3900ce2`](https://api.github.com/repos/saiaathish/picogent/actions/artifacts/9828732969) | `9828732969` | 987 bytes | `verification-manifest.json`, `release-gates.json` |
| [`release-attestation-3900ce2`](https://api.github.com/repos/saiaathish/picogent/actions/artifacts/9828733244) | `9828733244` | 16,217 bytes | predicate, subject checksums, and both verification outputs |

Both artifacts were present and unexpired when downloaded for this audit.

## Independent artifact recheck

The post-merge `release-gates.json` contains schema
`picogent.release-gates.v1`, event `push`, candidate SHA
`3900ce229440bd1ece64c9d2cf15960aa471bdcd`, and exactly two required `PASS`
records: `test` with exit code 0 and `security` with exit code 0. The existing
fail-closed validator was run against the downloaded file:

```text
release gates PASS: 2 required job(s) for push
```

The independently recomputed SHA-256 values are:

| Subject | SHA-256 |
| --- | --- |
| `artifacts/release-gates.json` | `dd9cd1c14f8ed536c1867b359c6e3bd71d6ad8ce044b0f2fd738280fb082f019` |
| `artifacts/verification-manifest.json` | `f6fc3aa151bef4b7286bc82540f47eb51ff96b0ceebaba1458099d962d6c86ee` |

The downloaded predicate repeats those exact digests and records:

```text
schema:       picogent.release-attestation.v1
repository:   saiaathish/picogent
candidate:    3900ce229440bd1ece64c9d2cf15960aa471bdcd
event:        push
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
run_id:       33581839408
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
issued_at:    2026-09-02T02:08:42.280635Z
expires_at:   2026-09-09T02:08:42.280635Z
```

The two downloaded verification outputs both contained the same multi-subject
in-toto statement, with the two subject names and digests above. Re-running
the hosted verifier locally against each downloaded subject returned:

```text
release-gates-attestation=PASS
verification-manifest-attestation=PASS
```

The verification result directly exposed:

- predicate type
  `https://github.com/saiaathishkarthik/picogent/attestation/release-evidence/v1`;
- signer certificate subject alternative name
  `https://github.com/saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main`;
- workflow trigger `push`, workflow ref `refs/heads/main`, and workflow SHA
  `3900ce229440bd1ece64c9d2cf15960aa471bdcd`;
- GitHub-hosted runner environment and source repository digest equal to the
  merge SHA; and
- a verified Rekor timestamp at `2026-09-02T02:08:43Z`.

This confirms the hosted Sigstore-backed in-toto attestation path for the two
JSON evidence subjects. It does not make the local Ed25519 envelope in
`internal/verify` a Sigstore bundle; the local contract remains a separate
bounded primitive with ephemeral test keys.

## Verification-manifest result

The main `verification-manifest.json` has:

```text
schema:       picogent.verify.v1
head.sha:     3900ce229440bd1ece64c9d2cf15960aa471bdcd
expected_sha: 3900ce229440bd1ece64c9d2cf15960aa471bdcd
head.match:   PASS
head.tree:    DIRTY
status:       UNVERIFIED
reason:       worktree is not proven clean
platform:     linux/amd64, go1.25.14
```

The manifest recorded one broader check, `go test ./...`, as `PASS` with 41
passed tests, and one targeted check as `SKIPPED` because no safe targeted
command was detected. Coverage was `UNVERIFIED` because coverage was not
collected. This is the intended conservative behavior described by
[`docs/V4-VERIFICATION-MANIFEST.md`](V4-VERIFICATION-MANIFEST.md), not a
reason to rewrite the result as `PASS`.

## SBOM and signing state

- `CONFIRMED`: the inspected post-merge run has exactly the two evidence
  artifact bundles listed above; the release-attestation bundle contains a
  Sigstore-backed attestation predicate, subject checksums, and verifier
  outputs.
- `UNVERIFIED`: no SBOM file or SBOM artifact was present in that run, and no
  production binary release artifact was produced by this workflow.
- `CONFIRMED`: the hosted evidence is signed by the GitHub Actions/Sigstore
  workflow identity and has a verified transparency-log timestamp.
- `UNVERIFIED`: there is no independent production-binary signing key,
  release package signature, or repository-owned trust service in scope.

The existing dependency vulnerability scan is useful reachability evidence; it
is not an SBOM and does not establish complete supply-chain provenance.

## Fresh adversarial review

| Boundary | Result | Finding |
| --- | --- | --- |
| Source SHA versus synthetic PR merge ref | `PASS` | PR evidence records the source head; main evidence records the pushed merge SHA; the merge parent relationship was independently checked. |
| Missing or failed required CI job | `PASS` | `always()` still runs the evidence job, and `ValidateReleaseGateLedger` rejects missing, duplicate, failed, mismatched, nonzero, or truncated records ([`internal/verify/release_gates.go:39-130`](../internal/verify/release_gates.go)). |
| Artifact subject binding | `PASS` | Both JSON subjects, their recomputed SHA-256 digests, predicate, repository, candidate SHA, run, and signer workflow agree. |
| Hosted signer and predicate verification | `PASS` | Both subjects re-verified with the explicit repository, predicate type, and exact signer workflow; certificate and Rekor observations are recorded above. |
| Verification truthfulness | `PASS` | The manifest preserves `UNVERIFIED` for dirty provenance, skipped targeted work, and missing coverage ([`internal/verify/manifest.go:234-277`](../internal/verify/manifest.go)). |
| Fork pull requests | `PASS` | Ordinary gate/manifest evidence remains enabled while write-capable hosted attestation steps are conditionally skipped and therefore remain `UNVERIFIED`. |
| Hosted workflow action immutability | `INCONCLUSIVE` | `actions/attest` is full-hash pinned, but checkout, setup, Node, and upload actions use moving major tags. This is a supply-chain maintenance gap, not a failed attestation for this run. |
| Predicate namespace | `P2 GAP` | The custom predicate URI uses `saiaathishkarthik` while the repository identity is `saiaathish/picogent`. Internal verification agrees on the URI, but the mismatch can confuse external consumers and should be narrowed or documented. |
| Clean release provenance | `P1 GAP` | The evidence job creates files under the checked-out workspace before collecting provenance, so the hosted manifest reports `DIRTY` and cannot prove a clean tree. |
| SBOM and production release | `P1 GAP` | No SBOM or production binary/signature was observed; green CI and a hosted evidence attestation cannot upgrade those states. |
| Live provider and rendered behavior | `UNVERIFIED` | Provider-token quality, permissions, undo/full recovery, live-provider behavior, and unsupported rendered platforms remain outside this audit. |
| Hostile runtime and filesystem boundaries | `UNVERIFIED` | Broader process-death, arbitrary same-UID TOCTOU, git hook/textconv, MCP prompt-injection, and child-environment leakage claims remain outside the observed evidence. |

The unresolved findings are intentionally retained as gaps. None changes task
completion, goal persistence, or user-facing workflow behavior.

## Reproduction commands

The evidence can be rechecked with GitHub authentication using the following
bounded sequence:

```sh
gh run download 33581839408 --repo saiaathish/picogent --dir <audit-dir>
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-3900ce2*/release-gates.json \
  --expected-sha 3900ce229440bd1ece64c9d2cf15960aa471bdcd \
  --event push --required test,security
gh attestation verify <audit-dir>/verification-manifest-3900ce2*/release-gates.json \
  --repo saiaathish/picogent \
  --predicate-type https://github.com/saiaathishkarthik/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --format=json
gh attestation verify <audit-dir>/verification-manifest-3900ce2*/verification-manifest.json \
  --repo saiaathish/picogent \
  --predicate-type https://github.com/saiaathishkarthik/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --format=json
```

The release-gate validator and hosted verifier are evidence checks only. A
successful reproduction does not authorize a release.

## Audit conclusion

The M lane is correctly merged and its hosted attestation path is directly
observable and independently rechecked. The audit remains `INCONCLUSIVE` for
release readiness because the verification manifest is `UNVERIFIED`, the
workflow produces no SBOM or production binary, and the runtime, platform, and
workflow supply-chain gaps above remain open. Parent #316 stays open until
those boundaries are either narrowed with new evidence or explicitly accepted
as outside the release claim.
