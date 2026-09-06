# v4 Linux same-UID parent-swap confinement evidence

Status: bounded Linux-only `PASS` for descriptor/handle-anchored parent
replacement confinement under a separate same-UID attacker process. This
record belongs to [#504](https://github.com/saiaathish/picogent/issues/504)
under parent [#453](https://github.com/saiaathish/picogent/issues/453). It
complements the Darwin harness from
[#496](https://github.com/saiaathish/picogent/issues/496).

It does **not** upgrade `hostile-filesystem-toctou`.

## Provenance

```text
repository: github.com/saiaathish/picogent
source:     90b611e3307f4d36a45cce2e47a10d2f0be32541
runtime:    go1.25.14 linux/arm64 (Docker golang:1.25-bookworm)
harness:    TestLinuxSameUIDParentSwapConfinement
            TestLinuxSameUIDWorkspaceParentSwapConfinement
observed:   2026-09-06T13:10:21Z
```

## Commands

```sh
PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT=/absolute/path/outside/checkout/hostile-parent-swap-linux.json \
  go test ./internal/securefile -run '^TestLinuxSameUIDParentSwapConfinement$' -count=1

PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT=/absolute/path/outside/checkout/hostile-workspace-parent-swap-linux.json \
  go test ./internal/workspace -run '^TestLinuxSameUIDWorkspaceParentSwapConfinement$' -count=1

go test -race ./internal/securefile ./internal/workspace -run 'TestLinuxSameUID' -count=1
```

Non-Linux platforms skip these build-tagged tests. Missing attacker activity
is `INCONCLUSIVE`; an outside-sentinel mutation, outside-tree mutation, or
same-name outside marker read is `FAIL`.

## Observed securefile campaign

One retained digest-only artifact at the source checkpoint reported:

| Operation | Attempts | Successes | Errors | Attacker swaps | Escape | Verdict |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| `securefile-write-atomic` | 250 | 56 | 194 | 1460 | no | `PASS` |
| `securefile-read-file` | 250 | 18 | 232 | 1152 | no | `PASS` |
| `securefile-write-exclusive` | 250 | 39 | 211 | 1246 | no | `PASS` |
| `securefile-remove-file` | 250 | 1 | 249 | 1083 | no | `PASS` |

```text
artifact-sha256=474e680af6095ede86d5d5f4aab6f554448fb95b74dc6e3ee25129c385f0cd6a
```

## Observed workspace campaign

Confirmed attacker activity with unchanged outside sentinel and complete
outside-tree digests:

```text
attempts=200 attacker_swaps=6268
write 27/173 read 11/189 remove 12/188
outside-sentinel=2d883261d326d6a778df1d7e2b707aa8943b1e228147d98c1ac56091b57835ab
outside-tree=a85c5826be93a55b91348e51a239116121669afd139fce0eb3553258d69ac593
artifact-sha256=f6ded21d60c71d2bdba9824f2c0d36a7e21b5b40d48c233f84c73ba3b4b2f3e3
verdict=PASS
```

## Explicit limits

- Linux-only; Windows remains outside this slice.
- Confirmed attacker activity with unchanged outside sentinel is required for
  `PASS`.
- Broad arbitrary same-UID TOCTOU across every surface remains `UNVERIFIED`.
- Not release authorization.
