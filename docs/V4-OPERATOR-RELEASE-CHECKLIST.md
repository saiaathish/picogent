# V4 operator release decision checklist

Status: human gate only. This checklist does **not** auto-authorize a release,
does **not** set `authorized: true`, and does **not** close
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Tip-bound evidence packet:
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) (historical anchors;
prefer tip packet [V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md)).
Formal TOCTOU residual-acceptance package:
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md)
([acceptance record stub](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md)).
Predicate contract (including the shorter approval list from
[#544](https://github.com/saiaathish/picogent/pull/544)):
[V4-RELEASE-AUTHORIZATION.md](V4-RELEASE-AUTHORIZATION.md).

## Decision anchors the operator must bind

Before supplying
`OperatorApproval{Approved: true, Scope: "v4-release", ...}` to a trusted
workflow, confirm all of the following bind to the **same** intended release
candidate (today: behavior evidence at
`f8a78c646877164009953401772acdb4d346175d`; docs tip
`71c31cadf0538610bd1637621a028f9b4cbb4bda` is a docs-only descendant — use
`--behavior-sha f8a78c646877164009953401772acdb4d346175d` for retention):

1. Exact `main` tip identity for the release decision SHA, clean worktree, and
   verification-manifest `head.match=PASS`.
2. Hosted required jobs (`test`, `security`) plus dependent
   `release-evidence` / `production-artifacts` succeeded at that SHA.
3. Verification manifest overall `PASS` under the targeted-only coverage
   contract ([#544](https://github.com/saiaathish/picogent/pull/544)).
4. Runtime-boundary matrix bound to the same SHA (or explicit
   docs-only / behavior-SHA continuity where the matrix contract allows), with
   every **non-residual** claim required by the predicate `PASS`.
5. Trusted release-attestation validation `PASS` for the same SHA.
6. Approving actor identity recorded by the trusted workflow that constructs
   the approval input (this package does not authenticate the actor).

A dry-run without step 6 must remain `authorized: false` even when steps 1–5
are green.

## Evidence the operator should review

| Evidence | Where / digest | What to confirm |
| --- | --- | --- |
| Tip rebound packet | [V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md) | Packet stays **NOT COMPLETE**; no completion claim |
| Exact-SHA matrix at behavior `f8a78c6` | digest `f0b4bca1…`; summary PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 | `rendered-cross-platform=PASS`; live PASS; TOCTOU UNVERIFIED |
| Three-platform aggregate | SHA-256 `5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd` | darwin/linux/windows `PASS` at `f8a78c6` only |
| Live provider | [V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md](V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md) · `df823bd9…` / `2123344d…` | Connectivity + fixed-no-tool quality PASS |
| Windows collect | [GHA 34112789982](https://github.com/saiaathish/picogent/actions/runs/34112789982) | Hosted SUCCEEDED collect at tip behavior |
| Hostile split | [#542](https://github.com/saiaathish/picogent/pull/542) + parent-swap docs | Parent-swap PASS ≠ TOCTOU PASS |
| Perf honesty | [#551](https://github.com/saiaathish/picogent/pull/551) / [V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md) | Alloc gains **PROVED**; outcome-quality gains remain UNPROVED |
| Tip evidence docs | [#552](https://github.com/saiaathish/picogent/pull/552) + this live package | Docs landings do not authorize release |

Operator-local retained roots (outside checkout):

```text
/private/tmp/picogent-evidence-f8a78c6/
```

Historical (do not project onto tip):

```text
/private/tmp/picogent-evidence-423d047/
/private/tmp/picogent-evidence-docs-tip-70dba64/
```

## Explicit residual acceptance (required if authorizing while open)

If the operator authorizes while
`hostile-filesystem-toctou` remains `UNVERIFIED`, that acceptance must be
conscious and recorded as a residual audit boundary. Bind the formal package
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md) and complete
[V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md)
(or an equivalent out-of-tree copy):

- Parent-swap confinement `PASS` does **not** upgrade TOCTOU.
- The predicate may allow residual TOCTOU without blocking; that is an
  eligibility rule, not a proof that TOCTOU is closed.
- Do **not** rewrite matrix docs or issue trackers to claim TOCTOU `PASS`.

## What remains blocked even after approval

Operator approval makes the release-authorization **predicate eligible**. It
does **not**, by itself:

| Still blocked / unchanged | Why |
| --- | --- |
| Publish / tag / deploy | Predicate does not publish |
| `UpdateGoal complete` / “v4.0 complete” | Goal completion is a separate human product decision |
| Closing [#450](https://github.com/saiaathish/picogent/issues/450) / [#453](https://github.com/saiaathish/picogent/issues/453) | Keep open until residuals accepted **and** release decision recorded |
| `hostile-filesystem-toctou=PASS` | Remains `UNVERIFIED` unless separately proved |
| Benchmark / outcome-quality “gains” | Outcome-quality gains remain **UNPROVED**; tip alloc cuts are proved in `#551` only |
| Live streaming / tool-use / multi-hour recovery | Outside tip live evidence PASS rows |
| Fabricating docs-tip `rendered-cross-platform=PASS` at candidate `71c31ca` | Aggregate is exact-candidate-bound to `f8a78c6` |

## Current dry-run posture (no approval observed)

| Field | Value |
| --- | --- |
| Docs tip | `71c31cadf0538610bd1637621a028f9b4cbb4bda` |
| Behavior SHA | `f8a78c646877164009953401772acdb4d346175d` |
| Matrix (exact head) | PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 |
| `release-authorization` | `INCONCLUSIVE` |
| `authorized` | `false` |
| Operator approval | **Absent — do not auto-sign** |

This checklist requests a human decision. It does not request approval in-band
from automation and does not claim the goal is complete.
