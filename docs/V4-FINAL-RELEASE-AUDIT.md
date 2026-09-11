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
The prior retained live/rendered packets at audited main candidate `8b022e8`:
[V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md](V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md)
for live-provider continuity from behavior `b7b5c5c`, and
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md)
for exact-candidate rendered behavior. These packets retain separate
provenance and must not be unioned into a synthetic release-authorizing
matrix. The underlying behavior observation remains documented in
[V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).
The earlier `0e2b156` → `ad34bd5` documentation-only continuity and rendered
packet remain historical.
The prior `f901e8f` packet is retained as historical evidence only:
[V4-TIP-EVIDENCE-F901E8F.md](V4-TIP-EVIDENCE-F901E8F.md).
The prior `492595f` packet is retained as historical evidence only:
[V4-TIP-EVIDENCE-492595F.md](V4-TIP-EVIDENCE-492595F.md).
The prior `1531850` packet is retained as historical evidence only:
[V4-TIP-EVIDENCE-1531850.md](V4-TIP-EVIDENCE-1531850.md).
The prior `97a3ec5` packet is retained as historical evidence only:
[V4-TIP-EVIDENCE-97A3EC5.md](V4-TIP-EVIDENCE-97A3EC5.md).
The older `3f5543d` packet is also retained as historical evidence only:
[V4-TIP-EVIDENCE-3F5543D.md](V4-TIP-EVIDENCE-3F5543D.md).
The older `1ce2059` packet is also retained as historical evidence only:
[V4-TIP-EVIDENCE-1CE2059.md](V4-TIP-EVIDENCE-1CE2059.md).
The older `f8a78c6` packet is also retained as historical evidence only:
[V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md).

## Current main posture (behavior candidate `1865a4c`)

The exact clean behavior candidate for this audit packet is
`1865a4c00356ac9b871b35b3a08f4e983f2af0e1`. The last non-documentation
behavior candidate is `eccebd293e58ec48c6553e9228ff4b201a74c77a`; its bounded
goal-state evidence is retained in
[V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md) and remains valid
through documentation-only descendants.

Fresh exact-head connectivity and fixed no-tool quality observations are
retained in
[V4-LIVE-PROVIDER-QUALITY-EVIDENCE-1865A4C.md](V4-LIVE-PROVIDER-QUALITY-EVIDENCE-1865A4C.md).
Fresh task-owned Darwin rendered recovery evidence is retained in
[V4-RENDERED-RECOVERY-EVIDENCE-1865A4C.md](V4-RENDERED-RECOVERY-EVIDENCE-1865A4C.md).
The exact-head matrix records `live-provider-connectivity=PASS` and
`live-provider-quality=PASS` and `rendered-platform-local=PASS`, while
`rendered-cross-platform=UNVERIFIED`, and broad
`hostile-filesystem-toctou=UNVERIFIED` remain explicit gaps.
`release-authorization` remains `INCONCLUSIVE` with `authorized=false` because
no human operator decision has been recorded. The bounded goal-state and live
provider PASS rows do not upgrade any broader rendered, hostile, or release
row, and the historical live/rendered packets below are not unioned into a
current release-authorizing matrix. The current docs-tip continuity matrix
records `PASS 9 / INCONCLUSIVE 1 / UNVERIFIED 2`; its local rendered PASS is
Darwin-only and does not upgrade the cross-platform row.

## Historical main live-provider continuity (`8b022e8`, behavior `b7b5c5c`)

The audited main candidate is
`8b022e82be4b2e8ade0a502c5f373eaf6dc61160`. The retained continuity matrix
`/private/tmp/picogent-live-619.R6pSMq/runtime-boundary-matrix-8b022e8.json`
has digest
`3260157d65e09aa43669bc565f76b0912a782fc7a0174fd3e28c3f0b96237ebd` and
records **PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3** with
`behavior_provenance=DOCS_ONLY_DESCENDANT`, `head_match=PASS`, and
`tree=CLEAN`.

