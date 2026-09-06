# v4 rendered recovery evidence at tip 28cc214

This record refreshes the direct BrowserOS neo allow→undo→fresh-process reload
observation against exact `origin/main` tip
`28cc214834f4d46dff555b2e60c83cd526827b31`. It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim rendered behavior on Linux or Windows, live-provider recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `28cc214834f4d46dff555b2e60c83cd526827b31`.
- Fixture binary: `vcs.revision=28cc214834f4d46dff555b2e60c83cd526827b31`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/rendered-tip-recovery`; task-owned page `360`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-06T13:34:56.309694Z`; reload started at
  `2026-09-06T13:35:54.114057Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The linked worktree build did not expose Go VCS settings, so it was rejected
for rendered provenance. The accepted fixture binary was built with
`go build -buildvcs=true -tags rendered_fixture` from a separate clean,
detached clone at the same exact SHA. Its build metadata and both fixture
manifests independently recorded the exact SHA and clean source state.

## Direct rendered observations

| Sequence | Direct UI observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page loaded in Safe mode without Undo. | Probe absent. | `PASS` |
| 2 | The task rendered Deny / This turn / Always allow / Allow before mutation. | Probe remained absent. | `PASS` |
| 3 | Allow rendered `Edited 1 file`, `Changed files (1)`, and `Undo last change`. | Probe digest matched the fixture contract. | `PASS` |
| 4 | Undo removed the Undo control while retaining `Changed files (1)`. | Probe absent after undo. | `PASS` |
| 5 | The same tab navigated to a fresh fixture process and retained `Changed files (1)` without stale Undo. | Probe remained absent after reload. | `PASS` |

Screenshots were not retained, so `screenshot_sha256` is `UNRECORDED`.

## Host-local artifacts

```text
/private/tmp/picogent-evidence-28cc214/rendered-recovery-observation.json
/private/tmp/picogent-evidence-28cc214/rendered-platform-evidence.json
/private/tmp/picogent-evidence-28cc214/rendered-seed-manifest.json
/private/tmp/picogent-evidence-28cc214/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     126525ed98a878d284eb64e5d5e040a31ff7a039be60d3cc09f1452547706e48
platform        a53ef4025c899b07cde489e074fb332b396562b7fa5f515465abd0cae4dfb779
seed-manifest   09990b9b1d6b612d79c354063c747d3b0417bb11f1c04ccfd4917e9371e16021
reload-manifest fe3c0493d6542088da1186dd70c7669d62f416c5d275ff68820c47d2e5099cee
```

The local platform artifact recorded:

```text
candidate_sha=28cc214834f4d46dff555b2e60c83cd526827b31
platform=darwin
architecture=arm64
browser=browseros-neo
fixture=rendered-recovery
observation_sha256=126525ed98a878d284eb64e5d5e040a31ff7a039be60d3cc09f1452547706e48
verdict=PASS
source_tree_modified=false
```

## Matrix result

The combined tip-bound matrix recorded:

```text
HEAD=PASS  tree=CLEAN
PASS: 8  INCONCLUSIVE: 1  UNVERIFIED: 2
rendered-platform-local=PASS
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Linux and Windows owned-browser artifacts do not exist at this SHA, so
`rendered-cross-platform` remains `UNVERIFIED`.
