# v4 macOS same-UID parent-swap confinement evidence

Status: bounded Darwin-only `PASS` for descriptor/handle-anchored parent
replacement confinement under a separate same-UID attacker process. This
record belongs to [#496](https://github.com/saiaathish/picogent/issues/496)
under parent [#453](https://github.com/saiaathish/picogent/issues/453).

It does **not** upgrade the broad runtime-boundary row
`hostile-filesystem-toctou`. That claim remains `UNVERIFIED`.

The attacker protocol exercised here renames the trusted parent, presents a
symlink to the outside directory at the original name, and restores the
trusted parent. Ordinary same-UID replacement with a different attacker-owned
directory, including the approval-to-execution workspace-root identity gap,
is a separate unresolved boundary.

## Provenance

```text
repository: github.com/saiaathish/picogent
source:     effe52a21f770bc435bbb1596df1452339cbd82a
runtime:    go1.26.6 darwin/arm64
harness:    TestDarwinSameUIDParentSwapConfinement
            TestDarwinSameUIDWorkspaceParentSwapConfinement
observed:   2026-09-06T12:23:24Z securefile / 2026-09-06T12:23:44Z workspace
```

## Commands

```sh
PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT=/absolute/path/outside/checkout/hostile-parent-swap-evidence.json \
  go test ./internal/securefile -run '^TestDarwinSameUIDParentSwapConfinement$' -count=1

PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT=/absolute/path/outside/checkout/hostile-workspace-parent-swap-evidence.json \
  go test ./internal/workspace -run '^TestDarwinSameUIDWorkspaceParentSwapConfinement$' -count=1

go test -race ./internal/securefile ./internal/workspace -run 'TestDarwinSameUID' -count=1
```

Unsupported platforms skip these Darwin-tagged tests. Missing attacker
activity is `INCONCLUSIVE`; an outside-sentinel mutation, outside-tree
mutation, or same-name outside marker read is `FAIL`.

## Observed securefile campaign

One retained digest-only artifact at the source checkpoint reported:

| Operation | Attempts | Successes | Errors | Attacker swaps | Escape | Verdict |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| `securefile-write-atomic` | 250 | 238 | 12 | 29 | no | `PASS` |
| `securefile-read-file` | 250 | 52 | 198 | 351 | no | `PASS` |
| `securefile-write-exclusive` | 250 | 247 | 3 | 27 | no | `PASS` |
| `securefile-remove-file` | 250 | 1 | 249 | 361 | no | `PASS` |

Outside sentinel digest was unchanged:

```text
36b7d5f2e58d5fb4252eb5e8895e72fedc9a74652a4494b4e30aec7b9b285cf3
```

The complete outside-tree digest was also unchanged:

```text
before=450c60f53d635f3d1480465244c59e59d517c1d4ea07d3d7caa78de36e999d64
after=450c60f53d635f3d1480465244c59e59d517c1d4ea07d3d7caa78de36e999d64
artifact-sha256=1823593e9d9de2ed5bfec45ccd519da005cdaab550e197f8b38bf91bfdc56273
```

Rejected races during the hostile interval are confinement successes. They are
not proof of universal race resistance or cross-process CAS.

## Workspace campaign

The workspace harness swaps a nested workspace directory from a separate
same-UID process while `WriteAtomic`, `OpenRead`, and `Remove` run against
descriptor-anchored paths. Confirmed attacker activity with unchanged
sentinel and complete outside-tree digests is required for `PASS`.

The exact-head workspace artifact reported 200 operation rounds and 1,475
confirmed swaps. It recorded 191/9 write successes/errors, 8/192 read
successes/errors, and 4/196 remove successes/errors. Both digests were
unchanged:

```text
sentinel=2d883261d326d6a778df1d7e2b707aa8943b1e228147d98c1ac56091b57835ab
tree-before=a85c5826be93a55b91348e51a239116121669afd139fce0eb3553258d69ac593
tree-after=a85c5826be93a55b91348e51a239116121669afd139fce0eb3553258d69ac593
artifact-sha256=8dfe6a54f26bc664e5972e85ebb0180a896111ebeb8b33953cfd9b44c6da03ce
```

## Explicit limits

- Darwin-only; Windows reparse-point and Linux cross-surface claims remain
  outside this record.
- Does not authorize upgrading `hostile-filesystem-toctou` to `PASS`.
- Does not cover ordinary attacker-directory replacement after permission
  approval; the workspace-root identity boundary remains a separate issue.
- Does not prove arbitrary same-UID races after every final identity check on
  every package surface.
- Does not claim live-provider, rendered, or release readiness.
