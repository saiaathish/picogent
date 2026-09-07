# v4 rendered recovery evidence at tip 21153f3

This record refreshes the direct BrowserOS neo
allow → undo → fresh-process reload observation against exact `origin/main` tip
`21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`. It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim tip-bound rendered behavior on Windows, live-provider recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`.
- Fixture binary:
  `vcs.revision=21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/tip-recovery`; task-owned page `410`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-07T03:52:47.765352Z`; reload started at
  `2026-09-07T03:54:12.450413Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The accepted fixture binary was built with `go build -buildvcs=true` from a
separate clean detached clone at the exact source SHA because linked-worktree
builds do not reliably expose Go VCS settings. Build metadata and both fixture
manifests independently recorded the exact SHA and clean source state.

Non-docs tip `#524` broke `--behavior-sha` continuity from `18dc4a1…` for
live/local-rendered rows, so this observation was collected afresh at the
current tip.

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
/private/tmp/picogent-evidence-21153f3/rendered-recovery-observation.json
/private/tmp/picogent-evidence-21153f3/rendered-platform-evidence.json
/private/tmp/picogent-evidence-21153f3/rendered-seed-manifest.json
/private/tmp/picogent-evidence-21153f3/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     4ae07c9199d113da64ceccd195a288a294ead025a59a05d9b05ad146ed70b4d4
platform        6e666be60546da9fc3a886294f9844cdd23069955c8f68fb16f3085b4d9ef2e2
seed-manifest   725346d966211f0b31497d09bcdd7430f4d9326d7db16e310b1e11feb7d0f279
reload-manifest ee8ceee3e5651d6742094aec717e29cae076b4ca2a0010875b0805be7dfbe5a9
```

The local platform artifact records
`candidate_sha=21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`,
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

Linux was refreshed at this same tip SHA. The historical three-platform
aggregate `e519f22a…` remains exact-candidate bound to `18dc4a1…` and is not
reattached here; tip `rendered-cross-platform` stays `UNVERIFIED` until all
three platforms are recollected at one tip.

At a later clean docs-only descendant, supply this artifact with
`--behavior-sha 21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`. This does not
upgrade rendered cross-platform behavior, hostile TOCTOU, or release
authorization.
