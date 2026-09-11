# Windows hostile parent-replacement evidence

This lane records a bounded same-UID Windows observation for the existing
`securefile` and `workspace` parent-replacement boundaries. It uses a
task-owned disposable directory, a separate helper process, and a Windows
junction created with `mklink /J`. The helper renames the trusted parent aside,
installs the junction to the outside fixture, waits for the victim operations,
then restores the trusted directory.

The fixture exercises bounded reads, atomic writes, exclusive writes, and
removes. It retains only JSON containing the candidate SHA, platform identity,
operation counts, and SHA-256 digests of the outside fixture. It never retains
provider output, credentials, or arbitrary repository data.

Run the focused lanes on a Windows checkout with a clean exact candidate:

```text
PICOGENT_WINDOWS_HOSTILE_SOURCE_SHA=$(git rev-parse HEAD) \\
PICOGENT_WINDOWS_HOSTILE_EVIDENCE_OUT=/absolute/path/outside/securefile.json \\
  go test -v ./internal/securefile -run '^TestWindowsSameUIDParentSwapConfinement$' -count=1

PICOGENT_WINDOWS_WORKSPACE_HOSTILE_SOURCE_SHA=$(git rev-parse HEAD) \\
PICOGENT_WINDOWS_WORKSPACE_HOSTILE_EVIDENCE_OUT=/absolute/path/outside/workspace.json \\
  go test -v ./internal/workspace -run '^TestWindowsSameUIDWorkspaceParentSwapConfinement$' -count=1
```

Hosted CI runs both lanes on `windows-latest` and uploads the digest-only
records as `hostile-parent-replacement-windows-<candidate-sha>`. The retained
records must report `broad_toctou_claim=UNVERIFIED`; a passing bounded result
does not establish universal same-UID race resistance, release authorization,
or v4 completion.
