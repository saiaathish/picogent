# v4 rendered recovery evidence at current docs tip 4854523

This record belongs to [#599](https://github.com/saiaathishkarthik/picogent/issues/599),
the focused current-platform child under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh task-owned BrowserOS neo observation of the real embedded GUI at exact
candidate SHA `48545233c0a453c419787a4ca8b06d21741cc31c`.

The observation is bounded to Darwin rendered recovery. It does not claim
Linux/Windows behavior, cross-platform rendered coverage, live-provider
quality, arbitrary hostile same-UID filesystem races, or release readiness.

## Provenance

- Source: clean temporary clone at
  `48545233c0a453c419787a4ca8b06d21741cc31c`.
- Fixture build: `vcs.revision=48545233c0a453c419787a4ca8b06d21741cc31c`,
  `vcs.modified=false`, `-tags rendered_fixture`.
- Runtime: `go-build-tags-rendered_fixture`.
- Host: `darwin/arm64`.
- Browser: task-owned BrowserOS neo, session `codex/rendered-macos`, page `60`
  (`ownership=mine`, title `Picogent`).
- Fixture session: `rendered-recovery-fixture`; implementation issue `#467`;
  parent issue `#453`.
- Seed started at `2026-09-10T04:37:24.146435Z`; reload started at
  `2026-09-10T04:40:05.211497Z`.
- Both fixture manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Fixture manifest applied-content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Disposable fixture home and manifests were kept under the macOS system temp
  directory and are not checkout or Desktop artifacts.
- BrowserOS returned an inline screenshot for the observation, but no durable
  screenshot path. Screenshot persistence is therefore `UNRECORDED`.

## Direct rendered observations

| Sequence | Direct UI/DOM observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page loaded in Safe mode with no enabled Undo control. | Seed manifest recorded the probe as absent before the turn. | `PASS` |
| 2 | Safe permission card rendered `Deny`, `This turn`, `Always allow`, and `Allow`; the permission text identified `rendered-recovery-probe.txt`. | Seed manifest recorded the probe as absent before the turn; no probe was observed in the pre-Allow UI state. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and enabled `Undo last change`. | Fixture manifest recorded the deterministic applied-content SHA; the independent on-disk hash was not retained before Undo. | `PASS` |
| 4 | Clicking `Undo last change` removed the Undo control and rendered `Undid last turn: removed rendered-recovery-probe.txt`. | Probe path was absent after Undo. | `PASS` |
| 5 | After stopping the seed process and loading the fresh reload process in the same task-owned page, durable history rendered `Changed files (1)` with no stale Undo control. | Probe remained absent after reload; reload manifest retained exact clean provenance. | `PASS` |

The `Changed files (1)` value after Undo and reload is retained task history; it
does not mean the undone path still exists.

## Exact-head matrix

The matrix was run from the clean exact-SHA clone with the retained local
Darwin platform record:

```text
HEAD=PASS  tree=CLEAN
PASS: 7  INCONCLUSIVE: 1  UNVERIFIED: 4
rendered-platform-local=PASS
rendered-cross-platform=UNVERIFIED
live-provider-connectivity=UNVERIFIED
live-provider-quality=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Digest-only retained artifacts outside the checkout:

```text
observation     e3dac9e678ab1b0eca700750dc3ae3bba73f94f477209fd39de85c78e6e2e315
platform        fcd7e33390c9dff0448eeaadae104f617da8f096264ebf16c6c6efb5be01eb59
matrix          7e35eef5b4bdaf7b490e2afaea54ae0331c469b806d5ebdb13153a7ecced42d9
screenshot      UNRECORDED
```

The local platform artifact uses schema
`picogent.v4.rendered-platform-evidence.v1`, exact candidate SHA, declared
`darwin/arm64`, `task-owned-disposable`, `browseros-neo`,
`rendered-recovery`, an observation digest, and an explicit clean-source
assertion. No DOM transcript, credentials, URL, or screenshot bytes are in the
retained platform record.

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Darwin.
- `PASS`: `rendered-platform-local` for `darwin/arm64` at candidate
  `48545233c0a453c419787a4ca8b06d21741cc31c`.
- `UNVERIFIED`: Linux, Windows, and the cross-platform rendered claim.
- `UNVERIFIED`: durable screenshot retention, live-provider claims, and broad
  same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
