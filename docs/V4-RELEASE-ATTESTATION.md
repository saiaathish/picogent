# V4 release attestation contract

`internal/verify.ReleaseAttestation` is a bounded verification primitive for
detached release-evidence signatures. It does not authorize a release, change
task completion, or create a user-facing workflow.

## Envelope

The JSON envelope uses schema `picogent.release-attestation.v1` and algorithm
`ed25519`:

- `payload.repository` identifies the repository;
- `payload.candidate_sha` identifies the exact tested commit;
- `payload.event`, `payload.workflow`, and `payload.run_id` identify the CI
  execution;
- `payload.release_gates_sha256` and
  `payload.verification_manifest_sha256` bind the two evidence artifacts;
- `payload.signer` identifies the claimed signer;
- `payload.issued_at` and `payload.expires_at` bound evidence freshness;
- `public_key` and `signature` carry canonical padded base64 values.

The signature covers a fixed protocol-domain prefix followed by deterministic
JSON for the payload. A consumer must provide the expected repository, commit,
event, workflow, run, artifact digests, signer, and trusted public key. A
self-signed envelope with an untrusted key is not accepted.

## Fail-closed states

`ValidateReleaseAttestationJSON` returns:

- `UNVERIFIED` with no error when the attestation is absent;
- `FAIL` with an error when present evidence is malformed, expired, tampered,
  oversized, or mismatched;
- `PASS` only when the complete envelope matches the expectation and its
  signature verifies.

The parser rejects unknown fields and trailing JSON. Payload text is bounded,
commit IDs must be full Git object IDs, artifact digests must be lowercase
SHA-256, run IDs must be positive, and validity windows cannot exceed seven
days. Future issuance is limited by the five-minute clock-skew allowance.

## Current evidence boundary

This S lane uses ephemeral test keys only. It does not publish an attestation,
store a private key, resolve GitHub or Sigstore trust, generate an SBOM, or
claim supply-chain or release readiness. Those are conditional follow-up
lanes under issue #316 and remain `UNVERIFIED` until directly observed.

## Local validation

```sh
go test ./internal/verify -count=1
go test -race ./internal/verify -count=3
```
