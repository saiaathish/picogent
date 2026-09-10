# V4 operator release decision checklist

Status: human gate only. This checklist does **not** auto-authorize a release,
does **not** set `authorized: true`, and does **not** close
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Current evidence is split across two exact behavior checkpoints, each bound to
its own claim family:

- Live-provider evidence at behavior SHA
  `0e2b15624b29c256c4b22364ecf6970890c90374`:
  [V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md).
- Rendered cross-platform evidence at behavior SHA
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`:
  [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md).

The `ad34bd5` candidate is a documentation-only descendant of `0e2b156`, and
the later `main` updates retain both records under that continuity boundary.
This preserves evidence provenance; it does **not** combine the two matrices
into a release-authorizing result. The audit narrative is in
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
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
candidate. The retained live-provider and rendered observations above are
separate continuity-bound records, not a synthetic union; the release
predicate still needs one exact-SHA matrix containing every required
non-residual claim:

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
| Live-provider packet | [V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md) | Connectivity and fixed-no-tool quality are `PASS` at `0e2b156`; rendered rows remain `UNVERIFIED` there |
| Live-provider matrix at `0e2b156` | digest `824e1072b4d7fc9acf9ef00b683b376632febf8556c1ded2282eb486510fe0a9`; summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3 | Provider identity and raw result semantics remain self-reported; this is not the rendered candidate matrix |
| Current-main live continuity matrix | [continuity record](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md#docs-only-continuity-refresh-at-2dd8e1b); candidate `2dd8e1b`, behavior `0e2b156`, digest `764a940da63d5003be6a6f3d302752ffbaa230010ab9f23a859a26cf60e94235` | `DOCS_ONLY_DESCENDANT`, `HEAD=PASS`, `tree=CLEAN`; live-provider rows `PASS`, rendered rows `UNVERIFIED` |
| Rendered cross-platform packet | [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md) | Darwin/Linux/Windows are `PASS` at exact candidate `ad34bd5`; no live-provider claim follows |
| Rendered exact-candidate matrix at `ad34bd5` | digest `0edb2195f34cd53af4aa4574f82f1f3cd864b319361d427a1d179df6b5c1f952`; summary PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4 | `rendered-cross-platform=PASS`; live-provider rows and broad TOCTOU remain `UNVERIFIED` |
| Evidence continuity | `0e2b156` → `ad34bd5` with intervening paths under `docs/` only | Retain the two claim packets separately; do not synthesize a release matrix by unioning their PASS rows |
| Exact-tip hosted changes | [PR #610](https://github.com/saiaathishkarthik/picogent/pull/610), [#611](https://github.com/saiaathishkarthik/picogent/pull/611), [#612](https://github.com/saiaathishkarthik/picogent/pull/612), [#614](https://github.com/saiaathishkarthik/picogent/pull/614) | Hosted checks and merge history cover the three platform records and aggregate ledger; they do not authorize release |
| Hostile split | [#542](https://github.com/saiaathish/picogent/pull/542) + parent-swap docs | Parent-swap PASS ≠ TOCTOU PASS |
| Perf honesty | [#551](https://github.com/saiaathish/picogent/pull/551) / [V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md) | Alloc gains **PROVED**; outcome-quality gains remain UNPROVED |
| Tip evidence docs | [#597](https://github.com/saiaathishkarthik/picogent/issues/597), [#613](https://github.com/saiaathishkarthik/picogent/issues/613), and this packet | Live-provider and rendered evidence are both retained, but separate; release remains unauthorized |

Current operator-local retained roots (outside checkout):

```text
/private/tmp/picogent-live-0e2b156/
/private/tmp/picogent-rendered-darwin-609/evidence/
/private/tmp/picogent-rendered-linux-607/rendered-linux-owned-browser-ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c/picogent-linux-rendered-evidence/
/private/tmp/picogent-rendered-windows-608/rendered-windows-evidence-ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c/
/private/tmp/picogent-rendered-aggregate-ad34/rendered-cross-platform-evidence.json
/private/tmp/picogent-rendered-aggregate-ad34/runtime-boundary-matrix.json
```

Historical (do not project onto tip):

```text
/private/tmp/picogent-evidence-f8a78c6/
/private/tmp/picogent-rendered-darwin-3f-collector-output.oIIuHB/
/private/tmp/picogent-rendered-linux-507-artifact.MC0E0Y/
/private/tmp/picogent-rendered-windows-507-artifact.OlfWSr/
/private/tmp/picogent-rendered-cross-platform-3f-aggregate.L5cYfm/
/private/tmp/picogent-rendered-cross-platform-3f-matrix-local.jGT2s2/
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
| Fabricating a unified docs-tip `PASS` | The live packet is bound to `0e2b156` and the rendered aggregate to `ad34bd5`; separate PASS rows must not be unioned for release authorization, and any later non-docs candidate requires fresh collection |

## Current dry-run posture (no approval observed)

| Field | Value |
| --- | --- |
| Docs tip | Documentation-only descendants retain separate packets; bind the release SHA and one exact-SHA matrix separately |
| Live-provider evidence SHA | `0e2b15624b29c256c4b22364ecf6970890c90374` |
| Live-provider matrix | PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3; live-provider rows `PASS`, rendered rows `UNVERIFIED` |
| Rendered evidence SHA | `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c` |
| Rendered exact-candidate matrix | PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4; `rendered-cross-platform=PASS`, live-provider rows `UNVERIFIED` |
| Release matrix posture | No synthetic union of the two matrices; `release-authorization` remains `INCONCLUSIVE` |
| `release-authorization` | `INCONCLUSIVE` |
| `authorized` | `false` |
| Operator approval | **Absent — do not auto-sign** |

This checklist requests a human decision. It does not request approval in-band
from automation and does not claim the goal is complete.
