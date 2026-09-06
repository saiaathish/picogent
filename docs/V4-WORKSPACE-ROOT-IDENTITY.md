# v4 workspace-root approval identity

Status: bounded fix for the approval-to-execution gap where an ordinary
same-UID directory replacement could redirect approved `read_file`,
`write_file`, or `edit_file` operations. This record belongs to
[#502](https://github.com/saiaathish/picogent/issues/502) under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

It does **not** upgrade the broad runtime-boundary row
`hostile-filesystem-toctou` and does not authorize a release.

## Behavior

- Path classification captures an opaque workspace-root identity using the
  opened directory's platform file identity (`os.SameFile`).
- The agent revalidates that identity after approval and again immediately
  before each pending file-tool execution.
- File-tool `Run` receives the approved identity on `tools.Context` and
  `mustWorkspace` fails closed on mismatch with `perm.ErrWorkspaceChanged`.
- Missing identity bindings for `read_file` / `write_file` / `edit_file` also
  fail closed.
- Safe/Fast permission UX and wire payloads remain path-only; the identity is
  unexported runtime state.

## Validation

```sh
go test ./internal/perm ./internal/tools ./internal/agent -count=1
go test -race ./internal/perm ./internal/tools -count=1
go vet ./internal/perm ./internal/tools ./internal/agent
```

## Explicit limits

- Not universal same-UID CAS for every pathname operation.
- Not Windows/Linux owned-browser rendered proof.
- Not release authorization.
