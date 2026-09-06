# v4 Linux same-UID parent-swap confinement evidence

Status: contract + Linux-only harness for descriptor/handle-anchored parent
replacement under a separate same-UID attacker process. This record belongs to
[#504](https://github.com/saiaathish/picogent/issues/504) under parent
[#453](https://github.com/saiaathish/picogent/issues/453). It complements the
Darwin harness from [#496](https://github.com/saiaathish/picogent/issues/496).

It does **not** upgrade `hostile-filesystem-toctou`.

## Commands

Hosted Ubuntu (or any Linux host):

```sh
PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT=/absolute/path/outside/checkout/hostile-parent-swap-linux.json \
  go test ./internal/securefile -run '^TestLinuxSameUIDParentSwapConfinement$' -count=1

go test ./internal/workspace -run '^TestLinuxSameUIDWorkspaceParentSwapConfinement$' -count=1
go test -race ./internal/securefile ./internal/workspace -run 'TestLinuxSameUID' -count=1
```

Non-Linux platforms skip these build-tagged tests.

## Explicit limits

- Linux-only; Windows remains outside this slice.
- Confirmed attacker activity with unchanged outside sentinel is required for
  `PASS`.
- Broad arbitrary same-UID TOCTOU across every surface remains `UNVERIFIED`.
- Not release authorization.
