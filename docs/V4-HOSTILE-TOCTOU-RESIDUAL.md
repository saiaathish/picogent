# V4 hostile filesystem TOCTOU residual (formal acceptance package)

Status: **residual audit packaging only**. This note is re-bound to the current
documentation tip while the latest retained behavior evidence remains bound to
an earlier candidate. It defines the defensive confinement residual boundary
for `hostile-filesystem-toctou=UNVERIFIED`. It does **not**:

- claim `hostile-filesystem-toctou=PASS`;
- authorize a release or set `authorized: true`;
- close [#450](https://github.com/saiaathish/picogent/issues/450) /
  [#453](https://github.com/saiaathish/picogent/issues/453);
- provide exploit, race-winning, or attack reproduction procedures.

Operator decision checklist:
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).
Tip-bound final release + hostile residual packet:
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
Fillable residual-acceptance record stub:
[V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md).
Predicate contract:
[V4-RELEASE-AUTHORIZATION.md](V4-RELEASE-AUTHORIZATION.md).

## Rebind baseline (main tip `0788a2b`)

This rebind starts from `main` tip
`0788a2b74155d5c46af37c5f7c19c5a3d5c22ef9`. The latest retained runtime
evidence remains bound to behavior candidate `592b07a633d9683354c4abeee09b767bc071ab35`.
Because PR #651 changed code after that candidate, the retained matrix is not
current-main runtime proof and must not be silently re-used as one.

| Anchor | Value |
| --- | --- |
| Base docs / `main` tip at rebind authoring | `0788a2b74155d5c46af37c5f7c19c5a3d5c22ef9` |
| Latest retained behavior candidate | `592b07a633d9683354c4abeee09b767bc071ab35` |
| Three-platform aggregate SHA-256 | `71bfe03c0afb889c9a548e9264cf0ab4509d80f69a85e346fb4501d85a626b05` |
| Runtime matrix SHA-256 | `6048c4244818db694b96b5875f8893f1d84aaf731a4caebc4493bfbff4213817` |
| Retained matrix summary | **PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1** |
| Residual row | `hostile-filesystem-toctou=UNVERIFIED` |
| Narrow persistence hardening | [#651](https://github.com/saiaathish/picogent/pull/651) merged; [#652](https://github.com/saiaathish/picogent/pull/652) removed dead session helpers |

The candidate and current-main anchors are intentionally separate. A fresh
exact-current behavior campaign is required before making claims about the
combined post-#651 code.

## Why this package exists

After [#542](https://github.com/saiaathish/picogent/pull/542), the matrix
splits bounded same-UID **parent-swap confinement** from the broader
**filesystem TOCTOU** residual:

| Claim | Verdict after #542 | Role |
| --- | --- | --- |
| `hostile-parent-swap-confinement` | `PASS` (Darwin + Linux evidence) | Hostile-runtime release gate for **bounded** confinement |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Explicit residual audit boundary |

[#548](https://github.com/saiaathish/picogent/pull/548) landed the tip-bound
final-release packet and operator checklist and already requires conscious
residual acceptance if authorizing while TOCTOU remains open. This document is
the formal residual-acceptance package the operator can bind when recording
that acceptance. Proving a universal TOCTOU PASS would require offensive
race-reproduction procedures outside this packaging track; those are
**out of scope** here by design.

The later boundary inventory ([#649](https://github.com/saiaathish/picogent/pull/649)
and [#650](https://github.com/saiaathish/picogent/pull/650)) identified the
evolution-store surface. [#651](https://github.com/saiaathish/picogent/pull/651)
now routes that store through the shared securefile boundary, and
[#652](https://github.com/saiaathish/picogent/pull/652) removed unreachable raw
session atomic helpers. Those are narrower hardening and cleanup slices; they
do not close the cross-surface residual.

## Historical package baseline (superseded)

| Anchor | Value |
| --- | --- |
| Docs / remote tip at package authoring (`origin/main`, [#548](https://github.com/saiaathish/picogent/pull/548)) | `2ea801d8898b99fbfb10f5afe1464889d2b2cb32` |
| Behavior SHA (matrix / aggregate / live+rendered evidence) | `423d0471c1864651473c2f1828b67066885f79bd` |
| Exact-SHA matrix summary at behavior `423d047` | **PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1** |
| Residual UNVERIFIED row | `hostile-filesystem-toctou` |
| Parent-swap split | [#542](https://github.com/saiaathish/picogent/pull/542) |
| Final release + operator packet | [#548](https://github.com/saiaathish/picogent/pull/548) |

These values are preserved as historical provenance only. Re-bind the current
docs tip and intended release-candidate SHA when signing a residual-acceptance
record. The current post-#651 main tip is not a docs-only descendant of the
latest retained behavior candidate.

## What parent-swap PASS proves

`hostile-parent-swap-confinement=PASS` means both platform evidence records are
present and the bounded harnesses observed confinement under the **named**
same-UID parent-replacement protocol:

- [V4-HOSTILE-PARENT-SWAP-DARWIN.md](V4-HOSTILE-PARENT-SWAP-DARWIN.md)
- [V4-HOSTILE-PARENT-SWAP-LINUX.md](V4-HOSTILE-PARENT-SWAP-LINUX.md)

That PASS is limited to descriptor/handle-anchored operations and the
ancestor/parent-swap families those records name (securefile, workspace,
checkpoint as documented). Rejected races during the hostile interval are
**confinement successes** for those families. They are **not**:

- proof of universal race resistance;
- proof against every cross-surface pathname boundary;
- proof that arbitrary same-UID writers cannot race every open/read/write/
  rename path Picogent or its children may use;
- an upgrade of `hostile-filesystem-toctou` from `UNVERIFIED` to `PASS`.

Deterministic hostile controls
([V4-HOSTILE-RUNTIME-EVIDENCE.md](V4-HOSTILE-RUNTIME-EVIDENCE.md)) remain a
separate narrower row (`hostile-filesystem-deterministic`) and likewise do not
upgrade the TOCTOU residual.

## What remains UNVERIFIED

`hostile-filesystem-toctou` stays `UNVERIFIED` because the matrix claim is the
**broader** arbitrary same-UID pathname-race residual across surfaces, not the
bounded parent-swap confinement gate. Explicitly outside the proved boundary:

- Universal "no TOCTOU anywhere" product claims.
- Surfaces and pathname families not covered by the parent-swap / deterministic
  evidence records.
- Treating parent-swap harness PASS counts, digests, or CI greenness as a
  TOCTOU PASS.
- Closing [#450](https://github.com/saiaathish/picogent/issues/450) /
  [#453](https://github.com/saiaathish/picogent/issues/453) solely because
  parent-swap PASS landed.

This residual package deliberately does **not** document how to widen the
claim with race-reproduction or attack procedures. Any future TOCTOU PASS must
come from a separate, bounded, defensive evidence track with its own matrix
contract—not from rewriting this residual into a fabricated PASS.

## Predicate relationship (eligibility ≠ proof)

Per [V4-RELEASE-AUTHORIZATION.md](V4-RELEASE-AUTHORIZATION.md):

- The `hostile_runtime` category is complete when the bounded hostile claims
  required by the matrix are `PASS`, including
  `hostile-parent-swap-confinement` when that row is present.
- The residual `hostile-filesystem-toctou` row **may remain `UNVERIFIED`
  without blocking** the release-authorization predicate.
- That rule is an **eligibility / audit-boundary** rule. It is **not** proof
  that TOCTOU is closed.

Dry-run without operator approval remains `INCONCLUSIVE` /
`authorized: false` even when non-residual lanes PASS
([V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md)).

## Operator residual-acceptance statement (template)

Use this statement (or the fillable record in
[V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md))
when authorizing while the residual remains open. Keep a copy outside the
checkout if the trusted workflow requires an out-of-tree record.

```text
I authorize release eligibility for Picogent v4 at candidate SHA
________ while consciously accepting the residual audit boundary:

  hostile-filesystem-toctou = UNVERIFIED

I affirm that:
  - hostile-parent-swap-confinement PASS (PR #542) is bounded confinement only
    and does not upgrade hostile-filesystem-toctou to PASS;
  - the release-authorization predicate may treat this residual as non-blocking
    eligibility, which is not a TOCTOU closure claim;
  - matrix docs and issue trackers must not be rewritten to claim TOCTOU PASS;
  - issues #450 and #453 remain open until residuals are accepted and the
    release decision is recorded (acceptance here is eligibility, not
    UpdateGoal complete / publish / deploy).

Approved residual: yes / no
Approver name: ________
Date (UTC): ________
Candidate / behavior SHA: ________
Docs tip SHA (if docs-only descendant): ________
Record reference: docs/V4-HOSTILE-TOCTOU-RESIDUAL.md + this statement
```

## What this package does not do

| Action | Status |
| --- | --- |
| Claim `hostile-filesystem-toctou=PASS` | **Forbidden** here |
| Auto-supply operator approval | **No** |
| Publish / tag / deploy | **Out of scope** |
| Close #450 / #453 | **Do not close** from this package |
| Mark v4.0 / UpdateGoal complete | **Separate human product decision** |
| Add exploit / PoC / "how to win the race" content | **Out of scope by design** |

## Related links

- Matrix hostile split: [V4-RUNTIME-BOUNDARY-MATRIX.md](V4-RUNTIME-BOUNDARY-MATRIX.md)
- Parent-swap Darwin / Linux evidence (bounded PASS only)
- [#542](https://github.com/saiaathish/picogent/pull/542) — split confinement from residual TOCTOU
- [#548](https://github.com/saiaathish/picogent/pull/548) — final release audit + operator checklist
- [#649](https://github.com/saiaathish/picogent/pull/649) / [#650](https://github.com/saiaathish/picogent/pull/650) — current boundary inventory
- [#651](https://github.com/saiaathish/picogent/pull/651) — evolution-store securefile hardening
- [#652](https://github.com/saiaathish/picogent/pull/652) — dead session helper cleanup
- Parents [#450](https://github.com/saiaathish/picogent/issues/450),
  [#453](https://github.com/saiaathish/picogent/issues/453) — remain **OPEN**
