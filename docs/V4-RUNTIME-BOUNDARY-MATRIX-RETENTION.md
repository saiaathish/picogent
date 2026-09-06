# V4 runtime-boundary matrix retention

Status: retention helper for the canonical
`picogent.v4.runtime-boundary-matrix.v1` artifact. It does not authorize a
release and does not upgrade live-provider, rendered-platform, or hostile
TOCTOU gaps to `PASS`.

Parent: [#461](https://github.com/saiaathish/picogent/issues/461). Read-side
follow-up: [#474](https://github.com/saiaathish/picogent/issues/474).

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

- artifact path inside the checkout (including after symlink resolution)
- symlinked parent directories or non-directory ancestors
- existing symlink at the artifact basename
- missing artifact on load
- overwrite of an existing artifact
- malformed / trailing / oversized JSON
- candidate SHA mismatch
- dirty tree or HEAD mismatch during collection

Parent creation uses `securefile.EnsureDir` and exclusive creation is
anchored with `os.OpenRoot`, so a later parent pathname swap cannot redirect
the already-opened directory handle. Loading now uses the existing
descriptor/handle-anchored `securefile.ReadFileLimited` primitive, so a later
parent rename cannot redirect the read. The path-based preflight before the
secure root opens, final basename replacement races, and arbitrary same-UID
TOCTOU coverage across every platform surface remain outside this guarantee.

## Hosted retention

The `release-evidence` job writes the matrix into
`${{ runner.temp }}/picogent-release-evidence/` and uploads it with the
verification-manifest artifact set. That keeps provenance outside the
checkout without treating the matrix as a release-authorization predicate.
