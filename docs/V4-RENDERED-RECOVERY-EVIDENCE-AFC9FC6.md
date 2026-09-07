# v4 rendered recovery evidence at tip afc9fc6

This record refreshes the direct BrowserOS neo allow→undo→fresh-process reload
observation against exact `origin/main` tip
`afc9fc6f85c23976d5a356757f48f1c5e4d50f4b`. It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim rendered behavior on Linux or Windows, live-provider recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `afc9fc6f85c23976d5a356757f48f1c5e4d50f4b`.
- Fixture binary:
  `vcs.revision=afc9fc6f85c23976d5a356757f48f1c5e4d50f4b`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/tip-recovery-evidence`; task-owned page `374`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-06T23:59:25.349866Z`; reload started at
  `2026-09-06T23:59:50.773052Z`.
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
  /private/tmp/picogent-rendered-source-afc9fc6
git -C /private/tmp/picogent-rendered-source-afc9fc6 \
  checkout --detach afc9fc6f85c23976d5a356757f48f1c5e4d50f4b

cd /private/tmp/picogent-rendered-source-afc9fc6
go build -buildvcs=true -tags rendered_fixture \
  -o /private/tmp/picogent-evidence-afc9fc6/picogent-rendered-fixture-stamped \
  ./cmd/picogent-rendered-fixture

PICOGENT_RENDERED_FIXTURE_SOURCE_SHA=afc9fc6f85c23976d5a356757f48f1c5e4d50f4b \
  /private/tmp/picogent-evidence-afc9fc6/picogent-rendered-fixture-stamped \
  -home "$disposable_home" -manifest "$disposable_home/seed-manifest.json" \
  -addr 127.0.0.1:0

PICOGENT_RENDERED_FIXTURE_SOURCE_SHA=afc9fc6f85c23976d5a356757f48f1c5e4d50f4b \
  /private/tmp/picogent-evidence-afc9fc6/picogent-rendered-fixture-stamped \
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
/private/tmp/picogent-evidence-afc9fc6/rendered-recovery-observation.json
/private/tmp/picogent-evidence-afc9fc6/rendered-platform-evidence.json
/private/tmp/picogent-evidence-afc9fc6/rendered-seed-manifest.json
/private/tmp/picogent-evidence-afc9fc6/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     8bd8851ec5153f4d3efdde56c9941063795fe412fb582a762fd21fa5cbe64bac
platform        7d39424a4b5ac6bea19963d6181c02b15c63d408381c9edee981b5ab349001df
seed-manifest   01540ba742e20e8ed17b582fc12f58812084af9cce2f28b5a4046b047dc27eff
reload-manifest 3bc807033813fbae00b7627d6c88fdc09f67b16b8c0668d6e13ff7277f7610c2
```

The local platform artifact recorded:

```text
candidate_sha=afc9fc6f85c23976d5a356757f48f1c5e4d50f4b
platform=darwin
architecture=arm64
browser=browseros-neo
fixture=rendered-recovery
observation_sha256=8bd8851ec5153f4d3efdde56c9941063795fe412fb582a762fd21fa5cbe64bac
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
