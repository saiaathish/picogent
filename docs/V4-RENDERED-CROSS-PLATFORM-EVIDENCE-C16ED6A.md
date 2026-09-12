# v4 rendered recovery evidence at exact candidate `c16ed6a`

Status: `PASS` for the task-owned rendered recovery flow on Darwin, Linux,
and Windows, plus the exact-candidate cross-platform aggregate. This child
belongs to [#712](https://github.com/saiaathish/picogent/issues/712) under the
runtime-boundary parent [#453](https://github.com/saiaathish/picogent/issues/453).
It does not authorize a release or claim v4 completion.

## Candidate and flow

All three observations used the exact clean candidate
`c16ed6ad7bd37133ed47c560840c7c692eb958c8`, fixture `rendered-recovery`, and
task-owned disposable browser/profile and home/workspace state. The recovery
flow exercised:

1. fresh Safe-mode page with no live Undo control;
2. submitted recovery task and rendered permission controls;
3. Allow for the contained one-file mutation;
4. rendered change confirmation and Undo;
5. process shutdown, fresh-process reload, and durable history without stale
   Undo state.

Each collector record reports `source_sha_verified=true`,
`source_tree_modified=false`, clean fixture exits, and all 18 required
recovery checks true. Only digest-oriented JSON is retained; screenshots,
browser profiles, manifests, and raw session state are not committed.

The Darwin run was built from a standalone clean clone so Go embedded
`vcs.revision=c16ed6ad7bd37133ed47c560840c7c692eb958c8` and
`vcs.modified=false`. An earlier linked-worktree attempt was rejected by the
fixture with `source_sha_verified=false` and was not used as evidence.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `ccd346599a1f7612fdc575b08255be25349c30cd341555de209365fb321dba7f` | `8e7dbb2911b7073d8311351ecce1079457828ec8eeef6dbc7448a0462d5f9bbe` | `142b0e4d2544d50edda45c90ada50ee76d5996d582bee9554aa97076f2f225b2` |
| `linux/amd64` | [hosted run 34667949843](https://github.com/saiaathish/picogent/actions/runs/34667949843) | `playwright-chromium-headless-134.0.6998.35` | `6fb89bdc9ea7891dd092169360e3a681155230fe642c9dad994ca54f47384486` | `f2e000ce8a3530b63eadfc05af300164f2cd86ef92ea509f8f9de744d3272ec6` | `d155d74fbd247af07ac5220b7a23aa7c299bb487a3477ce6189a0f8ae1ef2914` |
| `windows/amd64` | [hosted run 34667949836](https://github.com/saiaathishkarthik/picogent/actions/runs/34667949836) | `chrome-headless-152.0.7977.83` | `22396509a64934ad8d98ec95f210076866c82bf102e8469ab515af25b19604b1` | `82969cfef35a7de1e0ef3cf2b914554a316bf25b71b488107bb14ab87b843b3a` | `768d75c900b373a85a0db63588c37495894669839410d57e2aa3be6760b7c454` |

The hosted collectors completed successfully and uploaded their digest-only
JSON artifacts. Their Node.js 20 deprecation annotations are workflow-runtime
warnings, not evidence failures.

## Aggregate and exact-candidate matrix

The aggregate validator accepted exactly one valid PASS record for each
required platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=c16ed6ad7bd37133ed47c560840c7c692eb958c8
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=b0cfcd12f2cf22387ccabe21007c296803f3d984dd77535fd3291029f7c86165
```

The exact-candidate matrix was generated from clean main with the Darwin local
record and the three-platform aggregate supplied:

```text
candidate_sha=c16ed6ad7bd37133ed47c560840c7c692eb958c8
behavior_sha=c16ed6ad7bd37133ed47c560840c7c692eb958c8
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
summary=PASS 2 / UNVERIFIED 10
runtime_matrix_sha256=f483450808a82c0a6057f87cfff5c5eac535fa811c8602e03dde3d2545dd9073
```

The two PASS rows are `rendered-platform-local` and
`rendered-cross-platform`. The remaining rows are explicitly `UNVERIFIED`:

- hostile child-environment sanitization;
- deterministic hostile filesystem behavior;
- broad same-UID filesystem TOCTOU;
- hostile parent-swap confinement;
- live-provider connectivity and quality in this rendered-only matrix;
- release authorization;
- rendered long-horizon and recovery/undo/reload rows; and
- restart, steering, undo, and recovery.

The live-provider PASS records are retained separately in
[V4-LIVE-PROVIDER-EVIDENCE-8B88662.md](V4-LIVE-PROVIDER-EVIDENCE-8B88662.md)
under their exact behavior SHA and are not silently relabeled as current-head
rendered evidence.

## Retained artifacts

Secret-free artifacts remain outside the checkout under
`/private/tmp/picogent-rendered-out-712.g6xjPy/`:

```text
darwin-rendered-platform-evidence.json
linux/.../linux-rendered-platform-evidence.json
windows/.../windows-rendered-platform.json
rendered-cross-platform-evidence.json
runtime-boundary-matrix-rendered.json
```

The aggregate and matrix commands were:

```sh
go run ./cmd/rendered-cross-platform-evidence \
  --workspace . \
  --candidate-sha c16ed6ad7bd37133ed47c560840c7c692eb958c8 \
  --darwin /private/tmp/picogent-rendered-out-712.g6xjPy/darwin-rendered-platform-evidence.json \
  --linux /private/tmp/picogent-rendered-out-712.g6xjPy/linux/<run>/picogent-linux-rendered-evidence/linux-rendered-platform-evidence.json \
  --windows /private/tmp/picogent-rendered-out-712.g6xjPy/windows/<run>/windows-rendered-platform.json \
  --out /private/tmp/picogent-rendered-out-712.g6xjPy/rendered-cross-platform-evidence.json

PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-rendered-out-712.g6xjPy/darwin-rendered-platform-evidence.json \
PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT=/private/tmp/picogent-rendered-out-712.g6xjPy/rendered-cross-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha c16ed6ad7bd37133ed47c560840c7c692eb958c8 \
  --out /private/tmp/picogent-rendered-out-712.g6xjPy/runtime-boundary-matrix-rendered.json
```

This is bounded rendered-runtime evidence. It does not prove arbitrary
same-UID filesystem race resistance, live-provider quality, long-horizon
behavior, universal recovery, release authorization, or v4 completion.
