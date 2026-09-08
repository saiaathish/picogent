# V4 tip-bound final release + hostile residual audit

Status: **NOT COMPLETE**. This is a tip-bound evidence packet for operator
review. It does **not** authorize a release, does **not** claim
`release-authorization=PASS`, and does **not** close parents
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Operator decision checklist:
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).
Formal TOCTOU residual-acceptance package:
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md)
([acceptance record stub](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md)).
Predicate contract:
[V4-RELEASE-AUTHORIZATION.md](V4-RELEASE-AUTHORIZATION.md).
Historical dry-runs remain in [V4-RELEASE-AUDIT.md](V4-RELEASE-AUDIT.md).
Current exact-tip evidence packet:
[V4-TIP-EVIDENCE-3F5543D.md](V4-TIP-EVIDENCE-3F5543D.md).
The prior `1ce2059` packet is retained as historical evidence only:
[V4-TIP-EVIDENCE-1CE2059.md](V4-TIP-EVIDENCE-1CE2059.md).
The older `f8a78c6` packet is also retained as historical evidence only:
[V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md).

## Current exact behavior-evidence checkpoint (`3f5543d`)

The latest clean behavior candidate is
`3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe`. The exact-head retained matrix
`/private/tmp/picogent-rendered-cross-platform-3f-matrix-local.jGT2s2/exact-candidate-runtime-boundary-matrix.json`
has digest
`2926c24e7c818290ab57c1bfa6caf84a6d11d4a537f66c7e28c3cdc157a84e87` and
records **PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3**.

