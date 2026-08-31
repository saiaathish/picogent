# V4 quality gates

The CI workflow keeps the v4 release evidence bounded and reproducible. The
existing test matrix runs on Ubuntu, Windows, and macOS. Each matrix job runs
the full Go test suite and `go vet ./...`; the Ubuntu job additionally runs the
two security-boundary fuzz targets with a fixed two-second budget each.

The separate `security` job runs:

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 -show verbose ./...
```

The scanner version is pinned so a future database or tool release does not
silently change the gate. The scan is read-only and reports both reachable
vulnerabilities and module-only findings. A passing result is security
evidence for the scanned dependency graph, not a complete hostile audit or a
claim that every security property is proven.

The `release-evidence` job waits for the cross-platform test matrix and the
security job before producing the exact-head verification manifest. The
manifest remains a separate, bounded artifact and does not authorize a
release by itself.

## Local reproduction

```sh
go test ./...
go vet ./...
go test ./internal/perm -run '^$' -fuzz '^FuzzResolveWorkspacePathBoundary$' -fuzztime=2s
go test ./internal/tools -run '^$' -fuzz '^FuzzWebFetchIPBoundary$' -fuzztime=2s
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 -show verbose ./...
```

These commands provide bounded regression and dependency evidence. They do not
replace rendered GUI, live-provider, cross-process hostile-race, or full
release-signing evidence.
