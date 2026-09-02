# V4 release-evidence provenance boundary

Status: the S-lane contract and M-lane workflow implementation candidate for
issue #328, under parent issues #316 and #246. This document defines an
evidence boundary; it does not authorize a release.

## Problem

`verify-manifest` reports the state of the checked-out source tree. If the
workflow creates `release-gates.json`, `verification-manifest.json`, or any
attestation helper file inside that checkout before collecting the manifest,
those generated files appear as untracked changes. The resulting `DIRTY`
state is truthful, but it cannot prove that the candidate source tree itself
was clean.

## Contract

The release-evidence job must obey this order:

1. Check out the exact source candidate and record its expected full commit
   SHA.
2. Run the normal gates and validate their ledger.
3. Collect `verify-manifest` while the checkout has no generated evidence
   files.
4. Create predicates, checksums, verifier output, and upload inputs in one
   absolute runner-temporary directory outside the checkout.
5. Bind every subject and predicate digest to those exact files and the same
   candidate SHA.

`verify.ValidateReleaseEvidenceDirectory` is the bounded path guard for step
4. It rejects an empty or relative evidence path and any path equal to or
nested below the checked-out workspace. It accepts an external temporary
directory, including one whose name shares a prefix with the workspace.

The guard is a lexical layout contract. It does not establish protection
against an uncooperative same-UID writer, symlink/TOCTOU races, or a clean
production release. The workflow must still fail closed on directory creation,
write, digest, upload, or attestation errors.

## M-lane workflow implementation

`.github/workflows/ci.yml` sets one step-scoped `ARTIFACT_DIR` under
`${{ runner.temp }}`, validates it before creating the directory, and routes
the release ledger, verification manifest, predicate, checksum list,
attestation verifier output, and both uploads through that directory. The
source checkout remains unchanged when `verify-manifest` captures provenance.

`internal/verify/release_evidence_workflow_test.go` keeps the workflow wiring
bounded: it requires the validator to precede manifest generation, requires
the external artifact root and action inputs, and rejects checkout-relative
`artifacts/` paths. This is a regression guard for the repository workflow,
not hosted evidence.

## Evidence states

- `CLEAN` is valid only when `verify-manifest` observes an empty Git status
  for the exact expected commit.
- `DIRTY` remains truthful and must not be upgraded by moving or deleting
  evidence after the observation.
- Missing, malformed, or candidate-mismatched evidence remains
  `UNVERIFIED` or fails closed according to the existing manifest and
  attestation validators.
- Fork pull requests may run ordinary gates, but hosted write-capable
  attestation steps remain conditionally unavailable and therefore
  `UNVERIFIED`.

## Focused validation

```sh
go test ./internal/verify -run '^TestValidateReleaseEvidenceDirectory$' -count=1
```

The M lane must wire this contract into `.github/workflows/ci.yml` and move
all release-evidence outputs to the runner-temporary directory. The M-lane
candidate validation is:

```sh
go test ./cmd/release-evidence-layout ./internal/verify -count=1
go vet ./...
go build ./...
```

A passing local contract test is not hosted provenance evidence; the L lane
requires a fresh exact-main workflow run and independent artifact inspection.
