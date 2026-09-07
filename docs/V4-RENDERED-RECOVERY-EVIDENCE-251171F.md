# v4 rendered recovery evidence at tip 251171f

This record refreshes the direct BrowserOS neo
allow → undo → fresh-process reload observation against exact behavior tip
`251171f2972cc3a28b03cf35c222e77134cb65b4` (#527 merge). It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim tip-bound rendered behavior on Windows, live-provider recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `251171f2972cc3a28b03cf35c222e77134cb65b4`.
- Fixture binary:
  `vcs.revision=251171f2972cc3a28b03cf35c222e77134cb65b4`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/tip-rebind`; task-owned page `412`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-07T04:11:16.044305Z`; reload started at
  `2026-09-07T04:13:14.074959Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The accepted fixture binary was built with `go build -buildvcs=true` from a
separate clean detached clone at the exact source SHA because linked-worktree
builds do not reliably expose Go VCS settings. Build metadata and both fixture
manifests independently recorded the exact SHA and clean source state.

Non-docs `#527` invalidated `--behavior-sha` retention from the earlier
`21153f3…` observations documented in #531, so this observation was collected
afresh at `251171f…`.

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
/private/tmp/picogent-evidence-251171f/rendered-recovery-observation.json
/private/tmp/picogent-evidence-251171f/rendered-platform-evidence.json
/private/tmp/picogent-evidence-251171f/rendered-seed-manifest.json
/private/tmp/picogent-evidence-251171f/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     415425a4858006aa7d520930bb3c8157b9320304ebb3724d9684b05153eaf763
platform        ea53f7d83cc26e4e6ae01b922f70a9e15bb6547a71eea9f44b9fb3550c983ad3
seed-manifest   4d2ae7a9c4e49a7bbf849a5406f514309fee4f16eba66a4169515dae77631e96
reload-manifest a8230e54b109eb334b2a1d6525b0c594dbe6aabee8998073cd904daa04665a30
```

The local platform artifact records
`candidate_sha=251171f2972cc3a28b03cf35c222e77134cb65b4`,
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
`--behavior-sha 251171f2972cc3a28b03cf35c222e77134cb65b4`. This does not
upgrade rendered cross-platform behavior, hostile TOCTOU, or release
authorization.
