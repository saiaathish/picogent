# V4 secure-read identity checkpoint

Status: **bounded hardening**. This record documents the retained-file read
identity contract implemented for child issue #748 under
[#453](https://github.com/saiaathish/picogent/issues/453). It does not claim
universal same-UID filesystem TOCTOU resistance, release authorization, or v4
completion.

## Exact behavior checkpoint

The implementation checkpoint is:

```text
source: e4c58d65f6517684c893eb8149aba21eef225273
branch: codex/v4-retained-read-revalidate
```

For `ReadFile`, `ReadFileLimited`, and `ReadFilePrefix`, the secure-file
reader now:

1. classifies the final entry through the already-open parent descriptor or
   handle;
2. opens the entry through that same anchored parent and takes the existing
   shared read lock;
3. compares the opened descriptor/handle with the classified entry; and
4. compares the current final name with the opened descriptor/handle before
   consuming bytes.

Either identity mismatch returns `securefile.ErrReadChanged` and no payload.
Matching entries retain the existing bounded-size and prefix semantics.

## Local evidence

The exact source checkpoint passed:

```text
GOMAXPROCS=2 GOFLAGS=-p=1 go test ./internal/securefile
PASS

GOMAXPROCS=2 GOFLAGS=-p=1 go test -race ./internal/securefile ./internal/runtimeboundary
PASS

GOMAXPROCS=2 GOFLAGS=-p=1 GOOS=windows GOARCH=amd64 \
  go test -c -o /dev/null ./internal/securefile
PASS
```

The focused tests exercise both a pre-open classified-entry mismatch and a
post-open final-name mismatch through the existing `secureParent` seam. They
also retain a matching read regression. No timing-dependent race result is
represented as a universal security proof.

## Boundary that remains open

The identity checks narrow the pathname window around secure reads. They do
not detect every same-UID writer that mutates an already-open inode after the
last check, and they do not make multi-file reads or unrelated workspace,
checkpoint, release-artifact, or evolution-store transactions atomic. The
runtime matrix row `hostile-filesystem-toctou` therefore remains
`UNVERIFIED`; bounded parent-swap evidence remains a separate claim.
