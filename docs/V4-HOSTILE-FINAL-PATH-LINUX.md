# v4 Linux same-UID final-path replacement evidence

Status: bounded Linux-only `PASS` for final-name replacement confinement in
`securefile` under a separate same-UID attacker process. This record belongs
to [#566](https://github.com/saiaathishkarthik/picogent/issues/566), a child of
the broader observation parent [#453](https://github.com/saiaathishkarthik/picogent/issues/453),
and is implemented by [PR #567](https://github.com/saiaathishkarthik/picogent/pull/567).

It does **not** upgrade `hostile-filesystem-toctou`. It records a narrower
final-path replacement observation for four `securefile` operations.

## Provenance

```text
repository: github.com/saiaathishkarthik/picogent
source:     c44d8825732877aac9b33ca7bfd448a18085918f
runtime:    Go 1.25.x linux/amd64 (GitHub Actions ubuntu-24.04)
harness:    TestLinuxSameUIDFinalPathReplacementConfinement
observed:   2026-09-08T05:30:44Z
workflow:   https://github.com/saiaathishkarthik/picogent/actions/runs/34190476629
```

The source SHA was checked by the harness before evidence retention. The
captured report was produced with `source_tree_modified=false` in a
task-owned disposable directory.

## Attack model and acceptance rule

A separate same-UID helper repeatedly renames the trusted final entry to a
backup, installs a symlink at the original final name pointing at a same-name
file in an outside directory, records the confirmed swap, and restores the
trusted entry. The operation under test runs while the helper is active.

The harness hashes the complete outside tree before and after the campaign and
also checks read results for outside marker content. A campaign is `PASS` only
when attacker swaps are confirmed, trusted in-tree behavior still works, no
outside content is read, and the outside-tree digest is unchanged. Missing
attacker activity is `INCONCLUSIVE`; an outside escape is `FAIL`.

## Observed securefile campaign

The hosted artifact reported four operation-level `PASS` verdicts. Each
operation received 250 attempts:

| Operation | Attempts | Successes | Errors | Attacker swaps | Escape | Verdict |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| `securefile-write-atomic` | 250 | 62 | 188 | 699 | no | `PASS` |
| `securefile-read-file` | 250 | 32 | 218 | 472 | no | `PASS` |
| `securefile-write-exclusive` | 250 | 1 | 249 | 615 | no | `PASS` |
| `securefile-remove-file` | 250 | 1 | 249 | 597 | no | `PASS` |

The aggregate report was:

```text
schema=picogent.v4.hostile-final-path-replacement-evidence.v1
candidate_sha=c44d8825732877aac9b33ca7bfd448a18085918f
attacker_confirmed=true
outside_tree_before_sha256=3c03cebec6156e8347f012e3bf30d25c2421c42d3bb442e88ddd82e48f725b40
outside_tree_after_sha256=3c03cebec6156e8347f012e3bf30d25c2421c42d3bb442e88ddd82e48f725b40
outside_escape_observed=false
verdict=PASS
broad_toctou_claim=UNVERIFIED
```

The retained `final-path.json` digest is
`998df9b1610edddcecae7febb7158eccdb8752c8facf354b4a932e5c87c95ba7`.
It is uploaded in the `hostile-parent-swap-linux-c44d8825732877aac9b33ca7bfd448a18085918f`
artifact alongside the other Linux hostile-runtime evidence.

## Reproduction

Run from a clean checkout whose `HEAD` is the source SHA recorded above, and
retain the report outside the checkout:

```sh
PICOGENT_HOSTILE_FINAL_PATH_SOURCE_SHA="$(git rev-parse HEAD)" \
PICOGENT_HOSTILE_FINAL_PATH_EVIDENCE_OUT=/absolute/path/outside/checkout/final-path.json \
  go test -v ./internal/securefile \
    -run '^TestLinuxSameUIDFinalPathReplacementConfinement$' -count=1

go test -race ./internal/securefile -count=1
```

The hosted pull-request workflow ran this harness on the exact candidate SHA
and also passed the macOS, Windows, Ubuntu, security, release-evidence, and
production-artifacts checks. The production-artifacts workflow was retained at
[run 34190476700](https://github.com/saiaathishkarthik/picogent/actions/runs/34190476700).

## Explicit limits

- Linux/amd64 only; this record is not a Windows hostile-writer observation.
- Only the four named `securefile` operations are exercised here.
- The harness covers a bounded final-name replacement interval; it does not
  prove resistance to every external-process interleaving after every final
  identity check or pathname operation.
- Parent replacement, workspace, checkpoint, rendered, provider, and other
  surfaces have separate evidence records and are not inherited by this one.
- `hostile-filesystem-toctou` remains `UNVERIFIED`; this is not a security
  certification, production safety guarantee, or release authorization.
