# V4 operator release decision checklist

Status: human gate only. This checklist does **not** auto-authorize a release,
does **not** set `authorized: true`, and does **not** close
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Tip-bound evidence packet:
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
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
`423d0471c1864651473c2f1828b67066885f79bd`; docs tip
`70dba64efe9f96bce27ec6c1d2fb6f9109e5de33` is a docs-only descendant):

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
| Final release + hostile residual packet | [V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) | Packet stays **NOT COMPLETE**; no completion claim |
| Exact-SHA matrix at behavior `423d047` | digest `48cbd34752aabfbd8dea385b7ca13e4c1c343baea1738d096a1b4c8a4c8af748`; summary PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 | `rendered-cross-platform=PASS`; residuals unchanged |
| Three-platform aggregate | SHA-256 `d5e55136fa1af5c00b37b22d6f06d1b31752e66abe027626c23243ecc3b4702f` | darwin/linux/windows `PASS` at `423d047` only |
| Windows collect | [GHA 34090755069](https://github.com/saiaathish/picogent/actions/runs/34090755069); zip `4750b1ae…` | Hosted SUCCEEDED collect matches tip Windows docs |
| Hostile split | [#542](https://github.com/saiaathish/picogent/pull/542) + parent-swap docs | Parent-swap PASS ≠ TOCTOU PASS |
| Perf honesty | [#545](https://github.com/saiaathish/picogent/pull/545) / [V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md) | Gains **UNPROVED**; tip regressions recorded |
| Tip evidence docs | [#546](https://github.com/saiaathish/picogent/pull/546), [#547](https://github.com/saiaathish/picogent/pull/547) | Docs landings do not authorize release |

Operator-local retained roots (outside checkout):

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
| Benchmark / outcome-quality “gains” | Remain **UNPROVED** / inconclusive per [#545](https://github.com/saiaathish/picogent/pull/545) |
| Live streaming / tool-use / multi-hour recovery | Outside tip live evidence PASS rows |
| Fabricating docs-tip `rendered-cross-platform=PASS` at candidate `70dba64` | Aggregate is exact-candidate-bound to `423d047` |

## Current dry-run posture (no approval observed)

| Field | Value |
| --- | --- |
| Docs tip | `70dba64efe9f96bce27ec6c1d2fb6f9109e5de33` |
| Behavior SHA | `423d0471c1864651473c2f1828b67066885f79bd` |
| Matrix | PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 |
| `release-authorization` | `INCONCLUSIVE` |
| `authorized` | `false` |
| Operator approval | **Absent — do not auto-sign** |

This checklist requests a human decision. It does not request approval in-band
from automation and does not claim the goal is complete.
