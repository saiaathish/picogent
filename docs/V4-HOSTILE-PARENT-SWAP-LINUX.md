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
source:     bace7ecbf4da0118c543066beacdbff6a058f857
runtime:    Go 1.25.x linux/amd64 (GitHub Actions ubuntu-24.04)
harness:    TestLinuxSameUIDParentSwapConfinement
            TestLinuxSameUIDWorkspaceParentSwapConfinement
 observed:   2026-09-06T14:02:06Z
workflow:   https://github.com/saiaathish/picogent/actions/runs/34037551757
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
| `securefile-write-atomic` | 250 | 240 | 10 | 39 | no | `PASS` |
| `securefile-read-file` | 250 | 35 | 215 | 440 | no | `PASS` |
| `securefile-write-exclusive` | 250 | 59 | 191 | 587 | no | `PASS` |
| `securefile-remove-file` | 250 | 1 | 249 | 453 | no | `PASS` |

```text
artifact-sha256=ce769fd8eb2a316de9e6f1946b30e01c31df1cdd19aa03beb69f153f6b73fa61
```

## Observed workspace campaign

Confirmed attacker activity with unchanged outside sentinel and complete
outside-tree digests:

```text
attempts=200 attacker_swaps=974
write 183/17 read 172/28 remove 171/29
outside-sentinel=2d883261d326d6a778df1d7e2b707aa8943b1e228147d98c1ac56091b57835ab
outside-tree=a85c5826be93a55b91348e51a239116121669afd139fce0eb3553258d69ac593
artifact-sha256=c1b4758ebbd97f27e18dc034e4a48345a6bdc7b31bb169b86ac868f04d675756
verdict=PASS
```

## Explicit limits

- Linux-only; Windows remains outside this slice.
- Confirmed attacker activity with unchanged outside sentinel is required for
  `PASS`.
- Broad arbitrary same-UID TOCTOU across every surface remains `UNVERIFIED`.
- Not release authorization.
