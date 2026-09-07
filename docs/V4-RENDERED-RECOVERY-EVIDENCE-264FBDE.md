# v4 rendered recovery evidence at tip 264fbde

This record refreshes the direct BrowserOS neo allow→undo→fresh-process reload
observation against exact `origin/main` tip
`264fbde2b45609e6e85e175efe216e8f63509ab8`. It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim rendered behavior on Linux or Windows, live-provider recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `264fbde2b45609e6e85e175efe216e8f63509ab8`.
- Fixture binary:
  `vcs.revision=264fbde2b45609e6e85e175efe216e8f63509ab8`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/tip-recovery-evidence`; task-owned page `377`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-07T00:05:27.499915Z`; reload started at
  `2026-09-07T00:06:20.990197Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The accepted fixture binary was built with `go build -buildvcs=true` from a
separate clean detached clone at the exact source SHA because linked-worktree
builds do not reliably expose Go VCS settings. The build metadata and both
fixture manifests independently recorded the exact SHA and clean source state.

## Collection command pattern

```sh
git clone --no-tags https://github.com/saiaathish/picogent.git \
  /private/tmp/picogent-rendered-source-264fbde
git -C /private/tmp/picogent-rendered-source-264fbde \
  checkout --detach 264fbde2b45609e6e85e175efe216e8f63509ab8

cd /private/tmp/picogent-rendered-source-264fbde
go build -buildvcs=true -tags rendered_fixture \
  -o /private/tmp/picogent-evidence-264fbde/picogent-rendered-fixture-stamped \
  ./cmd/picogent-rendered-fixture

PICOGENT_RENDERED_FIXTURE_SOURCE_SHA=264fbde2b45609e6e85e175efe216e8f63509ab8 \
  /private/tmp/picogent-evidence-264fbde/picogent-rendered-fixture-stamped \
  -home "$disposable_home" -manifest "$disposable_home/seed-manifest.json" \
  -addr 127.0.0.1:0

PICOGENT_RENDERED_FIXTURE_SOURCE_SHA=264fbde2b45609e6e85e175efe216e8f63509ab8 \
  /private/tmp/picogent-evidence-264fbde/picogent-rendered-fixture-stamped \
  -phase reload -home "$disposable_home" \
  -workspace "$disposable_home/workspace" \
  -manifest "$disposable_home/reload-manifest.json" -addr 127.0.0.1:0
```

BrowserOS neo loaded the seed URL, submitted the fixed fixture prompt, acted on
the rendered permission and Undo controls, then navigated the same task-owned
tab to the fresh reload process.

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
/private/tmp/picogent-evidence-264fbde/rendered-recovery-observation.json
/private/tmp/picogent-evidence-264fbde/rendered-platform-evidence.json
/private/tmp/picogent-evidence-264fbde/rendered-seed-manifest.json
/private/tmp/picogent-evidence-264fbde/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     3d089633246b773f360a4bc5bdee8d085fc446ac72197451fedf18ad85862f9c
platform        f5feec7180e41dfe31ab036cb6ef0251401e18ba456498ad8b52cb1c4d823ba1
seed-manifest   b456150f3e6bcda8362a8ab01bfcb68d1950517ba1c125274231924511749ab5
reload-manifest ca87015543cf46e3956a5a663d2625cac1471b4e7da02f621e932b74597faa10
```

The local platform artifact recorded:

```text
candidate_sha=264fbde2b45609e6e85e175efe216e8f63509ab8
platform=darwin
architecture=arm64
browser=browseros-neo
fixture=rendered-recovery
observation_sha256=3d089633246b773f360a4bc5bdee8d085fc446ac72197451fedf18ad85862f9c
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

The behavior-SHA continuity rule can accept this local rendered artifact only
at a docs-only descendant of `264fbde`; it does not upgrade the cross-platform
row. The merge introducing that rule contains Go and test changes, so this
historical artifact cannot attach there. One fresh local observation at the
contract merge SHA is required; later docs-only evidence commits can then keep
that local artifact valid with `--behavior-sha`.
