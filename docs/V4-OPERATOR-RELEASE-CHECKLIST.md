# V4 operator release decision checklist

Status: human gate only. This checklist does **not** auto-authorize a release,
does **not** set `authorized: true`, and does **not** close
[#453](https://github.com/saiaathish/picogent/issues/453). Parent #450 is
already closed in GitHub; its historical references below are retained for
provenance.

The current exact-head audit packet is
[V4-CURRENT-RESIDUAL-PACKET-166B7D3.md](V4-CURRENT-RESIDUAL-PACKET-166B7D3.md)
at `166b7d37d3f5b07de389741110fa62a48bb6f65e`. Its matrix is
`PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5`: live-provider and current rendered
claims are fail-closed without fresh artifacts, broad hostile TOCTOU remains
`UNVERIFIED`, and release authorization remains `INCONCLUSIVE`. The matrix
artifact digest is
`66516803f5bec85084ff30dc4296ba661dee3c19372f91bc1a25aa4ed8d1c8d8`.

The prior fully rendered candidate for this checklist refresh was
`eabf8d6e35322170f7ab19dbd30d3cfb576f422c`. It is a documentation-only
descendant of behavior candidate
`9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb`. The behavior-bound live-provider
records are documented in
[V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md](V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md);
earlier bounded goal-state records remain documented in
[V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md).

The prior fully rendered three-platform recovery evidence is documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md).
It records `rendered-platform-local=PASS` and
`rendered-cross-platform=PASS` for candidate `eabf8d6`. The exact-head matrix
records the two live-provider rows as `UNVERIFIED`; the retained live-provider
records below remain behavior-bound to `9c1ca2e` under the documented
`DOCS_ONLY_DESCENDANT` continuity contract;
all separate historical packets remain separate:

- Historical live-provider continuity from behavior SHA
  `b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5`:
  [V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md](V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md).
- Historical rendered cross-platform evidence at behavior SHA
  `8b022e82be4b2e8ade0a502c5f373eaf6dc61160`:
  [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md).

