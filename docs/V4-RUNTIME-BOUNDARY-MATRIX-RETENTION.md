# V4 runtime-boundary matrix retention

Status: retention helper for the canonical
`picogent.v4.runtime-boundary-matrix.v1` artifact. It does not authorize a
release and does not upgrade live-provider, rendered-platform, or hostile
TOCTOU gaps to `PASS`.

Parent: [#461](https://github.com/saiaathish/picogent/issues/461)

## Command

```sh
ARTIFACT_DIR="$(mktemp -d)"
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)" \
  --out "$ARTIFACT_DIR/runtime-boundary-matrix.json"
```

`--out` must be an absolute path outside the clean workspace. Retention uses
`O_EXCL` so an existing artifact fails closed. After write, the CLI reloads
and validates schema, candidate SHA, clean-head provenance, and claim shape.

## Fail-closed cases

- artifact path inside the checkout
- missing artifact on load
- overwrite of an existing artifact
- malformed / trailing / oversized JSON
- candidate SHA mismatch
- dirty tree or HEAD mismatch during collection

## Hosted retention

The `release-evidence` job writes the matrix into
`${{ runner.temp }}/picogent-release-evidence/` and uploads it with the
verification-manifest artifact set. That keeps provenance outside the
checkout without treating the matrix as a release-authorization predicate.
