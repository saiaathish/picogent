# v4 macOS same-UID parent-swap confinement evidence

Status: bounded Darwin-only `PASS` for descriptor/handle-anchored parent
replacement confinement under a separate same-UID attacker process. This
record belongs to [#496](https://github.com/saiaathish/picogent/issues/496)
under parent [#453](https://github.com/saiaathish/picogent/issues/453).

It does **not** upgrade the broad runtime-boundary row
`hostile-filesystem-toctou`. That claim remains `UNVERIFIED`.

## Provenance

```text
repository: github.com/saiaathish/picogent
source:     e7c5f04d227c12eede0456d11a5ddcbb6636cee4
runtime:    go1.26.6 darwin/arm64
harness:    TestDarwinSameUIDParentSwapConfinement
            TestDarwinSameUIDWorkspaceParentSwapConfinement
observed:   2026-09-06T12:10:48Z
```

## Commands

```sh
PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT=/absolute/path/outside/checkout/hostile-parent-swap-evidence.json \
  go test ./internal/securefile -run '^TestDarwinSameUIDParentSwapConfinement$' -count=1

go test ./internal/workspace -run '^TestDarwinSameUIDWorkspaceParentSwapConfinement$' -count=1
go test -race ./internal/securefile ./internal/workspace -run 'TestDarwinSameUID' -count=1
```

Unsupported platforms skip these Darwin-tagged tests. Missing attacker
activity is `INCONCLUSIVE`; an outside-sentinel mutation is `FAIL`.

## Observed securefile campaign

One retained digest-only artifact at the source checkpoint reported:

| Operation | Attempts | Successes | Errors | Attacker swaps | Escape | Verdict |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| `securefile-write-atomic` | 250 | 247 | 3 | 27 | no | `PASS` |
| `securefile-read-file` | 250 | 46 | 204 | 272 | no | `PASS` |
| `securefile-write-exclusive` | 250 | 247 | 3 | 12 | no | `PASS` |
| `securefile-remove-file` | 250 | 247 | 3 | 13 | no | `PASS` |

Outside sentinel digest was unchanged:

```text
36b7d5f2e58d5fb4252eb5e8895e72fedc9a74652a4494b4e30aec7b9b285cf3
```

Rejected races during the hostile interval are confinement successes. They are
not proof of universal race resistance or cross-process CAS.

## Workspace campaign

The workspace harness swaps a nested workspace directory from a separate
same-UID process while `WriteAtomic`, `OpenRead`, and `Remove` run against
descriptor-anchored paths. Confirmed attacker activity with an unchanged
outside sentinel is required for `PASS`.

## Explicit limits

- Darwin-only; Windows reparse-point and Linux cross-surface claims remain
  outside this record.
- Does not authorize upgrading `hostile-filesystem-toctou` to `PASS`.
- Does not prove arbitrary same-UID races after every final identity check on
  every package surface.
- Does not claim live-provider, rendered, or release readiness.
