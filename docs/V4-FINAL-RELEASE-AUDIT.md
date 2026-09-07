# V4 tip-bound final release + hostile residual audit

Status: **NOT COMPLETE**. This is a tip-bound evidence packet for operator
review. It does **not** authorize a release, does **not** claim
`release-authorization=PASS`, and does **not** close parents
[#450](https://github.com/saiaathish/picogent/issues/450) /
[#453](https://github.com/saiaathish/picogent/issues/453).

Operator decision checklist:
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).
Predicate contract:
[V4-RELEASE-AUTHORIZATION.md](V4-RELEASE-AUTHORIZATION.md).
Historical dry-runs remain in [V4-RELEASE-AUDIT.md](V4-RELEASE-AUDIT.md).

## Anchors (docs tip `70dba64` / behavior `423d047`)

| Anchor | Value |
| --- | --- |
| Docs / remote tip SHA (`origin/main`, merge of [#547](https://github.com/saiaathish/picogent/pull/547)) | `70dba64efe9f96bce27ec6c1d2fb6f9109e5de33` |
| Prior docs tip ([#546](https://github.com/saiaathish/picogent/pull/546)) | `9ac2675fcc8b9bb62e60ba0f4cd85542623530b5` |
| Behavior SHA (matrix / aggregate / live+rendered evidence) | `423d0471c1864651473c2f1828b67066885f79bd` ([#545](https://github.com/saiaathish/picogent/pull/545)) |
| Docs-only delta `423d047..70dba64` | `DOCS_ONLY_DESCENDANT` (10 files under `docs/` only) |
| Exact-SHA matrix summary | **PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1** |
| Exact-SHA matrix artifact digest (SHA-256 of retained JSON) | `48cbd34752aabfbd8dea385b7ca13e4c1c343baea1738d096a1b4c8a4c8af748` |
| Three-platform aggregate SHA-256 | `d5e55136fa1af5c00b37b22d6f06d1b31752e66abe027626c23243ecc3b4702f` |
| Hosted Windows collect (SUCCEEDED) | [34090755069](https://github.com/saiaathish/picogent/actions/runs/34090755069) |
| Windows zip SHA-256 | `4750b1ae1685aa1d208b65a55d46a0a6a947d2dc812942ef58a0bd0095d3f321` |
| Tip evidence root (operator-local, outside checkout) | `/private/tmp/picogent-evidence-423d047/` |
| Docs-tip retention root | `/private/tmp/picogent-evidence-docs-tip-70dba64/` |

Related landings that shape this packet:

- [#542](https://github.com/saiaathish/picogent/pull/542) — split bounded
  `hostile-parent-swap-confinement` from residual `hostile-filesystem-toctou`
- [#544](https://github.com/saiaathish/picogent/pull/544) — targeted-only
  coverage + operator-approval checklist in the release-auth predicate
- [#545](https://github.com/saiaathish/picogent/pull/545) — tip benchmark
  evidence refresh (honest regressions; no fake gains)
- [#546](https://github.com/saiaathish/picogent/pull/546) — tip Windows +
  three-platform aggregate PASS docs at behavior `423d047`
- [#547](https://github.com/saiaathish/picogent/pull/547) — historical
  `cddb184` three-platform PASS docs retained on tip

## Exact-SHA full matrix at behavior `423d047`

Authoritative retained artifact:

`/private/tmp/picogent-evidence-docs-tip-70dba64/runtime-boundary-matrix-exact-423d047.json`

| Field | Value |
| --- | --- |
| `candidate_sha` / `behavior_sha` | `423d0471c1864651473c2f1828b67066885f79bd` |
| `behavior_provenance` | `EXACT_HEAD` |
| Summary | **PASS 10 · INCONCLUSIVE 1 · UNVERIFIED 1** |

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

`/private/tmp/picogent-evidence-423d047/aggregate/rendered-cross-platform-evidence.json`

- SHA-256: `d5e55136fa1af5c00b37b22d6f06d1b31752e66abe027626c23243ecc3b4702f`
- `verdict=PASS` for darwin / linux / windows at exact candidate `423d047…`
- Platform records:
  [Darwin](V4-RENDERED-RECOVERY-EVIDENCE-423D047.md),
  [Linux](V4-RENDERED-LINUX-EVIDENCE-423D047.md),
  [Windows](V4-RENDERED-WINDOWS-EVIDENCE-423D047.md),
  [cross-platform](V4-RENDERED-CROSS-PLATFORM.md)

## Docs-tip retention honesty (`70dba64` + `--behavior-sha 423d047`)

| Mode | Summary | Notes |
| --- | --- | --- |
| Exact-SHA at behavior `423d047` | PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 | Authority for `rendered-cross-platform=PASS` |
| Docs tip `70dba64`, behavior `423d047`, no cross env | PASS 9 / INCONCLUSIVE 1 / UNVERIFIED 2 | Live + local rendered PASS; `rendered-cross-platform=UNVERIFIED` (exact-candidate claim deferred) |
| Docs tip `70dba64` with `423d047` aggregate as cross env | `rendered-cross-platform=FAIL` | Expected: aggregate `candidate_sha` must match matrix candidate; docs-only descendant does not project that PASS |

Prefer the exact-SHA matrix / aggregate as the tip full-matrix authority for
cross-platform PASS. Do not treat a docs-tip candidate SHA as carrying the
behavior-bound aggregate.

## What is proved (PASS)

Foundations at behavior `423d047` that this packet treats as proved for the
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
  ([34090755069](https://github.com/saiaathish/picogent/actions/runs/34090755069)).

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
consciously if authorization is later granted.

## Release-authorization dry-run (no operator approval)

| Input | Dry-run observation |
| --- | --- |
| Event / ref | Assumed `push` / `refs/heads/main` for tip audit only |
| Exact-SHA matrix (non-residual lanes) | PASS rows as above at behavior `423d047` |
| Residual TOCTOU | Remains `UNVERIFIED` (allowed residual; not a fake PASS) |
| Operator approval for scope `v4-release` | **Absent** |
| Predicate result | **`INCONCLUSIVE` / `authorized: false`** |

Closing tip `rendered-cross-platform` and landing [#546](https://github.com/saiaathish/picogent/pull/546) /
[#547](https://github.com/saiaathish/picogent/pull/547) do **not** authorize
release. Missing operator approval alone keeps the predicate unauthorized.

## Benchmark / outcome-quality honesty

Per [#545](https://github.com/saiaathish/picogent/pull/545) /
[V4-PERFORMANCE-CAMPAIGN.md](V4-PERFORMANCE-CAMPAIGN.md):

- Tip microbenchmark refreshes record **regressions** vs prior tip `38b45ff`
  (context manage, session metadata list, session load, warm scripted-edit).
- **No benchmark gains are proved.** Explicit: “No fake gains are claimed.”
- Outcome-quality comparison remains `inconclusive` (no comparable structured
  v3 telemetry for a product win claim).

Do **not** treat flat microbenchmark rows or missing v3 telemetry as a v4.0
performance exit criterion.

## Parents remain open

- [#450](https://github.com/saiaathish/picogent/issues/450) — **OPEN**
- [#453](https://github.com/saiaathish/picogent/issues/453) — **OPEN**

Keep both open until residual TOCTOU is closed or formally accepted and an
operator release decision is recorded. This packet does not merge, close, or
mark the goal complete.

## Overall packet verdict

**NOT COMPLETE / unauthorized.**

Proved tip-bound foundations at behavior `423d047` (including
`rendered-cross-platform=PASS`) sit under a clean docs tip `70dba64`. Residual
`hostile-filesystem-toctou=UNVERIFIED` and `release-authorization=INCONCLUSIVE`
(`authorized: false`) remain. Benchmark gains stay **UNPROVED**. Human
operator review is required before any authorization claim.
