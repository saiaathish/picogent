# v4 rendered recovery evidence at tip 18dc4a1

This record refreshes the direct BrowserOS neo
allow → undo → fresh-process reload observation against exact `origin/main` tip
`18dc4a1ca1137ab78dfd0102848eb99c417653cc`. It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim rendered behavior on Windows, live-provider recovery, hostile-filesystem
TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `18dc4a1ca1137ab78dfd0102848eb99c417653cc`.
- Fixture binary:
  `vcs.revision=18dc4a1ca1137ab78dfd0102848eb99c417653cc`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/post519-recovery`; task-owned page `402`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-07T02:02:59.761846Z`; reload started at
  `2026-09-07T02:04:24.515124Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The accepted fixture binary was built with `go build -buildvcs=true` from a
separate clean detached clone at the exact source SHA because linked-worktree
builds do not reliably expose Go VCS settings. Build metadata and both fixture
manifests independently recorded the exact SHA and clean source state.

## Direct rendered observations

1. The fresh seed page loaded in Safe mode without Undo; the probe was absent.
2. The task rendered Deny, This turn, Always allow, and Allow before mutation;
   the probe remained absent.
3. Allow rendered `Edited 1 file`, `Changed files (1)`, and
   `Undo last change`; the probe digest matched the fixture contract.
4. Undo removed the Undo control while retaining `Changed files (1)`; the probe
   was absent.
5. The same task-owned tab navigated to a fresh fixture process and retained
   `Changed files (1)` without stale Undo; the probe remained absent.

The direct observation verdict is `PASS`. A final-state screenshot was visually
inspected but not retained, so `screenshot_sha256` is `UNRECORDED`.

## Host-local artifacts

```text
/private/tmp/picogent-evidence-post519/rendered-recovery-observation.json
/private/tmp/picogent-evidence-post519/rendered-platform-evidence.json
/private/tmp/picogent-evidence-post519/rendered-seed-manifest.json
/private/tmp/picogent-evidence-post519/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     255162a9a87363e6895974fe79c452c81fe0b5260a84f758f28a705c984ff5ce
platform        41792849e31fd69b7b601da00fc04e3500faa7c56a12f3a22b055aabe8139b50
seed-manifest   5b4682967f3f3e181fb3fed73001e8a7327d659c30dbe02cf4fae503d697fd75
reload-manifest e224db607e42c80fa4027b3418bec9d6bc458e7b61a1c31b894438bd38985963
```

The local platform artifact records
`candidate_sha=18dc4a1ca1137ab78dfd0102848eb99c417653cc`,
`platform=darwin`, `architecture=arm64`, `browser=browseros-neo`,
`fixture=rendered-recovery`, `verdict=PASS`, and
`source_tree_modified=false`.

## Matrix result and boundaries

The combined exact-tip matrix recorded:

```text
HEAD=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
PASS: 8  INCONCLUSIVE: 1  UNVERIFIED: 2
rendered-platform-local=PASS
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Linux was separately refreshed at this same SHA, but Windows owned-browser
evidence does not exist. No three-platform aggregate was produced and
`rendered-cross-platform` remains `UNVERIFIED`.

At a later clean docs-only descendant, supply this artifact with
`--behavior-sha 18dc4a1ca1137ab78dfd0102848eb99c417653cc`. This does not
upgrade rendered cross-platform behavior, hostile TOCTOU, or release
authorization.
