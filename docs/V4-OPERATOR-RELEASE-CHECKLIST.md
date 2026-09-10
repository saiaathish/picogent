# V4 operator release decision checklist

Status: human gate only. This checklist does **not** auto-authorize a release,
does **not** set `authorized: true`, and does **not** close
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Current behavior-evidence packet:
[V4-TIP-EVIDENCE-492595F.md](V4-TIP-EVIDENCE-492595F.md), with the audit
narrative in [V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
The prior [V4-TIP-EVIDENCE-1531850.md](V4-TIP-EVIDENCE-1531850.md),
[V4-TIP-EVIDENCE-97A3EC5.md](V4-TIP-EVIDENCE-97A3EC5.md),
[V4-TIP-EVIDENCE-3F5543D.md](V4-TIP-EVIDENCE-3F5543D.md),
[V4-TIP-EVIDENCE-1CE2059.md](V4-TIP-EVIDENCE-1CE2059.md), and
[V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md) packets are historical
only.
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
candidate (current behavior evidence source:
`0e2b15624b29c256c4b22364ecf6970890c90374`; later documentation commits are
docs-only descendants and must not be treated as a new behavior observation):

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
| Behavior-evidence packet | [V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md) | Current exact-tip matrix is recorded; release remains **NOT COMPLETE** |
| Exact-SHA matrix at behavior `0e2b156` | digest `824e1072b4d7fc9acf9ef00b683b376632febf8556c1ded2282eb486510fe0a9`; summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3 | Live connectivity and fixed quality are PASS; rendered rows and broad TOCTOU remain UNVERIFIED |
| Exact-tip hosted changes | [PR #596](https://github.com/saiaathishkarthik/picogent/pull/596) | Required hosted checks passed for the docs-only exact-head evidence refresh |
| Current three-platform rendered aggregate | None supplied at `0e2b156`; prior [record](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md) is historical | Keep `rendered-cross-platform` **UNVERIFIED** until a fresh exact-tip collection exists |
| Current local rendered artifact | None supplied at `0e2b156`; prior record is historical | Keep `rendered-platform-local` **UNVERIFIED** until a fresh exact-tip observation exists |
| Live provider | [V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md) / digests recorded there | Connectivity and fixed-no-tool quality are `PASS`; provider identity and raw result semantics remain self-reported |
| Hostile split | [#542](https://github.com/saiaathish/picogent/pull/542) + parent-swap docs | Parent-swap PASS ≠ TOCTOU PASS |
| Perf honesty | [#551](https://github.com/saiaathish/picogent/pull/551) / [V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md) | Alloc gains **PROVED**; outcome-quality gains remain UNPROVED |
| Tip evidence docs | [#597](https://github.com/saiaathishkarthik/picogent/issues/597) + this packet | Live-provider evidence refresh is complete; rendered evidence remains separate and release is unauthorized |

Operator-local retained roots (outside checkout):

```text
/private/tmp/picogent-evidence-f8a78c6/
/private/tmp/picogent-live-0e2b156/
/private/tmp/picogent-rendered-darwin-3f-collector-output.oIIuHB/
/private/tmp/picogent-rendered-linux-507-artifact.MC0E0Y/
/private/tmp/picogent-rendered-windows-507-artifact.OlfWSr/
/private/tmp/picogent-rendered-cross-platform-3f-aggregate.L5cYfm/
/private/tmp/picogent-rendered-cross-platform-3f-matrix-local.jGT2s2/
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
| Fabricating docs-tip `rendered-cross-platform=PASS` at another candidate | Historical aggregate is exact-candidate-bound to `3f5543d`; current `0e2b156` row is UNVERIFIED |

## Current dry-run posture (no approval observed)

| Field | Value |
| --- | --- |
| Docs tip | docs-only descendant containing this packet; bind the release SHA separately |
| Behavior evidence SHA | `0e2b15624b29c256c4b22364ecf6970890c90374` |
| Matrix (exact behavior head) | PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3; live-provider rows `PASS`, rendered rows `UNVERIFIED` |
| `release-authorization` | `INCONCLUSIVE` |
| `authorized` | `false` |
| Operator approval | **Absent — do not auto-sign** |

This checklist requests a human decision. It does not request approval in-band
from automation and does not claim the goal is complete.
