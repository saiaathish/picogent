# v4 rendered recovery evidence at tip d129899

This record refreshes the direct BrowserOS neo allow→undo→fresh-process reload
observation against exact `origin/main` tip
`d1298998cc000c40ad2fdaf71a24e17649e09e19`. It belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves only the local `darwin/arm64` rendered recovery path. It does not
claim rendered behavior on Linux or Windows, live-provider recovery,
hostile-filesystem TOCTOU safety, or release authorization.

## Provenance

- Source SHA: `d1298998cc000c40ad2fdaf71a24e17649e09e19`.
- Fixture binary:
  `vcs.revision=d1298998cc000c40ad2fdaf71a24e17649e09e19`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`; host: `darwin/arm64`.
- Browser session: `cursor/tip-recovery-evidence`; task-owned page `392`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-07T01:14:57.893403Z`; reload started at
  `2026-09-07T01:16:01.017648Z`.
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
  /private/tmp/picogent-rendered-source-d129899
git -C /private/tmp/picogent-rendered-source-d129899 \
  checkout --detach d1298998cc000c40ad2fdaf71a24e17649e09e19

cd /private/tmp/picogent-rendered-source-d129899
go build -buildvcs=true -tags rendered_fixture \
  -o /private/tmp/picogent-evidence-d129899/picogent-rendered-fixture-stamped \
  ./cmd/picogent-rendered-fixture

PICOGENT_RENDERED_FIXTURE_SOURCE_SHA=d1298998cc000c40ad2fdaf71a24e17649e09e19 \
  /private/tmp/picogent-evidence-d129899/picogent-rendered-fixture-stamped \
  -home "$disposable_home" -manifest "$disposable_home/seed-manifest.json" \
  -addr 127.0.0.1:0

PICOGENT_RENDERED_FIXTURE_SOURCE_SHA=d1298998cc000c40ad2fdaf71a24e17649e09e19 \
  /private/tmp/picogent-evidence-d129899/picogent-rendered-fixture-stamped \
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
/private/tmp/picogent-evidence-d129899/rendered-recovery-observation.json
/private/tmp/picogent-evidence-d129899/rendered-platform-evidence.json
/private/tmp/picogent-evidence-d129899/rendered-seed-manifest.json
/private/tmp/picogent-evidence-d129899/rendered-reload-manifest.json
```

Artifact SHA-256:

```text
observation     868fce3e44400bd83c9e804d4808965e06ae6d08cda80ae67db580aafcbddbcf
platform        77804610e72ea1afee325857224ba2be8b646a89a10cb8fe4e42329c8f4aa683
seed-manifest   46e222f13738b35338742cd54101a966e8d6268e1e4498d505dde6ccb687448f
reload-manifest 4c0568ac312a28de5f4021425ca1e4e99b725f0fb02b8e03e8e0f2d544fcb45f
```

The local platform artifact recorded:

```text
candidate_sha=d1298998cc000c40ad2fdaf71a24e17649e09e19
platform=darwin
architecture=arm64
browser=browseros-neo
fixture=rendered-recovery
observation_sha256=868fce3e44400bd83c9e804d4808965e06ae6d08cda80ae67db580aafcbddbcf
verdict=PASS
source_tree_modified=false
```

## Matrix result and retention

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

Linux and Windows owned-browser artifacts do not exist at this SHA, so
`rendered-cross-platform` remains `UNVERIFIED`.

At a later clean docs-only descendant, supply the unchanged artifact together
with the candidate tip and this observation SHA:

```sh
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-d129899/rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)" \
  --behavior-sha d1298998cc000c40ad2fdaf71a24e17649e09e19
```

The matrix accepts that local artifact only when Git proves every intervening
touch is under `docs/`; it then records
`behavior_provenance=DOCS_ONLY_DESCENDANT`. This does not upgrade
`rendered-cross-platform`, hostile-filesystem TOCTOU, or release authorization.