The underlying historical live observation remains
[V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).
These packets preserve evidence provenance and do **not** combine separate
matrices into a current release-authorizing result. The audit narrative is in
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
| Historical goal-state persistence at `eccebd2` | [V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md); retained artifacts from CI `34541903769` | Linux, Windows, and macOS bounded records are `PASS` for their exact candidate; do not project this earlier behavior evidence through later non-documentation changes |
| Behavior-bound live-provider packet at `9c1ca2e` | [V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md](V4-LIVE-PROVIDER-EVIDENCE-9C1CA2E.md); matrix digest `a49c2bbb74d13f7a2053f3cbd131c7fc8aaf6dc32a295ccea2c87e2c79126293` | Task-owned connectivity and fixed no-tool quality are `PASS`; rendered, hostile, and release rows remain independently bounded |
| Last fully rendered packet at `eabf8d6` | [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-EABF8D6.md); aggregate digest `e59d54ebd6f75403974744432b03e28ba2725654c4a11e264a868d4842a0f681`; matrix digest `32c49185692511abf753e678d682b668bda025bf14741b5877e3342e53c30e17` | Darwin, Linux, and Windows rendered recovery are `PASS` at the historical exact candidate; the prior `9a11719` checkpoint also had a separate Darwin-only packet, while current cross-platform, broad TOCTOU, and release authorization remain separately bounded |
| Prior Darwin packet at `9a11719` | [V4-RENDERED-DARWIN-EVIDENCE-9A11719.md](V4-RENDERED-DARWIN-EVIDENCE-9A11719.md); observation digest `e43f4c0d450d4a17f93a15e26a1242607dc55a6f115da7514fb07d71143147ef`; platform digest `517ebe333f3b80ade0471f94100acabb1b7a4b96e17719255706072fd6bdb94c`; matrix digest `c2e11de481c22087d9ac7ca9c8c7db1b40c61b2739d5eccfd9ac6f6d00c1baba` | Darwin/arm64 Safe permission, contained Allow, Undo, and fresh-process reload were `PASS`; no cross-platform or live-provider upgrade; broad TOCTOU and release authorization remain separate gates |
| Live-provider packet | [V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md) | Connectivity and fixed-no-tool quality are `PASS` for behavior `b7b5c5c`; provider identity and raw result semantics remain self-reported |
| Live-provider continuity at `8b022e8` | digest `3260157d65e09aa43669bc565f76b0912a782fc7a0174fd3e28c3f0b96237ebd`; summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3 | `DOCS_ONLY_DESCENDANT`, `HEAD=PASS`, `tree=CLEAN`; live rows `PASS`, rendered rows `UNVERIFIED` |
| Historical live-provider matrix at `b7b5c5c` | digest `5be231cd13d31915753c1743365bbe0582f47accd3e210f6f493b8f4264d074f`; summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3 | Exact behavior observation; continuity does not rebind rendered evidence |
| Live-provider matrix at `0e2b156` | digest `824e1072b4d7fc9acf9ef00b683b376632febf8556c1ded2282eb486510fe0a9`; summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3 | Provider identity and raw result semantics remain self-reported; this is not the rendered candidate matrix |
| Historical live continuity matrix | [continuity record](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md#docs-only-continuity-refresh-at-2dd8e1b); candidate `2dd8e1b`, behavior `0e2b156`, digest `764a940da63d5003be6a6f3d302752ffbaa230010ab9f23a859a26cf60e94235` | `DOCS_ONLY_DESCENDANT`, `HEAD=PASS`, `tree=CLEAN`; live-provider rows `PASS`, rendered rows `UNVERIFIED` |
| Historical rendered cross-platform packet | [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md) | Retain separately; the prior fully rendered packet is recorded above at `eabf8d6` |
| Rendered exact-candidate matrix at `8b022e8` | digest `b1a735b0a377d2b519e6306fb11308ef10498a5d612680e4a858d4c4ad772ae2`; summary PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3 | `rendered-platform-local=PASS`, `rendered-cross-platform=PASS`; live-provider rows and broad TOCTOU remain `UNVERIFIED` |
| Historical rendered packet at `ad34bd5` | [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md) | Retain separately; do not project it onto the current candidate |
| Historical evidence continuity | `0e2b156` → `ad34bd5` with intervening paths under `docs/` only | Retain the historical claim packets separately; do not synthesize a release matrix by unioning their PASS rows |
| Exact-tip hosted changes | [PR #610](https://github.com/saiaathishkarthik/picogent/pull/610), [#611](https://github.com/saiaathishkarthik/picogent/pull/611), [#612](https://github.com/saiaathishkarthik/picogent/pull/612), [#614](https://github.com/saiaathishkarthik/picogent/pull/614) | Hosted checks and merge history cover the three platform records and aggregate ledger; they do not authorize release |
| Hostile split | [#542](https://github.com/saiaathish/picogent/pull/542) + parent-swap docs | Parent-swap PASS ≠ TOCTOU PASS |
| Perf honesty | [#551](https://github.com/saiaathish/picogent/pull/551) / [V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md) | Alloc gains **PROVED**; outcome-quality gains remain UNPROVED |
| Tip evidence docs | [#619](https://github.com/saiaathishkarthik/picogent/issues/619), [#597](https://github.com/saiaathishkarthik/picogent/issues/597), [#613](https://github.com/saiaathishkarthik/picogent/issues/613), and this packet | Current live-provider evidence and older rendered evidence remain separate; release remains unauthorized |

Historical operator-local retained roots (outside checkout):

```text
/private/tmp/picogent-live-0e2b156/
/private/tmp/picogent-live-619.R6pSMq/
/private/tmp/picogent-rendered-current-8b022e8/darwin/evidence/
/private/tmp/picogent-rendered-current-8b022e8/linux/rendered-linux-owned-browser-8b022e82be4b2e8ade0a502c5f373eaf6dc61160/picogent-linux-rendered-evidence/
/private/tmp/picogent-rendered-current-8b022e8/windows/rendered-windows-evidence-8b022e82be4b2e8ade0a502c5f373eaf6dc61160/
/private/tmp/picogent-rendered-current-8b022e8/aggregate/rendered-cross-platform-evidence.json
/private/tmp/picogent-rendered-current-8b022e8/aggregate/runtime-boundary-matrix-rendered.json
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
/private/tmp/picogent-rendered-darwin-609/evidence/
/private/tmp/picogent-rendered-linux-607/rendered-linux-owned-browser-ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c/picogent-linux-rendered-evidence/
/private/tmp/picogent-rendered-windows-608/rendered-windows-evidence-ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c/
/private/tmp/picogent-rendered-aggregate-ad34/
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
| Closing [#453](https://github.com/saiaathish/picogent/issues/453) | Keep open until residuals accepted **and** release decision recorded; #450 is already closed |
| `hostile-filesystem-toctou=PASS` | Remains `UNVERIFIED` unless separately proved |
| Benchmark / outcome-quality “gains” | Outcome-quality gains remain **UNPROVED**; tip alloc cuts are proved in `#551` only |
| Live streaming / tool-use / multi-hour recovery | Outside tip live evidence PASS rows |
| Fabricating a unified current-tip `PASS` | No fresh current live-provider or rendered-platform packet was supplied for `166b7d3`; historical `3535bda`, `eabf8d6`, and `9a11719` packets remain separate records. Broad TOCTOU remains `UNVERIFIED` and release authorization remains `INCONCLUSIVE`. Historical artifacts must not be unioned, and any later non-docs candidate requires fresh collection |

## Current dry-run posture (no approval observed)

| Field | Value |
| --- | --- |
| Current main evidence checkpoint | `166b7d37d3f5b07de389741110fa62a48bb6f65e` |
| Current rendered evidence candidate | None supplied for this exact-head refresh; `rendered-platform-local` and `rendered-cross-platform` are `UNVERIFIED` |
| Live-provider evidence | None supplied for this exact-head refresh; connectivity and quality are `UNVERIFIED` |
| Rendered evidence | Historical packets remain separate; no current rendered packet is projected onto `166b7d3` |
| Release matrix posture | Exact-head matrix is PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5; no synthetic union; broad TOCTOU remains `UNVERIFIED` and `release-authorization` remains `INCONCLUSIVE` |
| `release-authorization` | `INCONCLUSIVE` |
| `authorized` | `false` |
| Operator approval | **Absent — do not auto-sign** |

This checklist requests a human decision. It does not request approval in-band
from automation and does not claim the goal is complete.
