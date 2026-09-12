# v4 current hostile-runtime residual packet

Status: current, bounded evidence packet for [#706](https://github.com/saiaathishkarthik/picogent/issues/706)
under parent [#453](https://github.com/saiaathishkarthik/picogent/issues/453).
This packet binds the admitted hostile-runtime observations to the exact
behavior checkpoint below. It is documentation-only evidence; it is not a
security certification, v4-complete claim, release authorization, or reason
to close #453.

## Source and matrix identity

```text
repository:        github.com/saiaathishkarthik/picogent
behavior-sha:      e770897891541c31d592220ae902c7fbc5b3db70
packet-candidate:  b26774dbd6f222ade218eb8b40e4621d18aeef7b
behavior-binding:  DOCS_ONLY_DESCENDANT
matrix:            /private/tmp/picogent-v4-evidence-706/runtime-boundary-matrix-hostile.json
matrix-sha256:     948bdce8dc39fba7c9d7330b7e33d4d3a66fae990f35b71cda2a66257ee06a02
matrix-schema:     picogent.v4.runtime-boundary-matrix.v1
head-match:        PASS
tree:              CLEAN
runtime:           go1.26.6 darwin/arm64
observed:          2026-09-12T01:06:24Z
hosted-ci:         https://github.com/saiaathishkarthik/picogent/actions/runs/34661288831
```

`behavior-sha` is the exact clean `main` checkpoint whose runtime behavior is
being claimed. `packet-candidate` is the documentation descendant that carries
this residual packet. The packet does not pretend that a documentation-only
descendant changed the runtime behavior.

## Admitted matrix result

The hostile matrix records `PASS=3` and `UNVERIFIED=9`.

The three admitted bounded claims are:

| Claim | Evidence | Verdict |
| --- | --- | --- |
| Non-interactive child-environment sanitization | Deterministic `procenv` and hostile MCP/environment tests recorded in `docs/V4-SECURITY-CAMPAIGN.md` | `PASS` |
| Deterministic hostile-filesystem boundary | Bounded `securefile`, `workspace`, and related retained-artifact tests recorded in `docs/V4-HOSTILE-RUNTIME-EVIDENCE.md` | `PASS` |
| Same-UID parent-swap confinement | Separate-process Darwin and Linux `securefile`, workspace, and checkpoint campaigns | `PASS` |

The exact-main hosted run above retained the Linux and Windows records. The
Darwin parent-swap records were rerun from a clean clone at
`behavior-sha`. Every retained parent-swap artifact reported confirmed
attacker activity, no outside escape, and unchanged outside digests.

## Darwin retained artifacts

These are the digest-only artifacts used by the exact-main Darwin rerun:

```text
/private/tmp/picogent-v4-evidence-706/darwin/securefile.json
sha256=1aa6bd69788d1e3d96a85b28c2456075205b824337cd9196b268b08543f80134

/private/tmp/picogent-v4-evidence-706/darwin/workspace.json
sha256=a6c2721ae1f114581bc7b0f744e30840be0af0e23a9c2750ce2401c99659c482

/private/tmp/picogent-v4-evidence-706/darwin/checkpoint.json
sha256=b81df1780ebde62f1c1841827a113ccd92502f27f5c52110ee1c7152063b6848
```

The corresponding repository records are
`docs/V4-HOSTILE-PARENT-SWAP-DARWIN.md` and
`docs/V4-HOSTILE-PARENT-SWAP-LINUX.md`. Their narrowed
`hostile-parent-swap-confinement` result does not upgrade the broader
`hostile-filesystem-toctou` row.

## Explicit residuals

The nine residual rows remain `UNVERIFIED`:

```text
hostile-filesystem-toctou
live-provider-connectivity
live-provider-quality
release-authorization
rendered-cross-platform
rendered-long-horizon-local
rendered-platform-local
rendered-recovery-undo-reload
restart-steer-undo-recovery
```

In particular, the parent-swap harness is not a universal same-UID race proof.
It does not establish resistance to every writer between a final identity check
and a later pathname operation, ordinary attacker-owned directory replacement
after permission approval, every supported platform's reparse/ACL behavior, or
cross-surface TOCTOU safety. Missing, stale, unsupported, or incomplete
artifacts remain fail-closed.

This packet therefore closes only the documentation/provenance slice of #706
when its focused PR checks and post-merge exact-head checks pass. It does not
close #453, authorize a release, or assert that v4 is complete.
