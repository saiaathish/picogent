# V4 hostile TOCTOU residual acceptance record (operator stub)

Status: **unsigned stub** (re-bound from base `main` tip `0788a2b`). Fill this
record when consciously accepting
`hostile-filesystem-toctou=UNVERIFIED` as a residual audit boundary during
release eligibility. This file does **not** authorize a release by itself,
does **not** claim TOCTOU PASS, and does **not** close
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Canonical residual package:
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md).
Operator checklist:
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).

Prefer retaining a completed copy **outside** the checkout if the trusted
workflow requires an immutable out-of-tree approval artifact. Do not commit
secrets or credentials into this stub.

## Binding

| Field | Value |
| --- | --- |
| Residual claim | `hostile-filesystem-toctou` |
| Residual verdict (must remain) | `UNVERIFIED` |
| Bounded confinement proved | `hostile-parent-swap-confinement=PASS` via [#542](https://github.com/saiaathish/picogent/pull/542) |
| Final-release / checklist packet | [#548](https://github.com/saiaathish/picogent/pull/548) |
| Latest retained candidate / behavior SHA | `592b07a633d9683354c4abeee09b767bc071ab35` |
| Base `main` tip before this docs rebind | `0788a2b74155d5c46af37c5f7c19c5a3d5c22ef9` |
| Exact-SHA matrix digest (optional) | `71bfe03c0afb889c9a548e9264cf0ab4509d80f69a85e346fb4501d85a626b05` aggregate; `6048c4244818db694b96b5875f8893f1d84aaf731a4caebc4493bfbff4213817` runtime |
| Record date (UTC) | `_fill_` |

The retained candidate predates the post-candidate code changes in #651. A
signer must replace it with a fresh exact-current candidate before treating
this record as release-bound; this stub does not imply that refresh occurred.

## Decision

| Field | Value |
| --- | --- |
| Approved residual | `_yes / no_` |
| Approver name | `_fill_` |
| Approver role / authority basis | `_fill_` |
| Scope | `v4-release` residual acceptance only |

## Affirmations (operator initials)

- [ ] Parent-swap confinement PASS is **bounded** and does **not** upgrade TOCTOU to PASS. Initials: `_ _`
- [ ] Predicate non-blocking residual rule is **eligibility**, not TOCTOU closure. Initials: `_ _`
- [ ] Matrix docs / trackers will **not** be rewritten to claim TOCTOU PASS. Initials: `_ _`
- [ ] [#450](https://github.com/saiaathish/picogent/issues/450) / [#453](https://github.com/saiaathish/picogent/issues/453) stay **open** until residuals accepted **and** release decision recorded. Initials: `_ _`
- [ ] This acceptance is **not** publish/tag/deploy and **not** UpdateGoal complete. Initials: `_ _`

## Signed-style attestation

```text
Approved residual: ________
Approver name: ________
Date (UTC): ________
Candidate / behavior SHA: ________
Docs tip SHA: ________
Signature / attestation mark: ________
```

Unsigned or incomplete stubs must be treated as **no residual acceptance**.
