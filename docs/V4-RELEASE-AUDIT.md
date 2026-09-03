# V4 independent release-evidence audit

Status: `INCONCLUSIVE` for release authorization. This is an independently
rechecked evidence report, not a release approval or a supply-chain
certification.

Latest audit snapshot: 2026-09-02 UTC
Latest issue: [#356](https://github.com/saiaathish/picogent/issues/356)
Parent: [#316](https://github.com/saiaathish/picogent/issues/316)
Broader parent: [#246](https://github.com/saiaathish/picogent/issues/246)

Historical snapshot: [#321](https://github.com/saiaathish/picogent/issues/321)

“Independent” here means a fresh repository-side recheck of downloaded
artifacts and the exact Git history. It does not mean a third-party audit.

## Latest follow-up audit — current main after bounded GUI teardown

This follow-up rechecks the exact `main` commit after [PR #355](https://github.com/saiaathish/picogent/pull/355)
merged the bounded GUI fresh-process teardown evidence. It preserves the
earlier lifecycle and attestation snapshots below as historical evidence; old
artifact observations are not rewritten retroactively.

### Candidate and hosted run

- PR #355 source head: `8a3a347ea111818c164b2eac4782d9c861a84c15`.
- Merge commit and current `main`: `92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2`.
- Post-merge CI run: [33697379530](https://github.com/saiaathish/picogent/actions/runs/33697379530).
- The run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100469082287` | `PASS` |
| `test (ubuntu-latest)` | `100469082423` | `PASS` |
| `test (windows-latest)` | `100469082441` | `PASS` |
| `test (macos-latest)` | `100469082510` | `PASS` |
| `release-evidence` | `100469841050` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2` | `9872375889` | 970 bytes | present, unexpired |
| `release-attestation-92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2` | `9872376304` | 16,448 bytes | present, unexpired |

### Follow-up verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success`. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates with exactly the required `test` and `security` records at the candidate SHA. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | `head.sha` equals `expected_sha` and `head.tree` is `CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | Both verifier outputs record `saiaathish/picogent`, the candidate, the `push` event, run `33697379530`, and the main workflow signer. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects re-verify with the explicit repository, predicate type, and signer workflow; each records a Rekor timestamp. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | The broader `go test ./...` check passed 42 tests, but the targeted check was skipped and coverage was not collected. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: UNVERIFIED`. Its targeted check is
`SKIPPED` because no safe targeted command was detected. The broader
`go test ./...` check is `PASS`, reports `passed: 42`, and has coverage
`UNVERIFIED` because coverage was not collected. The hosted matrix's
independent test jobs still reported `PASS`; this difference is retained
rather than collapsed into one claim.

The downloaded release-gates ledger was independently validated:

```text
release gates PASS: 2 required job(s) for push
```

The recomputed subject digests and the predicate values are:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `94b58b2fc11caa440e1837510506a35a8518df137521752af06dae3ed9e1f5bb` |
| `verification-manifest.json` | `693986b34eddce6886b54c15ba1a08cfade2637532caa70b2ddbe9c5e277f39e` |

The predicate and both independently downloaded verifier outputs record:

```text
predicate:    https://github.com/saiaathish/picogent/attestation/release-evidence/v1
repository:   saiaathish/picogent
candidate:    92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2
event:        push
run_id:       33697379530
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
issued_at:    2026-09-03T00:01:44.690037Z
expires_at:   2026-09-10T00:01:44.690037Z
```

The local rechecks used the exact repository, predicate type, and signer
workflow. `go run ./cmd/release-gates` returned success, and both
`gh attestation verify` commands returned success. Their certificate evidence
bound the workflow and source repository to `92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2`
and recorded a Rekor timestamp at `2026-09-02T19:01:45-05:00`. This confirms
the bounded hosted attestation path; it does not authorize a release or
upgrade the manifest's `UNVERIFIED` state.

### Remaining follow-up boundaries

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact candidate SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `UNVERIFIED` | The broader check passed, but targeted work was skipped and coverage was not collected. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `INCONCLUSIVE` | `actions/attest` is full-hash pinned; checkout, setup, Node, and upload actions still use moving major tags. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

The current run refreshes provenance after PR #355 and confirms the bounded
hosted subject binding. It does not close the remaining evidence gaps; parent
#316 remains open.

## Previous follow-up audit — current main after lifecycle evidence

This follow-up rechecks the exact `main` commit after [PR #343](https://github.com/saiaathish/picogent/pull/343)
merged the cross-surface crash-recovery evidence. It preserves the prior
namespace-correction audit below as historical evidence; the earlier run's
artifact state and observations are not rewritten retroactively.

### Candidate and hosted run

- Merge commit and current `main`: `a3c4ccf1a20add17724243521f088e1c4ec11ab2`.
- Lifecycle evidence source checkpoint: `9d53982` from PR #343.
- Post-merge CI run: [33683301465](https://github.com/saiaathish/picogent/actions/runs/33683301465).
- The run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100424700280` | `PASS` |
| `test (ubuntu-latest)` | `100424700380` | `PASS` |
| `test (windows-latest)` | `100424700508` | `PASS` |
| `test (macos-latest)` | `100424700576` | `PASS` |
| `release-evidence` | `100425773856` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-a3c4ccf1a20add17724243521f088e1c4ec11ab2` | `9867281970` | 989 bytes | present, unexpired |
| `release-attestation-a3c4ccf1a20add17724243521f088e1c4ec11ab2` | `9867282469` | 16,511 bytes | present, unexpired |

### Follow-up verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success`. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates with exactly the required `test` and `security` records at the candidate SHA. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | `head.sha` equals `expected_sha` and `head.tree` is `CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | Both verifier outputs record the candidate, `saiaathish/picogent`, the `push` event, run `33683301465`, and the main workflow signer. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects re-verify with the explicit repository, predicate type, and signer workflow; each records a Rekor timestamp. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | Its broader `go test ./...` check was killed after the 90-second bound; targeted coverage was skipped and coverage was not collected. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `a3c4ccf1a20add17724243521f088e1c4ec11ab2`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: INCONCLUSIVE` with reason `signal:
killed`. The targeted check is `SKIPPED` because no safe targeted command was
detected. The broader `go test ./...` check is `INCONCLUSIVE`, reports
`passed: 7`, ran for `90.013127485s`, and has coverage `UNVERIFIED` because
coverage was not collected. The hosted matrix's independent `test` jobs still
reported `PASS`; this difference is retained rather than collapsed into one
claim.

The downloaded release-gates ledger was independently validated:

```text
release gates PASS: 2 required job(s) for push
```

The recomputed subject digests and the predicate values are:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `831960356a424bc29ee2630b249577f11d440598a4fb3d2ec5af393b059b630a` |
| `verification-manifest.json` | `d11779e0d51f5a8bdf9fba767fa34b23792023e4ba4d5363ba64f4b1746e76db` |

The predicate and both independently downloaded verifier outputs record:

```text
predicate:    https://github.com/saiaathish/picogent/attestation/release-evidence/v1
repository:   saiaathish/picogent
candidate:    a3c4ccf1a20add17724243521f088e1c4ec11ab2
event:        push
run_id:       33683301465
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
issued_at:    2026-09-02T21:12:24.191068Z
expires_at:   2026-09-09T21:12:24.191068Z
```

Local rechecks used the exact repository, predicate type, and signer workflow:

```sh
gh run download 33683301465 --repo saiaathish/picogent --dir <audit-dir>
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-a3c4ccf1*/release-gates.json \
  --expected-sha a3c4ccf1a20add17724243521f088e1c4ec11ab2 \
  --event push --required test,security
gh attestation verify <audit-dir>/verification-manifest-a3c4ccf1*/release-gates.json \
  --repo saiaathish/picogent \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --format=json
gh attestation verify <audit-dir>/verification-manifest-a3c4ccf1*/verification-manifest.json \
  --repo saiaathish/picogent \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --format=json
```

The ledger validator and both hosted verifier commands returned success. Both
verifier outputs recorded the signer certificate for the main workflow, source
and workflow SHA `a3c4ccf1a20add17724243521f088e1c4ec11ab2`, and a Rekor
timestamp at `2026-09-02T21:12:24Z`. This confirms the bounded hosted
attestation path; it does not authorize a release or upgrade the manifest's
`INCONCLUSIVE` state.

### Remaining follow-up boundaries

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact candidate SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `INCONCLUSIVE` | The bounded broader check ended with `signal: killed`; targeted work was skipped. |
| Coverage | `UNVERIFIED` | No coverage was collected by the manifest command. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `INCONCLUSIVE` | `actions/attest` is full-hash pinned; checkout, setup, Node, and upload actions still use moving major tags. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

That historical run refreshed provenance after PR #343 and confirms the canonical
attestation subject binding. It does not close the remaining evidence gaps;
parent #316 remains open.

## Previous follow-up audit — canonical predicate namespace

The previous follow-up rechecked the exact `main` commit after [PR #335](https://github.com/saiaathish/picogent/pull/335)
corrected the hosted release-attestation predicate namespace. It preserves the
earlier audit below as historical evidence; the earlier run's emitted URI and
other observations are not rewritten retroactively.

### Candidate and hosted run

- Merge commit and current `main`: `8c020fe7c20d75bda143b9b93d77f1b0dd74400a`.
- Namespace correction source checkpoint: `9c4b9aeb27daf3cf52ac7a5ec4f7407c7743650d`.
- Post-merge CI run: [33661263971](https://github.com/saiaathish/picogent/actions/runs/33661263971).
- The run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100352086277` | `PASS` |
| `test (ubuntu-latest)` | `100352086358` | `PASS` |
| `test (windows-latest)` | `100352085926` | `PASS` |
| `test (macos-latest)` | `100352086113` | `PASS` |
| `release-evidence` | `100353264283` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-8c020fe7c20d75bda143b9b93d77f1b0dd74400a` | `9858936821` | 974 bytes | present, unexpired |
| `release-attestation-8c020fe7c20d75bda143b9b93d77f1b0dd74400a` | `9858937674` | 16,518 bytes | present, unexpired |

### Follow-up verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| The corrected workflow emits and verifies the canonical predicate namespace | `CONFIRMED` | Both hosted attestation subjects use `https://github.com/saiaathish/picogent/attestation/release-evidence/v1`. |
| The two subjects bind to the exact post-merge candidate and repository | `CONFIRMED` | Both predicates record candidate `8c020fe7c20d75bda143b9b93d77f1b0dd74400a` and repository `saiaathish/picogent`. |
| The signer and transparency-log evidence are directly inspectable | `CONFIRMED` | Both verifier outputs record the main workflow signer and a Rekor timestamp at `2026-09-02T17:32:40Z`. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates with exactly the required `test` and `security` records at the candidate SHA. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | `head.sha` equals `expected_sha` and `head.tree` is `CLEAN`. |
| The manifest proves complete release readiness | `UNVERIFIED` | Broader `go test ./...` passed, but targeted coverage was skipped and coverage was not collected. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `8c020fe7c20d75bda143b9b93d77f1b0dd74400a`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: UNVERIFIED` because coverage was not
collected. Its broader `go test ./...` check passed 42 tests; the targeted
check was `SKIPPED` because no safe targeted command was detected.

The downloaded release-gates ledger was independently validated:

```text
release gates PASS: 2 required job(s) for push
```

The recomputed subject digests and the predicate values are:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `cf9389e12796ef0dfcd33643bd888a32d51b4060a2959a315e0ce2bff0c5d972` |
| `verification-manifest.json` | `495a9fc27c427de34849c11858580d422f1a929b6da82ed62c61c6edb3193266` |

The predicate and both independently downloaded verifier outputs record:

```text
predicate:    https://github.com/saiaathish/picogent/attestation/release-evidence/v1
repository:   saiaathish/picogent
candidate:    8c020fe7c20d75bda143b9b93d77f1b0dd74400a
event:        push
run_id:       33661263971
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
```

Local rechecks used the exact repository, predicate type, and signer workflow:

```sh
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-8c020fe7*/release-gates.json \
  --expected-sha 8c020fe7c20d75bda143b9b93d77f1b0dd74400a \
  --event push --required test,security
gh attestation verify <audit-dir>/verification-manifest-8c020fe7*/release-gates.json \
  --repo saiaathish/picogent \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --format=json
gh attestation verify <audit-dir>/verification-manifest-8c020fe7*/verification-manifest.json \
  --repo saiaathish/picogent \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --format=json
```

All three commands returned success. Both hosted verifier outputs recorded a
Rekor timestamp at `2026-09-02T17:32:40Z`. This confirms the corrected,
bounded hosted-attestation path; it does not authorize a release or upgrade
the manifest's `UNVERIFIED` state.

### Remaining follow-up boundaries

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace | `PASS` | The live workflow, current contract doc, predicate, and verifier outputs agree on `saiaathish/picogent`. |
| Clean source provenance | `PASS` | The external evidence directory leaves the checked-out source tree `CLEAN` at manifest capture. |
| Targeted coverage | `UNVERIFIED` | The manifest records no safe targeted command and no collected coverage. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `INCONCLUSIVE` | `actions/attest` is full-hash pinned; other workflow actions still use moving major tags. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

The corrected namespace closes the prior P2 mismatch only. Parent #316 remains
open for the remaining conditional audit and release-evidence boundaries.

## Historical snapshot — original M-lane audit

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
