# v4 rendered long-horizon direct evidence at exact main `6914a42`

Status: `PASS` for one bounded, task-owned Darwin BrowserOS neo observation of
the existing rendered long-horizon fixture at the exact current `main` tip.
This slice belongs to [#714](https://github.com/saiaathishkarthik/picogent/issues/714)
under the runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It is evidence only;
it is not live-provider, cross-platform, hostile-filesystem, or release-readiness
proof, and it does not claim v4 completion.

The prior record at `f8a78c6…` is superseded for current-tip claims. The new
observation was built from a true standalone clean clone. An earlier linked
worktree build was rejected from evidence because Go omitted VCS build metadata.

## Provenance

| Field | Value |
| --- | --- |
| Candidate and behavior SHA | `6914a42abddb1a789a9a655abeb4db61d9e9956c` |
| Head / tree | `PASS` / `CLEAN` |
| Behavior provenance | `EXACT_HEAD` |
| Runtime | `go-build-tags-rendered_fixture`, Go `1.26.6` |
| Fixture scenario | `rendered-multi-turn-outcome` / `long-horizon` |
| Fixture session | `rendered-long-horizon-fixture` |
| Browser | BrowserOS neo Chrome `148.0.0.0`, Darwin `arm64` |
| Browser session | `codex/long-horizon`, task-owned page `37` |
| Viewport | `1864 × 969` |
| Seed started | `2026-09-12T03:28:15.971317Z` |
| Reload started | `2026-09-12T03:37:02.037668Z` |
| Final observation | `2026-09-12T03:41:27.519Z` |
| Source metadata | `source_sha_verified=true`, `source_tree_modified=false` in both manifests |

The fixture used a disposable home/workspace and the deterministic fixture
provider. The seed process exited before the reload process started; the reload
process also exited cleanly after the final observation. No live provider was
used.

## Digest record

Secret-free artifacts remain outside the checkout at
`/private/tmp/picogent-evidence-6914a42-long-horizon-714/`:

```text
observation     6626b3263b359f8d6691ab5cae7c1d4ed45580c577be469cf4da206fe4d82cbb
seed-manifest   3430687e6eb06d84520a261316e889b41b436aa5323a7df9550316d7eb66ab86
reload-manifest 45dc4cfd810674352413546e0fa88fce9177a483a1ce0d4c88cfbe3097037345
screenshot      UNRECORDED (BrowserOS neo returned inline capture only)
```

The observation record is
`picogent.v4.rendered-long-horizon-observation.v2`. It retains only direct UI
strings, provenance, assertions, and digests; raw transcripts, credentials,
browser profile data, and fixture logs were not retained.

## Direct browser observations

The browser stayed in Safe mode and followed the fixture runbook. Each row is a
direct observation from the task-owned page, not an inference from the API
contract test.

| Sequence | Prompt / boundary | Directly observed UI | Outcome and freshness signal |
| --- | --- | --- | --- |
| 1 | `Create the rendered UI outcome probe` after mutation approval | `Blocked`; `Changed files (1)`; `Undo is available for the latest change`. | `INCONCLUSIVE · verify INCONCLUSIVE — rendered inspection is pending`; contained mutation staged while proof remained pending. |
| 2 | `Verify the rendered UI outcome probe` after verifier approval | Completion remained proof-gated; `PASS · verify PASS — deterministic workspace observation`; Undo remained available. | Deterministic workspace pass did not create direct rendered proof. |
| 3 | `Review the rendered UI outcome after steering its scope` after verifier approval | `Blocked`; `Steering changed the outcome contract; earlier proof is stale.`; `Undo last change`. | `INCONCLUSIVE · verify INCONCLUSIVE — fresh rendered inspection is required`; earlier proof was invalidated. |
| 4 | Fresh-process reload at the second fixture URL | Same durable task and transcript; `Blocked`; `Changed files (1)`; stale-steering text; `Undo last change`. | Reload crossed the process boundary without promoting stale completion proof. |
| 5 | `Verify the rendered UI outcome after reload` after verifier approval | `The reloaded task is still fail-closed; fresh rendered inspection is required.`; `Undo last change`. | `INCONCLUSIVE · verify INCONCLUSIVE — fresh rendered inspection is required`; fresh rendered proof remained required. |

## Interpretation

- The bounded rendered surface preserved task identity, mutation visibility,
  verification history, steering invalidation, Undo availability, and durable
  reload recovery.
- The deterministic `PASS` did not promote completion because direct rendered
  proof was still missing.
- Steering invalidated earlier proof, and reload did not reuse stale proof.
- `live-provider-connectivity`, `live-provider-quality`,
  `hostile-filesystem-toctou`, unsupported platforms, cross-platform aggregate,
  broader crash windows, and `release-authorization` remain `UNVERIFIED`.

## Reproduction

From a true standalone clean checkout at the exact SHA, with low-resource
settings:

```sh
GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local \
  go build -buildvcs=true -tags rendered_fixture \
  -o /private/tmp/picogent-rendered-long-horizon-714-standalone \
  ./cmd/picogent-rendered-fixture
```

Run the seed fixture with `PICOGENT_RENDERED_FIXTURE_SOURCE_SHA` set to the
exact SHA, follow the three seed prompts in Safe mode, approving only the
contained mutation and verifier permissions, then stop the process. Start the
reload phase with the exact home/workspace paths from the seed manifest and
submit `Verify the rendered UI outcome after reload`. Verify both manifests
report `source_sha_verified=true` and `source_tree_modified=false` before
retaining any result.

Focused API coverage also passed:

```text
ok  github.com/saiaathish/picogent/internal/gui  1.918s
```

This bounded observation must not be relabeled as live-provider or release
evidence, and it does not close #453.