The exact-tip packet is documented in
[V4-TIP-EVIDENCE-3F5543D.md](V4-TIP-EVIDENCE-3F5543D.md). The matching
Darwin/Linux/Windows owned-browser aggregate is documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3F5543D.md)
and has digest
`be790e046ee209b74531dfbfc7c71a6604f6b6401c214a951f2c54932111f7e5`.
Rendered local and cross-platform rows are `PASS` for this exact fixture and
candidate. Exact-tip live-provider rows remain `UNVERIFIED`; the broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`; and
`release-authorization` remains `INCONCLUSIVE` because no human operator
decision has been recorded.

The prior `1ce2059` live-provider and rendered-browser observations are not
rebound: PR #570 changed non-docs release-consumer code after that behavior
candidate. The fresh `3f5543d` rendered collection closes the named #507
checkpoint without projecting older evidence.

## Historical exact-tip rebinding (`1ce2059`)

The latest clean behavior candidate is
`1ce2059fc352b5d32b5da3adbaeb1ecb48834db7`. The exact-head retained matrix
`/private/tmp/picogent-evidence-1ce2059/runtime-boundary-matrix-full.json`
has digest
`c0e56a26407c19828a9e1a76e6d8a8e229cdaf6ffc1c7b0b7056ba52ea7ed6c7` and
records **PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1**.

The matching live-provider records are documented in
[V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md](V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md);
the matching Darwin/Linux/Windows rendered aggregate is documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md).
The broad `hostile-filesystem-toctou` claim remains `UNVERIFIED`, and
`release-authorization` remains `INCONCLUSIVE` because no human operator
decision has been recorded.

## Historical anchors (docs tip `4d0c507` / behavior `f8a78c6`)

| Anchor | Value |
| --- | --- |
| Docs / remote tip SHA (`origin/main`, merge of [#553](https://github.com/saiaathish/picogent/pull/553)) | `4d0c5074e5fa3a2ca473ea0efb6d9b951970799b` |
| Prior docs tip ([#552](https://github.com/saiaathish/picogent/pull/552)) | `71c31cadf0538610bd1637621a028f9b4cbb4bda` |
| Behavior SHA (matrix / aggregate / live+rendered evidence) | `f8a78c646877164009953401772acdb4d346175d` ([#551](https://github.com/saiaathish/picogent/pull/551)) |
| Docs-only lineage `f8a78c6` â†’ `71c31ca` â†’ `4d0c507` | `DOCS_ONLY_DESCENDANT` (docs/ only) |
| Exact-SHA matrix summary | **PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1** |
| Exact-SHA matrix artifact digest (SHA-256 of retained JSON) | `f0b4bca1ac62â€¦` (`aggregate/runtime-boundary-matrix-full.json`) |
| Three-platform aggregate SHA-256 | `5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd` |
| Hosted Windows collect (SUCCEEDED) | [34112789982](https://github.com/saiaathish/picogent/actions/runs/34112789982) |
| Tip evidence root (operator-local, outside checkout) | `/private/tmp/picogent-evidence-f8a78c6/` |

Historical (do **not** treat as current tip): docs tip `70dba64` /
behavior `423d047` and `/private/tmp/picogent-evidence-docs-tip-70dba64/`.

Related landings that shape this packet:

- [#542](https://github.com/saiaathish/picogent/pull/542) â€” split bounded
  `hostile-parent-swap-confinement` from residual `hostile-filesystem-toctou`
- [#544](https://github.com/saiaathish/picogent/pull/544) â€” targeted-only
  coverage + operator-approval checklist in the release-auth predicate
- [#549](https://github.com/saiaathish/picogent/pull/549) â€” formal TOCTOU
  residual-acceptance package + unsigned stub
- [#551](https://github.com/saiaathish/picogent/pull/551) â€” behavior tip
  alloc cuts (ValueAwareWindow / ListMeta); invalidates prior behavior-SHA
  retention from `423d047`
- [#552](https://github.com/saiaathish/picogent/pull/552) / [#553](https://github.com/saiaathish/picogent/pull/553)
  â€” docs-only tip packaging of cross-platform + live-provider PASS at
  behavior `f8a78c6`

## Exact-SHA full matrix at behavior `f8a78c6`

Authoritative retained artifact:

`/private/tmp/picogent-evidence-f8a78c6/aggregate/runtime-boundary-matrix-full.json`

| Field | Value |
| --- | --- |
| `candidate_sha` / `behavior_sha` | `f8a78c646877164009953401772acdb4d346175d` |
| `behavior_provenance` | `EXACT_HEAD` |
| Summary | **PASS 10 Â· INCONCLUSIVE 1 Â· UNVERIFIED 1** |

| Claim | Verdict |
| --- | --- |
| `hostile-child-env-sanitization` | `PASS` |
| `hostile-filesystem-deterministic` | `PASS` |
| `hostile-parent-swap-confinement` | `PASS` |
| `hostile-filesystem-toctou` | `UNVERIFIED` |
| `live-provider-connectivity` | `PASS` |
| `live-provider-quality` | `PASS` |
| `rendered-platform-local` | `PASS` |
| `rendered-cross-platform` | `PASS` |
| `rendered-recovery-undo-reload` | `PASS` |
| `rendered-long-horizon-local` | `PASS` |
| `restart-steer-undo-recovery` | `PASS` |
| `release-authorization` | `INCONCLUSIVE` |

Aggregate evidence for `rendered-cross-platform=PASS`:

`/private/tmp/picogent-evidence-f8a78c6/aggregate/rendered-cross-platform-evidence.json`

- SHA-256: `5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd`
- `verdict=PASS` for darwin / linux / windows at exact candidate `f8a78c6â€¦`
- Platform records:
  [Darwin](V4-RENDERED-RECOVERY-EVIDENCE-F8A78C6.md),
  [Linux](V4-RENDERED-LINUX-EVIDENCE-F8A78C6.md),
  [Windows](V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md),
  [cross-platform](V4-RENDERED-CROSS-PLATFORM.md),
  [live](V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md)

## Docs-tip retention honesty (`4d0c507` + `--behavior-sha f8a78c6`)

| Mode | Summary | Notes |
| --- | --- | --- |
| Exact-SHA at behavior `f8a78c6` | PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 | Authority for `rendered-cross-platform=PASS` |
| Docs tip `4d0c507`, behavior `f8a78c6`, no cross env | PASS 9 / INCONCLUSIVE 1 / UNVERIFIED 2 | Live + local rendered PASS; `rendered-cross-platform=UNVERIFIED` (exact-candidate claim deferred) |
| Docs tip `4d0c507` with `f8a78c6` aggregate as cross env | `rendered-cross-platform=FAIL` | Expected: aggregate `candidate_sha` must match matrix candidate; docs-only descendant does not project that PASS |

Prefer the exact-SHA matrix / aggregate as the tip full-matrix authority for
cross-platform PASS. Do not treat a docs-tip candidate SHA as carrying the
behavior-bound aggregate.

## What is proved (PASS)

Foundations at behavior `f8a78c6` that this packet treats as proved for the
non-residual matrix lanes:

- Local-first Safe/Fast permission gate, checkpoint/undo, criterion-bound
  verification, GUI/TUI/headless outcome contracts (scorecard + tip docs).
- Live connectivity + bounded fixed no-tool quality (`PASS`).
- Local + three-platform rendered recovery aggregate (`rendered-cross-platform=PASS`).
- Deterministic restart / steer / undo / recovery (`PASS`).
- Hostile deterministic controls + child-env sanitization (`PASS`).
- Bounded same-UID parent-swap confinement on Darwin + Linux (`PASS` via
  [#542](https://github.com/saiaathish/picogent/pull/542)).
- Hosted Windows owned-browser collect SUCCEEDED
  ([34112789982](https://github.com/saiaathish/picogent/actions/runs/34112789982)).
- `#551` alloc gains **PROVED** (ValueAwareWindow / ListMeta); broader
  outcome-quality gains remain UNPROVED.

