# V4 SBOM and production artifact evidence

Status: this lane produces deterministic production binaries, packages, and
SPDX SBOMs bound to an exact source SHA. It does not authorize a release and
does not prove live-provider, rendered-platform, or hostile-runtime readiness.

## Command

```sh
SOURCE_DATE_EPOCH=$(git show -s --format=%ct <full-commit-id>)
go run ./cmd/release-artifact \
  --workspace . \
  --output-dir /tmp/picogent-release-artifacts \
  --candidate-sha <full-commit-id> \
  --source-date-epoch "$SOURCE_DATE_EPOCH" \
  --go-version go1.25.14 \
  --targets linux/amd64,darwin/arm64,windows/amd64
```

The output directory must be outside the checkout. The command fails closed on
dirty trees, SHA mismatch, missing LICENSE, or module-set disagreement between
the built binary (`go version -m`) and `go list -deps ./cmd/picogent`.

## Artifacts

For each target the lane retains:

- `picogent-<goos>-<goarch>[.exe]` production binary
- `picogent-<goos>-<goarch>.tar.gz` package containing the binary and `LICENSE`
- `picogent-<goos>-<goarch>.spdx.json` SPDX 2.3 SBOM derived from the binary's
  embedded module list
- `release-artifact-manifest.json` digest ledger

Hosted runs use `.github/workflows/release-artifacts.yml` with Go `1.25.14`,
exact source-SHA checkout, and an optional Sigstore attestation under predicate
`https://github.com/saiaathish/picogent/attestation/release-artifact/v1`.
Fork pull requests record attestation status `UNVERIFIED` instead of writing
attestations.

## Explicit boundaries

- Provider CLIs, credentials, OAuth, and runtime-installed tools remain
  `UNVERIFIED`.
- Cross-compiled binaries prove deterministic construction, not native runtime
  behavior on every host.
- Hosted Sigstore attestations are keyless evidence, not traditional detached
  package signatures.
- Overall release authorization remains separate from this artifact lane.
