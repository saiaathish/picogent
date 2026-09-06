# V4 independent release-evidence audit

Status: `INCONCLUSIVE` for release authorization. This is an independently
rechecked evidence report, not a release approval or a supply-chain
certification.

Historical audit snapshot: 2026-09-06 UTC
The checkpoint named below is historical and is superseded by later `main`
merges; it is not a current release-readiness snapshot.
Latest checkpoint: post-[#459](https://github.com/saiaathish/picogent/pull/459)/[#463](https://github.com/saiaathish/picogent/pull/463)/[#465](https://github.com/saiaathish/picogent/pull/465)/[#466](https://github.com/saiaathish/picogent/pull/466) main `b00b88c15b310c05042ebb8debd876b221160389`
Parent: [#316](https://github.com/saiaathish/picogent/issues/316)
Broader parent: [#246](https://github.com/saiaathish/picogent/issues/246)

Historical snapshot: [#321](https://github.com/saiaathish/picogent/issues/321)

“Independent” here means a fresh repository-side recheck of downloaded
artifacts and the exact Git history. It does not mean a third-party audit.

Later runtime-boundary work is recorded in
[V4-RUNTIME-BOUNDARY-MATRIX.md](V4-RUNTIME-BOUNDARY-MATRIX.md) and its linked
evidence records. At the current checkpoint, #450 and #453 remain open.

## Historical follow-up audit — exact current main after PR #459/#463/#465/#466

Status: `INCONCLUSIVE` for release authorization. This refresh rechecks exact
`main` after the runtime-boundary matrix (#459), rendered recovery API-boundary
fixture (#463), evidence-contract consolidation (#465), and exact-SHA matrix
retention (#466). It preserves earlier audits below as historical evidence and
does not authorize a release. Parent [#450](https://github.com/saiaathish/picogent/issues/450)
is closed as completed; [#453](https://github.com/saiaathish/picogent/issues/453)
remains open for live-provider, cross-platform rendered, and hostile TOCTOU
observation.

Current ledger note: [#450](https://github.com/saiaathishkarthik/picogent/issues/450)
is open now; the closed state above belongs only to this historical audit.

### Candidate and hosted runs

- PR #459 merge: `563a865f74f866569de2403bd2869022f5c327bc`.
- PR #463 merge: `e6c5bb9486870a08642996f2d5e2deb62714764c`.
- PR #465 merge: `ccb2c505538c04934afbe581d947c2efbe09d438`.
- PR #466 merge / current `main`: `b00b88c15b310c05042ebb8debd876b221160389`.
- Post-merge CI: [34010511135](https://github.com/saiaathish/picogent/actions/runs/34010511135), `push`, conclusion `success`.
- Post-merge release-artifacts: [34010511126](https://github.com/saiaathish/picogent/actions/runs/34010511126), `push`, conclusion `success`.

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `101425394269` | `success` |
| `test (ubuntu-latest)` | `101425394200` | `success` |
| `test (windows-latest)` | `101425394338` | `success` |
| `test (macos-latest)` | `101425394271` | `success` |
| `release-evidence` | `101426239399` | `success` |
| `production-artifacts` | `101425394188` | `success` |

| Artifact | Artifact ID | Size | GitHub artifact digest |
| --- | ---: | ---: | --- |
| `verification-manifest-b00b88c15b310c05042ebb8debd876b221160389` | `9982412615` | 9,498 bytes | `sha256:2ced37b24c4832aafac7d4410a42d282b4033002b041b5831c9b24d44830adab` |
| `release-attestation-b00b88c15b310c05042ebb8debd876b221160389` | `9982412773` | 16,442 bytes | `sha256:052e26a07d447d19dae41ca9791c542816db4190a1e3787ea2d7fe0c2c9929e2` |
| `release-artifacts-b00b88c15b310c05042ebb8debd876b221160389` | `9982314801` | 57,890,810 bytes | `sha256:9966da5c471cb3aae12d766d05c68204d019844d4899a62a47372564f68b01ad` |

### Current-head verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed at the exact candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs completed with `success`. |
| The release-gates ledger is valid | `CONFIRMED` | Local `release-gates` validation returned `release gates PASS: 2 required job(s) for push` for the downloaded ledger. |
| Exact candidate provenance is clean | `CONFIRMED` | Manifest `head.match: PASS`, `head.tree: CLEAN`, SHA `b00b88c15b310c05042ebb8debd876b221160389`. |
| Targeted verification evidence is present | `CONFIRMED` | Targeted `internal/verify` check `PASS` with coverage `79.72027972027972%`. |
| Hosted attestations verify for the exact candidate | `CONFIRMED` | Both subjects re-verified under predicate `https://github.com/saiaathish/picogent/attestation/release-evidence/v1`, signer `…/ci.yml@refs/heads/main`, source ref `refs/heads/main`; Rekor timestamp `2026-09-06T04:12:16Z`. |
| Deterministic production binaries/SBOM lane completed | `CONFIRMED` | Production-artifacts job succeeded and uploaded the release-artifacts bundle for this SHA. Bound artifact evidence only. |
| Runtime-boundary matrix retained outside checkout | `CONFIRMED` | Hosted artifact includes `runtime-boundary-matrix.json` (`picogent.v4.runtime-boundary-matrix.v1`) bound to `b00b88c…` with summary `PASS:4`, `INCONCLUSIVE:1`, `UNVERIFIED:3`. |
| Rendered recovery allow→undo→reload API boundary | `CONFIRMED` | Matrix row `rendered-recovery-undo-reload` is `PASS` for the deterministic fixture; not browser-DOM or live-provider proof. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | Broader `go test ./...` is `INCONCLUSIVE` (`signal: killed` after ~90.02s, 9 passed); overall manifest `INCONCLUSIVE`. |
| Live-provider quality | `UNVERIFIED` | Matrix row `live-provider-quality` remains `UNVERIFIED`. |
| Cross-platform rendered behavior | `UNVERIFIED` | Matrix row `rendered-cross-platform` remains `UNVERIFIED`. |
| Hostile filesystem TOCTOU | `UNVERIFIED` | Matrix row `hostile-filesystem-toctou` remains `UNVERIFIED`. |
| Overall release authorization | `INCONCLUSIVE` | Green CI, attestations, SBOM, matrix retention, and API-boundary evidence do not close the live/hostile/authorization gaps. |

### Independent artifact recheck

Downloaded `verification-manifest.json` reports schema `picogent.verify.v1`,
matching candidate/expected SHA `b00b88c15b310c05042ebb8debd876b221160389`,
`head.match: PASS`, `head.tree: CLEAN`, targeted `PASS` with measured coverage
`79.72027972027972%`, broader `INCONCLUSIVE` with reason `signal: killed`, and
overall status `INCONCLUSIVE`.

Local SHA-256 recomputation matched the signed predicate:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `d43e78837dc4fe04bd2202f2bd88d33356e57011716095875b3e811f8a42fa37` |
| `verification-manifest.json` | `02681c9253b6e087fbf7ad3484acee68b36baa21c39c9a44358d0281706340df` |

Retained matrix unverified IDs: `hostile-filesystem-toctou`,
`live-provider-quality`, `rendered-cross-platform`. Release authorization row
is `INCONCLUSIVE`. This does **not** authorize a release.

## Latest follow-up audit — exact current main after PR #445 and #447

This refresh rechecks the exact current `main` after security child-env
sanitization (#445) and the post-#442 outcome-quality matrix refresh (#447).
It preserves the prior #440/#446, #420, and #419 observations below as
historical evidence; no earlier artifact observation is rewritten
retroactively. The result is an evidence report, not release authorization.

### Candidate and hosted run

- PR #447 source head: `c0543fcec92d8192bad22b76f59e690fbd29754c`.
- Merge commit and current `main`: `a4a4de276091e8110576adb6bbf1f526ec007f5d`.
- Immediate predecessors on `main`: #445 (`7e6ccea`), #446 (`dc628bd`),
  #444 (`821ae84`), #443 (`a493c2e`), #442 (`a6d3af3`), #441 (`923ce9a`).
- PR #447 validation run: [34002680270](https://github.com/saiaathish/picogent/actions/runs/34002680270), all five required gates passed.
- Post-merge CI run: [34003534333](https://github.com/saiaathish/picogent/actions/runs/34003534333).
- The post-merge run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `101406532734` | `PASS` |
| `test (ubuntu-latest)` | `101406532869` | `PASS` |
| `test (windows-latest)` | `101406532831` | `PASS` |
| `test (macos-latest)` | `101406532828` | `PASS` |
| `release-evidence` | `101407369018` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-a4a4de276091e8110576adb6bbf1f526ec007f5d` | `9980326421` | 991 bytes | present, unexpired |
| `release-attestation-a4a4de276091e8110576adb6bbf1f526ec007f5d` | `9980326595` | 16,532 bytes | present, unexpired |

### Current-head verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success` at the merge SHA. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates locally with exactly the required `test` and `security` records at the candidate SHA and `push` event. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | The manifest records matching candidate and expected SHA values, `head.match: PASS`, and `head.tree: CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | The predicate and downloaded Sigstore bundles record `saiaathish/picogent`, candidate `a4a4de27`, event `push`, run `34003534333`, and the main workflow identity. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects independently verify with the explicit repository, predicate type, signer workflow, and `refs/heads/main`; each records a Rekor timestamp. |
| Hosted workflow actions are pinned immutably | `CONFIRMED` | All inspected `uses:` references in `.github/workflows/ci.yml` resolve to full commit hashes. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | The manifest's targeted check is `SKIPPED`; the bounded broader `go test ./...` check is `INCONCLUSIVE` with reason `signal: killed` after about 90.00 seconds and 7 passed tests. Coverage is `UNVERIFIED`. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `a4a4de276091e8110576adb6bbf1f526ec007f5d`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: INCONCLUSIVE` with reason `signal:
killed`. Its targeted check is `SKIPPED` because no safe targeted command was
detected. The broader check records `passed: 7`,
`duration_ns: 90002359382`, and coverage `UNVERIFIED` because coverage was not
collected. The release-gate validator returned:

```text
release gates PASS: 2 required job(s) for push
```

The locally recomputed subject digests match the signed predicate:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `0351b8275c95aa974fb9d1df9552e91c368174ca26b6905ca0603ebe4d752cd4` |
| `verification-manifest.json` | `5e9ce9f5c26033008ef8b9146448b1c3ae33927757c430e8d5af12f9431e61d7` |

Both `gh attestation verify` commands returned success against the downloaded
Sigstore bundles with the explicit repository, canonical predicate type, main
signer workflow, and `--source-ref refs/heads/main`. The verified certificate
evidence records signer
`https://github.com/saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main`
and a Rekor timestamp of `2026-09-05T21:26:57-04:00` for each subject. The
signed predicate records `issued_at: 2026-09-06T01:26:57.062313Z` and
`expires_at: 2026-09-13T01:26:57.062313Z`. Neither verification result
authorizes a release or upgrades the manifest's `INCONCLUSIVE` state.

### Hostile review (bounded)

This is a repository-side hostile pass over the exact-head evidence, not a
claim that every hostile runtime path is closed.

| Pressure | Result | Finding |
| --- | --- | --- |
| Dirty or mismatched candidate SHA | `PASS` | Manifest `head.match`/`head.tree` and gate `candidate_sha` agree with the pushed merge SHA; local gate validation fails closed when the expected SHA is wrong. |
| Attestation subject digests | `PASS` | Local SHA-256 of both subjects equals the signed predicate fields and the verified statement digests. |
| Predicate / signer / ref mismatch | `PASS` | Offline bundle verify requires the canonical predicate type, main signer workflow, and `refs/heads/main`. |
| Moving action tags | `PASS` | Every `uses:` entry in `.github/workflows/ci.yml` is full-hash pinned at this head. |
| Child-environment leakage (non-interactive) | `BOUNDED` | #445 lands `procenv.Sanitized()` for rg/bash/OpenCode/GUI helper children with hostile coverage from #444; arbitrary same-UID TOCTOU and untrusted MCP prompt injection remain outside this boundary. |
| Live-provider / rendered / SBOM claims | `UNVERIFIED` | Not exercised by this audit run. |

### Current-head boundary

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact current main SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `INCONCLUSIVE` | The bounded broader check was killed; targeted work was skipped and coverage was not collected. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `PASS` | All workflow action references inspected in `.github/workflows/ci.yml` resolve to full commit hashes. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

This is the latest exact-head release-evidence checkpoint, not a release
approval. Signed supply-chain scope is bounded to the observed hosted
subjects; production packaging, SBOM, live-provider quality, rendered
behavior, broader hostile runtime, targeted coverage, outcome-quality win
claims, and overall v4 readiness remain unverified or inconclusive. Parent
#316 and broader parent #246 remain open until their scoped closeout records
this exact-head terminus.

### Current-head reproduction commands

```sh
gh run download 34003534333 --repo saiaathish/picogent --dir <audit-dir>
jq -c '.[].attestation.bundle' \
  <audit-dir>/release-attestation-a4a4de276091e8110576adb6bbf1f526ec007f5d/*attestation.json \
  > <audit-dir>/release-evidence-bundle.jsonl
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-a4a4de276091e8110576adb6bbf1f526ec007f5d/release-gates.json \
  --expected-sha a4a4de276091e8110576adb6bbf1f526ec007f5d \
  --event push --required test,security
gh attestation verify <audit-dir>/verification-manifest-a4a4de276091e8110576adb6bbf1f526ec007f5d/release-gates.json \
  --repo saiaathish/picogent \
  --bundle <audit-dir>/release-evidence-bundle.jsonl \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --source-ref refs/heads/main --format=json
gh attestation verify <audit-dir>/verification-manifest-a4a4de276091e8110576adb6bbf1f526ec007f5d/verification-manifest.json \
  --repo saiaathish/picogent \
  --bundle <audit-dir>/release-evidence-bundle.jsonl \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --source-ref refs/heads/main --format=json
```

## Historical follow-up audit — exact main after PR #440

This historical refresh rechecked the exact `main` after documentation-only PR
#440. It preserves the prior #420 and #419 observations below as historical
evidence; no earlier artifact observation is rewritten retroactively. The
result is an evidence report, not release authorization. Superseded as the
latest checkpoint by the post-#445/#447 refresh above.

### Candidate and hosted run

- PR #440 source head: `2c38d58b1bf7fdb78249f5784e54b8330e213c21`.
- Merge commit and current `main`: `1e1ef174d47a84d32d5a33cd6cb0f062cb9c3758`.
- PR validation run: [33998091299](https://github.com/saiaathish/picogent/actions/runs/33998091299), all five required gates passed.
- Post-merge CI run: [33998473168](https://github.com/saiaathish/picogent/actions/runs/33998473168).
- The post-merge run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `101392018360` | `PASS` |
| `test (ubuntu-latest)` | `101392018431` | `PASS` |
| `test (windows-latest)` | `101392018433` | `PASS` |
| `test (macos-latest)` | `101392018434` | `PASS` |
| `release-evidence` | `101392731138` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-1e1ef174d47a84d32d5a33cd6cb0f062cb9c3758` | `9978868236` | 990 bytes | present, unexpired |
| `release-attestation-1e1ef174d47a84d32d5a33cd6cb0f062cb9c3758` | `9978868427` | 16,699 bytes | present, unexpired |

### Current-head verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success` at the merge SHA. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates locally with exactly the required `test` and `security` records at the candidate SHA and `push` event. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | The manifest records matching candidate and expected SHA values, `head.match: PASS`, and `head.tree: CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | The predicate and downloaded Sigstore bundles record `saiaathish/picogent`, candidate `1e1ef174`, event `push`, run `33998473168`, and the main workflow identity. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects independently verify with the explicit repository, predicate type, signer workflow, and `refs/heads/main`; each records a Rekor timestamp. |
| Hosted workflow actions are pinned immutably | `CONFIRMED` | All inspected `uses:` references in `.github/workflows/ci.yml` resolve to full commit hashes. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | The manifest's targeted check is `SKIPPED`; the bounded broader `go test ./...` check is `INCONCLUSIVE` with reason `signal: killed` after about 90.05 seconds and 7 passed tests. Coverage is `UNVERIFIED`. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `1e1ef174d47a84d32d5a33cd6cb0f062cb9c3758`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: INCONCLUSIVE` with reason `signal:
killed`. Its targeted check is `SKIPPED` because no safe targeted command was
detected. The broader check records `passed: 7`,
`duration_ns: 90050203630`, and coverage `UNVERIFIED` because coverage was not
collected. The release-gate validator returned:

```text
release gates PASS: 2 required job(s) for push
```

The locally recomputed subject digests match the signed predicate:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `b5557fe2055cb0b240893a54ed62e6e9462fca9890c107f083c38ac1a15eb3c6` |
| `verification-manifest.json` | `0a779cbe44666b993faf70888792bd26f5936e88e39f3bb07ff9b9819cc63aff` |

Both `gh attestation verify` commands returned success against the downloaded
Sigstore bundles with the explicit repository, canonical predicate type, main
signer workflow, and `--source-ref refs/heads/main`. The verified certificate
evidence records signer
`https://github.com/saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main`
and a Rekor timestamp of `2026-09-05T19:30:35-04:00` for each subject. The
signed predicate records `issued_at: 2026-09-05T23:30:34.886599Z` and
`expires_at: 2026-09-12T23:30:34.886599Z`. Neither verification result
authorizes a release or upgrades the manifest's `INCONCLUSIVE` state.

### Current-head boundary

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact current main SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `INCONCLUSIVE` | The bounded broader check was killed; targeted work was skipped and coverage was not collected. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `PASS` | All workflow action references inspected in `.github/workflows/ci.yml` resolve to full commit hashes. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

This historical checkpoint is superseded by the post-#445/#447 refresh above.
Signed supply-chain scope remained bounded to the observed hosted subjects;
production packaging, SBOM, live-provider quality, rendered behavior, hostile
runtime, targeted coverage, and overall v4 readiness remained unverified or
inconclusive.

### Historical reproduction commands (#440 head)

```sh
gh run download 33998473168 --repo saiaathish/picogent --dir <audit-dir>
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-1e1ef174d47a84d32d5a33cd6cb0f062cb9c3758/release-gates.json \
  --expected-sha 1e1ef174d47a84d32d5a33cd6cb0f062cb9c3758 \
  --event push --required test,security
```

The attestation reproduction uses the downloaded wrapper's inner Sigstore
bundles and the same explicit repository, canonical predicate, signer
workflow, and `refs/heads/main` constraints shown in the prior section.

## Historical follow-up audit — exact main after PR #419

This refresh rechecks the exact current `main` after the documentation
checkpoint in [PR #419](https://github.com/saiaathish/picogent/pull/419). It
preserves the prior #395 audit below as historical evidence; no earlier
artifact observation is rewritten retroactively. The result is an evidence
report, not release authorization.

### Candidate and hosted run

- PR #419 source head: `cb370495eae144302ade542d7bace4404705fb70`.
- Merge commit and current `main`: `b73db3f297edd7759de4030145574b26dc6eefdc`.
- PR validation run: [33824175340](https://github.com/saiaathish/picogent/actions/runs/33824175340), all five required gates passed.
- Post-merge CI run: [33824519857](https://github.com/saiaathish/picogent/actions/runs/33824519857).
- The post-merge run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100874217319` | `PASS` |
| `test (ubuntu-latest)` | `100874217343` | `PASS` |
| `test (windows-latest)` | `100874217118` | `PASS` |
| `test (macos-latest)` | `100874217270` | `PASS` |
| `release-evidence` | `100874933192` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-b73db3f297edd7759de4030145574b26dc6eefdc` | `9919545307` | 974 bytes | present, unexpired |
| `release-attestation-b73db3f297edd7759de4030145574b26dc6eefdc` | `9919545558` | 16,217 bytes | present, unexpired |

### Current-head verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success` at the merge SHA. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates with exactly the required `test` and `security` records at the candidate SHA and `push` event. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | The manifest records matching candidate and expected SHA values, `head.match: PASS`, and `head.tree: CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | The predicate and downloaded Sigstore bundles record `saiaathish/picogent`, candidate `b73db3f`, event `push`, run `33824519857`, and the main workflow identity. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects re-verify from the downloaded bundle with the explicit repository, predicate type, signer workflow, and `refs/heads/main`; each records a Rekor timestamp. |
| Hosted workflow actions are pinned immutably | `CONFIRMED` | The current workflow references checkout, Go, Node, attest, and artifact-upload actions by full commit SHA. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | The broader `go test ./...` check passed 43 tests, but targeted work was skipped and coverage was not collected. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `b73db3f297edd7759de4030145574b26dc6eefdc`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: UNVERIFIED` because coverage was not
collected. Its targeted check is `SKIPPED` because no safe targeted command was
detected. The broader `go test ./...` check is `PASS`, reports `passed: 43`, and
ran for about 42.2 seconds. This distinction is retained rather than collapsed
into a release approval.

The downloaded release-gates ledger was independently validated:

```text
release gates PASS: 2 required job(s) for push
```

The locally recomputed subject digests match the signed predicate:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `a9b25060e44a51498bb4f5e55c430bc1c8102fd448974f3da23de03275bbc1d3` |
| `verification-manifest.json` | `b560a029b25c47c2124db68e637ccc4a5e0f95c72fb9a6db7ddc6119434ceb19` |

The signed predicate and both independently verified subject bundles record:

```text
predicate:    https://github.com/saiaathish/picogent/attestation/release-evidence/v1
repository:   saiaathish/picogent
candidate:    b73db3f297edd7759de4030145574b26dc6eefdc
event:        push
run_id:       33824519857
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
issued_at:    2026-09-04T01:11:50.895293Z
expires_at:   2026-09-11T01:11:50.895293Z
```

Both `gh attestation verify` commands returned success against the downloaded
Sigstore bundle with the explicit repository, canonical predicate type, main
signer workflow, and `--source-ref refs/heads/main`. The verified certificate
evidence records a GitHub-hosted runner, workflow trigger `push`, source
repository digest `b73db3f297edd7759de4030145574b26dc6eefdc`, and a Rekor
timestamp of `2026-09-03T20:11:51-05:00` for each subject. The downloaded
wrapper is `gh` JSON output rather than the direct bundle format, so extracting
the inner bundle is part of this offline reproduction. Neither verification
result authorizes a release or upgrades the manifest's `UNVERIFIED` state.

### Current-head boundary

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact current main SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `UNVERIFIED` | The broader check passed, but targeted work was skipped and coverage was not collected. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `PASS` | All workflow action references inspected in `.github/workflows/ci.yml` resolve to full commit hashes. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

This is the latest exact-head release-evidence checkpoint, not a release
approval. Signed supply-chain scope is bounded to the observed hosted subjects;
production packaging, SBOM, live-provider quality, rendered behavior, hostile
runtime, targeted coverage, and overall v4 readiness remain unverified or
inconclusive. Parent #316 and broader parent #246 remain open.

### Current-head reproduction commands

The current evidence can be rechecked with GitHub authentication using the
following bounded sequence:

```sh
gh run download 33824519857 --repo saiaathish/picogent --dir <audit-dir>
jq -c '.[].attestation.bundle' \
  <audit-dir>/release-attestation-b73db3f297edd7759de4030145574b26dc6eefdc/*attestation.json \
  > <audit-dir>/release-evidence-bundle.jsonl
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-b73db3f297edd7759de4030145574b26dc6eefdc/release-gates.json \
  --expected-sha b73db3f297edd7759de4030145574b26dc6eefdc \
  --event push --required test,security
gh attestation verify <audit-dir>/verification-manifest-b73db3f297edd7759de4030145574b26dc6eefdc/release-gates.json \
  --repo saiaathish/picogent \
  --bundle <audit-dir>/release-evidence-bundle.jsonl \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --source-ref refs/heads/main --format=json
gh attestation verify <audit-dir>/verification-manifest-b73db3f297edd7759de4030145574b26dc6eefdc/verification-manifest.json \
  --repo saiaathish/picogent \
  --bundle <audit-dir>/release-evidence-bundle.jsonl \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main \
  --source-ref refs/heads/main --format=json
```

The release-gate validator and hosted verifier are evidence checks only. A
successful reproduction does not authorize a release.

## Historical snapshot — exact current main after PR #394

This historical audit snapshot rechecked the exact `main` after the
cross-surface documentation checkpoint in [PR #392](https://github.com/saiaathish/picogent/pull/392)
and the Windows sustained-trace timing correction in
[PR #394](https://github.com/saiaathish/picogent/pull/394). It preserves every
earlier audit below as historical evidence; no earlier artifact observation is
rewritten retroactively. The result is an evidence report, not release
authorization.

### Candidate and hosted run

- PR #394 source head: `a65b791d4282b9657b6ff71270b173c469e4d8f0`.
- Merge commit and current `main`: `563f45d4480c115aec882baa4fa0aa75c736b0c7`.
- PR validation run: [33742019293](https://github.com/saiaathish/picogent/actions/runs/33742019293), all five required gates passed.
- Post-merge CI run: [33742545804](https://github.com/saiaathish/picogent/actions/runs/33742545804).
- The post-merge run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100607544686` | `PASS` |
| `test (ubuntu-latest)` | `100607544436` | `PASS` |
| `test (windows-latest)` | `100607544685` | `PASS` |
| `test (macos-latest)` | `100607544949` | `PASS` |
| `release-evidence` | `100608426716` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-563f45d4480c115aec882baa4fa0aa75c736b0c7` | `9888461530` | 973 bytes | present, unexpired |
| `release-attestation-563f45d4480c115aec882baa4fa0aa75c736b0c7` | `9888462292` | 16,402 bytes | present, unexpired |

### Final-audit verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success` at the merge SHA. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates with exactly the required `test` and `security` records for the candidate and `push` event. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | The manifest records matching candidate and expected SHA values, `head.match: PASS`, and `head.tree: CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | Both signed predicates record `saiaathish/picogent`, candidate `563f45d`, event `push`, run `33742545804`, and the main workflow signer. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects verify online and offline with the explicit repository, predicate type, signer workflow, and `refs/heads/main`; each records a Rekor timestamp. |
| Hosted workflow actions are pinned immutably | `CONFIRMED` | The current workflow uses full commit hashes for checkout, setup-go, setup-node, attest, and upload-artifact actions. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | The broader `go test ./...` check passed 42 tests, but targeted work was skipped and coverage was not collected. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `563f45d4480c115aec882baa4fa0aa75c736b0c7`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: UNVERIFIED` because coverage was not
collected. Its targeted check is `SKIPPED` because no safe targeted command was
detected. The broader `go test ./...` check is `PASS`, reports `passed: 42`, and
ran for about 46.6 seconds. This distinction is retained rather than collapsed
into a release approval.

The downloaded release-gates ledger was independently validated:

```text
release gates PASS: 2 required job(s) for push
```

The locally recomputed subject digests match both the signed predicate and the
workflow digest list:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `295b1abb86f4d89fec807c61dca7aeed32adcacb2e8b78c205f870aa0b52072e` |
| `verification-manifest.json` | `b631aca9313c566994ad98df520be93ce33ef0e81e66570e1f40d662b4beb6d3` |

The signed predicate and both independently verified subject bundles record:

```text
predicate:    https://github.com/saiaathish/picogent/attestation/release-evidence/v1
repository:   saiaathish/picogent
candidate:    563f45d4480c115aec882baa4fa0aa75c736b0c7
event:        push
run_id:       33742545804
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
issued_at:    2026-09-03T10:11:28.559721Z
expires_at:   2026-09-10T10:11:28.559721Z
```

Both `gh attestation verify` commands succeeded with the downloaded bundle,
and both direct online lookups also succeeded with the repository, predicate,
signer-workflow, and source-ref constraints. The verification output records
the Rekor timestamp `2026-09-03T05:11:29-05:00`. The downloaded wrapper remains
`gh` JSON output, so extracting the inner Sigstore bundle is part of the
offline reproduction. Neither verification result authorizes a release or
upgrades the manifest's `UNVERIFIED` state.

The local exact-head audit worktree also passed `go test ./... -count=1`,
`go vet ./...`, `go build ./...`, and `git diff --check`; the audit worktree was
clean at `563f45d4480c115aec882baa4fa0aa75c736b0c7`. These local checks support
the artifact recheck but do not replace the hosted provenance above.

### Final-audit boundary

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact candidate SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `UNVERIFIED` | The broader check passed, but targeted work was skipped and coverage was not collected. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `PASS` | All workflow action references inspected in `.github/workflows/ci.yml` resolve to full commit hashes. |
| GitHub online custom-predicate lookup | `PASS` | Both subjects verified directly with the explicit repository, predicate type, signer workflow, and main source ref. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

This is the final exact-head release-evidence checkpoint currently available,
not a release approval. Signed supply-chain scope is bounded to the observed
hosted subjects; production packaging, SBOM, live-provider quality, rendered
behavior, hostile runtime, targeted coverage, and overall v4 readiness remain
unverified or inconclusive.

## Historical snapshot — current main after GUI S/M/L evidence chain

This follow-up rechecks the exact `main` commit after the GUI signal S/M/L
chain landed through [PR #384](https://github.com/saiaathish/picogent/pull/384),
[PR #386](https://github.com/saiaathish/picogent/pull/386), and
[PR #388](https://github.com/saiaathish/picogent/pull/388). It records a fresh
post-merge evidence run and preserves every earlier audit below as historical
evidence; old artifact observations are not rewritten retroactively.

### Candidate and hosted run

- S source and merge: `aa4298dbf81f9da773ea674483aaf733bea4a61c` → `6a352899b10b2d1aa3300ba54800768af239b8f0` ([PR #384](https://github.com/saiaathish/picogent/pull/384)).
- M source and merge: `6fc4dadae40f52ee7193b2a7ce99ddad39a84614` → `c54501f03a44b420b61f04762461e19011c7b93e` ([PR #386](https://github.com/saiaathish/picogent/pull/386)).
- L source and merge/current `main`: `bcfb45eafe75b95771ccc18dbe849a28734ff426` → `8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c` ([PR #388](https://github.com/saiaathish/picogent/pull/388)).
- Post-merge CI run: [33731901077](https://github.com/saiaathish/picogent/actions/runs/33731901077).
- The run completed successfully for all five jobs:

| Job | Job ID | Result |
| --- | ---: | --- |
| `security` | `100573527605` | `PASS` |
| `test (ubuntu-latest)` | `100573527512` | `PASS` |
| `test (windows-latest)` | `100573527611` | `PASS` |
| `test (macos-latest)` | `100573527520` | `PASS` |
| `release-evidence` | `100575478086` | `PASS` |

| Artifact | Artifact ID | Size | Result |
| --- | ---: | ---: | --- |
| `verification-manifest-8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c` | `9884438519` | 973 bytes | present, unexpired |
| `release-attestation-8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c` | `9884439305` | 16,676 bytes | present, unexpired |

### Follow-up verdict

| Claim | Result | Boundary |
| --- | --- | --- |
| All required hosted CI jobs passed for the exact pushed candidate | `CONFIRMED` | Security, Ubuntu, Windows, macOS, and dependent release-evidence jobs all completed with `success`. |
| The release-evidence gates are valid | `CONFIRMED` | The downloaded ledger validates with exactly the required `test` and `security` records at the candidate SHA. |
| The candidate source tree was clean when the manifest was collected | `CONFIRMED` | `head.sha` equals `expected_sha` and `head.tree` is `CLEAN`. |
| The two signed subjects bind to the exact candidate and repository | `CONFIRMED` | Both verifier outputs record `saiaathish/picogent`, the candidate, the `push` event, run `33731901077`, and the main workflow signer. |
| The hosted attestations verify under the canonical predicate namespace | `CONFIRMED` | Both subjects re-verify offline with the explicit repository, custom predicate type, signer workflow, and `refs/heads/main`; each records a Rekor timestamp. |
| GitHub's online lookup verifies the custom predicate without a local bundle | `UNVERIFIED` | The current `gh` client could not initialize a verifier for this custom predicate when querying GitHub directly; the downloaded Sigstore bundles verify offline. |
| The verification manifest proves complete release readiness | `INCONCLUSIVE` | The broader `go test ./...` check passed 42 tests, but the targeted check was skipped and coverage was not collected. |
| Production release, SBOM, provider, rendered-platform, and hostile-runtime claims are proven | `UNVERIFIED` | Those evidence boundaries remain outside this run. |

### Independent artifact recheck

The downloaded manifest reports schema `picogent.verify.v1`, candidate and
expected SHA `8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c`, `head.match: PASS`,
`head.tree: CLEAN`, and overall `status: UNVERIFIED` because coverage was not
collected. Its targeted check is `SKIPPED` because no safe targeted command was
detected. The broader `go test ./...` check is `PASS`, reports `passed: 42`, and
ran for about 47.1 seconds. Its coverage remains `UNVERIFIED` because coverage
was not collected. This distinction is retained rather than collapsed into a
release approval.

The downloaded release-gates ledger was independently validated:

```text
release gates PASS: 2 required job(s) for push
```

The recomputed subject digests match the signed predicate:

| Subject | SHA-256 |
| --- | --- |
| `release-gates.json` | `cbe29042a5ac5fd8cf11c1d4111b8eee86118bceb96846999a1fd01f744fed9a` |
| `verification-manifest.json` | `024c21cce7b072b10aafb2c51023ebb71361dcd5ce7ec4b2240749d7ad3224be` |

The signed predicate and both independently verified subject bundles record:

```text
predicate:    https://github.com/saiaathish/picogent/attestation/release-evidence/v1
repository:   saiaathish/picogent
candidate:    8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c
event:        push
run_id:       33731901077
workflow:     saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
signer:       saiaathish/picogent/.github/workflows/ci.yml@refs/heads/main
issued_at:    2026-09-03T08:18:04.712050Z
expires_at:   2026-09-10T08:18:04.712050Z
```

The independent offline checks used the exact downloaded subject files and
the inner Sigstore bundle extracted from the downloaded attestation wrapper:

```sh
gh run download 33731901077 --repo saiaathish/picogent --dir <audit-dir>
jq -c '.[].attestation.bundle' \
  <audit-dir>/release-attestation-8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c/*attestation.json \
  > <audit-dir>/release-evidence-bundle.jsonl
go run ./cmd/release-gates \
  --ledger <audit-dir>/verification-manifest-8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c/release-gates.json \
  --expected-sha 8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c \
  --event push --required test,security
gh attestation verify <audit-dir>/verification-manifest-8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c/release-gates.json \
  --repo saiaathish/picogent \
  --bundle <audit-dir>/release-evidence-bundle.jsonl \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml \
  --source-ref refs/heads/main --format=json
gh attestation verify <audit-dir>/verification-manifest-8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c/verification-manifest.json \
  --repo saiaathish/picogent \
  --bundle <audit-dir>/release-evidence-bundle.jsonl \
  --predicate-type https://github.com/saiaathish/picogent/attestation/release-evidence/v1 \
  --signer-workflow saiaathish/picogent/.github/workflows/ci.yml \
  --source-ref refs/heads/main --format=json
```

Both `gh attestation verify` commands returned success. Their certificate
evidence bound the workflow and source repository to
`8c8883c15bad2c7edb538d44bc2b2ff2530d1f1c` and recorded a Rekor timestamp at
`2026-09-03T08:18:05Z`. The downloaded wrapper is `gh` JSON output rather than
the direct bundle format, so the `jq` extraction is part of this offline
reproduction. The direct online lookup was separately attempted and remains
`UNVERIFIED` for this custom predicate because the client could not initialize
a verifier. Neither result authorizes a release or upgrades the manifest's
`UNVERIFIED` state.

### Remaining follow-up boundaries

| Boundary | Result | Finding |
| --- | --- | --- |
| Predicate namespace and subject binding | `PASS` | The live workflow, predicate, repository, candidate, event, run, signer, and recomputed subject digests agree. |
| Clean source provenance | `PASS` | The manifest records `head.tree: CLEAN` at the exact candidate SHA. |
| Required hosted gate ledger | `PASS` | `test` and `security` are present exactly once with `PASS`, zero exit codes, and matching candidate/event. |
| Verification manifest | `UNVERIFIED` | The broader check passed, but targeted work was skipped and coverage was not collected. |
| SBOM and production release | `UNVERIFIED` | No SBOM, production binary, or release-package signature was present in the inspected run. |
| Hosted action immutability | `INCONCLUSIVE` | `actions/attest` is full-hash pinned; checkout, setup, Node, and upload actions still use moving major tags. |
| GitHub online custom-predicate lookup | `UNVERIFIED` | Direct `gh attestation verify` could not initialize a verifier; the downloaded bundle path is the independently verified path for this audit. |
| Live provider, rendered behavior, and hostile runtime | `UNVERIFIED` | These boundaries were not exercised by this evidence run. |

This refresh confirms the bounded hosted subject-binding path after the GUI
S/M/L chain. It does not close the remaining evidence gaps; parent #316 and
broader parent #246 remain open.

## Historical snapshot — current main after bounded GUI teardown

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
