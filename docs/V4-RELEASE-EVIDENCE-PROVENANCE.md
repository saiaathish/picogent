# V4 release-evidence provenance boundary

Status: the S-lane contract for issue #328, under parent issues #316 and
#246. This document defines an evidence boundary; it does not authorize a
release.

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
all release-evidence outputs to the runner-temporary directory. A passing
local contract test is not hosted provenance evidence; the L lane requires a
fresh exact-main workflow run and independent artifact inspection.