The continuity packet is documented in
[V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md](V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md).
The live-provider connectivity and fixed quality rows are `PASS`; rendered
platform rows remain `UNVERIFIED` in this live-only matrix. The broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`, and
`release-authorization` remains `INCONCLUSIVE` because no human operator
decision has been recorded. This is a retained-artifact continuity check, not
a new provider session and not a rebind of the rendered observation.

## Historical exact-head rendered checkpoint (`8b022e8`)

The exact clean rendered candidate is
`8b022e82be4b2e8ade0a502c5f373eaf6dc61160`. The retained three-platform
aggregate
`/private/tmp/picogent-rendered-current-8b022e8/aggregate/rendered-cross-platform-evidence.json`
has SHA-256
`47f62799caa8dd11b655adff287a5ab1b29c4835b8e777832eb7fae2079bca04` and
records Darwin, Linux, and Windows as `PASS`. Its exact-candidate runtime
matrix at
`/private/tmp/picogent-rendered-current-8b022e8/aggregate/runtime-boundary-matrix-rendered.json`
has digest
`b1a735b0a377d2b519e6306fb11308ef10498a5d612680e4a858d4c4ad772ae2` and
records **PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3** with
`rendered-platform-local=PASS` and `rendered-cross-platform=PASS`.

The exact rendered packet is documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md).
Its independent live-provider rows remain `UNVERIFIED`, as does the broad
`hostile-filesystem-toctou` residual; `release-authorization` remains
`INCONCLUSIVE`. The rendered and live PASS rows are recorded in separate
matrices because their retained artifacts have different behavior-SHA
provenance. They must not be combined to authorize a release.

## Historical live-provider checkpoint (`0e2b156`)

The retained clean behavior candidate for live-provider evidence is
`0e2b15624b29c256c4b22364ecf6970890c90374`. The exact-head retained matrix
`/private/tmp/picogent-live-0e2b156/runtime-boundary-matrix.json` has digest
`824e1072b4d7fc9acf9ef00b683b376632febf8556c1ded2282eb486510fe0a9` and
records **PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3**.

The live-provider packet is documented in
[V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md](V4-LIVE-PROVIDER-EVIDENCE-0E2B156.md).
The retained collector proves `EXACT_HEAD`, `head_match=PASS`, and
`tree=CLEAN`. The live-provider connectivity and fixed quality rows are
`PASS`; rendered-platform-local and rendered-cross-platform remain
`UNVERIFIED`. The broad `hostile-filesystem-toctou` residual remains
`UNVERIFIED`, and `release-authorization` remains `INCONCLUSIVE` because no
human operator decision has been recorded. The adaptive-depth quality result
is benchmark-only and does not change this matrix. This checkpoint is an
ancestor of the rendered `ad34bd5` checkpoint through documentation-only
changes; that continuity retains this live-provider record but does not upgrade
the rendered rows in this matrix.

The live-provider packet also has a current-main continuity refresh at docs tip
`2dd8e1bb6b395d7b14bca4d45bf2b4fc53f93fb0`, retained at
`/private/tmp/picogent-continuity-main-2dd8e1b/runtime-boundary-matrix.json`
with digest
`764a940da63d5003be6a6f3d302752ffbaa230010ab9f23a859a26cf60e94235`. It
records `DOCS_ONLY_DESCENDANT`, `HEAD=PASS`, `tree=CLEAN`, and the same PASS 8 /
INCONCLUSIVE 1 / UNVERIFIED 3 live-provider-only posture. It is a continuity
refresh, not a second rendered observation and not a synthetic release matrix.

## Historical exact rendered checkpoint (`ad34bd5`)

The exact clean rendered candidate is
`ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`. The retained three-platform
aggregate
`/private/tmp/picogent-rendered-aggregate-ad34/rendered-cross-platform-evidence.json`
has SHA-256
`f3def9dc069980125752abbe3d22b7c2ad6a3af8ad74d52091afc38974e35630` and
records Darwin, Linux, and Windows as `PASS`. Its exact-candidate runtime
matrix at
`/private/tmp/picogent-rendered-aggregate-ad34/runtime-boundary-matrix.json`
has digest
`0edb2195f34cd53af4aa4574f82f1f3cd864b319361d427a1d179df6b5c1f952` and
records **PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4** with
`rendered-cross-platform=PASS`.

The exact rendered packet is documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-AD34BD5.md).
Its independent live-provider connectivity and quality rows remain
`UNVERIFIED`, as do the broad `hostile-filesystem-toctou` residual and
`release-authorization` (`INCONCLUSIVE`). The three platform PASS rows cannot
be combined with the historical live-provider PASS rows from `b7b5c5c` to
authorize a release, because the release predicate requires one exact-SHA
matrix.

## Historical exact behavior-evidence checkpoint (`f901e8f`)

The prior clean behavior candidate was
`f901e8fa1a7de08ef8c5f4f9db5246b7b7d64a25`. Its exact-head retained matrix
`/private/tmp/picogent-evidence-f901e8f/runtime-boundary-matrix.json` has
digest
`6e7cc3a996bc69b7cdcab7d7d735c2f6d359bf1a8920d112aafab59e4a877774` and
records **PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5**.

The historical exact-tip packet is documented in
[V4-TIP-EVIDENCE-F901E8F.md](V4-TIP-EVIDENCE-F901E8F.md). No live-provider or
rendered-platform artifacts were supplied for that candidate, so those rows
remain `UNVERIFIED`.

## Historical exact behavior-evidence checkpoint (`492595f`)

The latest clean behavior candidate is
`492595fb69bb02a91665cb34998a9d7c486ead72`. The exact-head retained matrix
`/private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-492595f.json`
has digest
`a59a2d485aff8156fa31cc99f70ec8f260562f238999fd1e96b15402f94acbfa` and
records **PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5**.

The exact-tip packet is documented in
[V4-TIP-EVIDENCE-492595F.md](V4-TIP-EVIDENCE-492595F.md). No current-tip
live-provider or rendered-platform artifacts were supplied, so those rows are
`UNVERIFIED`. The broad `hostile-filesystem-toctou` residual remains
`UNVERIFIED`, and `release-authorization` remains `INCONCLUSIVE` because no
human operator decision has been recorded.

PR #581's post-merge continuity check correctly rejected rebinding behavior
`97a3ec5` because that PR touched `README.md`; this repository contract permits
continuity only across intervening `docs/` paths. The fresh `1531850` exact-head
collection was the prior packet. The fresh `492595f` exact-head collection after
docs-only PR #583 was authoritative for that historical checkpoint.

## Historical exact behavior-evidence checkpoint (`1531850`)

The prior clean behavior candidate was
`1531850d80485e441bae58369595cb15a57e4c99`. Its exact-head retained matrix
`/private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-1531850.json`
had digest
`d261af32e1d547b9c97243f4d4b3667c4ba6a99981770ff80ea4dd0915c3957c` and
recorded **PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5**.

The historical packet is
[V4-TIP-EVIDENCE-1531850.md](V4-TIP-EVIDENCE-1531850.md). Its live-provider,
rendered-platform, broad hostile-filesystem-TOCTOU, and release-authorization
boundaries remain fail-closed and are not projected onto another candidate.

## Historical exact behavior-evidence checkpoint (`97a3ec5`)

The latest clean behavior candidate is
`97a3ec5951137a353fcb42276c701040e1e716c1`. The exact-head retained matrix
`/private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-97a3ec5.json`
has digest
`f244d37976965874d25a9465c071cdc6ff63caf26a411182ccf339bd003dddc3` and
records **PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5**.

The exact-tip packet is documented in
[V4-TIP-EVIDENCE-97A3EC5.md](V4-TIP-EVIDENCE-97A3EC5.md). No current-tip
live-provider or rendered-platform artifacts were supplied, so those rows are
`UNVERIFIED`. The broad `hostile-filesystem-toctou` residual remains
`UNVERIFIED`, and `release-authorization` remains `INCONCLUSIVE` because no
human operator decision has been recorded.

The `3f5543d` and `1ce2059` live-provider and rendered-browser observations are
not rebound: PR #578 and PR #579 changed non-documentation code after those
behavior candidates. The current packet records the new tip's fail-closed
state without inventing replacement runtime observations.

## Historical exact behavior-evidence checkpoint (`3f5543d`)

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

The retained evidence is split across two exact behavior candidates. At
`0e2b156`, the live-provider matrix records PASS 8 / INCONCLUSIVE 1 /
UNVERIFIED 3 with live-provider connectivity and fixed quality `PASS`, while
rendered rows remain `UNVERIFIED`. At `ad34bd5`, the rendered matrix records
PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4 with
`rendered-cross-platform=PASS`, while live-provider rows remain `UNVERIFIED`.
The documentation-only continuity between them preserves provenance but does
not create a synthetic union. The residual
`hostile-filesystem-toctou=UNVERIFIED` and
`release-authorization=INCONCLUSIVE` (`authorized: false`) remain. The older
`f901e8f`, `492595f`, `1531850`, `97a3ec5`, `3f5543d`, `1ce2059`, and
`f8a78c6` anchors above are historical and are not projected onto either
current packet. Human operator review is required before any authorization
claim.