## Hostile residual (explicitly open)

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `hostile-parent-swap-confinement` | `PASS` | Bounded Darwin + Linux same-UID parent-swap confinement only ([Darwin](V4-HOSTILE-PARENT-SWAP-DARWIN.md), [Linux](V4-HOSTILE-PARENT-SWAP-LINUX.md)). |
| `hostile-filesystem-deterministic` | `PASS` | Named securefile / procenv / workspace families ([hostile evidence](V4-HOSTILE-RUNTIME-EVIDENCE.md)). |
| `hostile-filesystem-toctou` | **`UNVERIFIED`** | Broader arbitrary same-UID pathname race claim remains deliberately unproved post-[#542](https://github.com/saiaathish/picogent/pull/542). Do **not** fabricate PASS. |

The release-authorization predicate may treat residual TOCTOU as an explicit
audit boundary rather than a hard universal PASS gate
([V4-RELEASE-AUTHORIZATION.md](V4-RELEASE-AUTHORIZATION.md)). That does **not**
make TOCTOU PASS, and operator acceptance of the residual must be recorded
consciously if authorization is later granted. Use the formal package
[V4-HOSTILE-TOCTOU-RESIDUAL.md](V4-HOSTILE-TOCTOU-RESIDUAL.md) and fillable
stub [V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md](V4-HOSTILE-TOCTOU-RESIDUAL-ACCEPTANCE.md)
for that record.

## Release-authorization dry-run (no operator approval)

| Input | Dry-run observation |
| --- | --- |
| Event / ref | Assumed `push` / `refs/heads/main` for tip audit only |
| Exact-SHA matrix (non-residual lanes) | PASS rows as above at behavior `f8a78c6` |
| Residual TOCTOU | Remains `UNVERIFIED` (allowed residual; not a fake PASS) |
| Operator approval for scope `v4-release` | **Absent** |
| Predicate result | **`INCONCLUSIVE` / `authorized: false`** |

Closing tip `rendered-cross-platform` and landing [#552](https://github.com/saiaathish/picogent/pull/552) /
[#553](https://github.com/saiaathish/picogent/pull/553) do **not** authorize
release. Missing operator approval alone keeps the predicate unauthorized.

## Benchmark / outcome-quality honesty

Per [#551](https://github.com/saiaathish/picogent/pull/551) /
[V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md):

- Alloc columns: ValueAwareWindow / ListMeta cuts are **PROVED** at behavior
  `f8a78c6`.
- Broader outcome-quality comparison remains open / inconclusive where v3
  telemetry is missing.
- Do **not** invent fake microbenchmark gains beyond the proved alloc rows.

## Parents remain open

- [#450](https://github.com/saiaathish/picogent/issues/450) â€” **OPEN**
- [#453](https://github.com/saiaathish/picogent/issues/453) â€” **OPEN**

Keep both open until residual TOCTOU is closed or formally accepted and an
operator release decision is recorded. This packet does not merge, close, or
mark the goal complete.

## Overall packet verdict

**NOT COMPLETE / unauthorized.**

The current behavior-evidence packet at `3f5543d` records exact-tip PASS 8 /
INCONCLUSIVE 1 / UNVERIFIED 3. Its rendered local and owned-browser
cross-platform rows are `PASS` for the named fixture; live-provider rows remain
`UNVERIFIED`, the residual `hostile-filesystem-toctou=UNVERIFIED`, and
`release-authorization=INCONCLUSIVE` (`authorized: false`) remain. The older
`1ce2059` and `f8a78c6` anchors above are historical and are not projected onto
the current candidate. Human operator review is required before any
authorization claim.
