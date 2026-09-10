# v4 Windows rendered recovery evidence at current candidate f0e5dd7

This record belongs to [#603](https://github.com/saiaathishkarthik/picogent/issues/603),
the focused Windows child under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh hosted Windows owned-browser observation of the real embedded GUI at
exact candidate SHA `f0e5dd7da8387748b723964e559c7f8a8e300df6`.

The observation is bounded to Windows rendered recovery. It does not claim
Darwin behavior at this candidate, cross-platform rendered coverage,
live-provider quality, arbitrary hostile same-UID filesystem races, or release
authorization.

## Provenance

- Hosted run: [GitHub Actions run 34441949574](https://github.com/saiaathishkarthik/picogent/actions/runs/34441949574).
- Source: clean exact-SHA checkout of
  `f0e5dd7da8387748b723964e559c7f8a8e300df6` on the hosted Windows runner.
- Fixture: `go-build-tags-rendered_fixture`; the workflow built it with
  `-buildvcs=true` from the exact checked-out candidate and verified the clean
  source checkout before collection.
- Runtime: `windows/amd64`, Go `go1.25.14`.
- Browser: task-owned Chrome headless `151.0.7922.174`; fixture session
  `rendered-recovery-fixture`.
- Observation completed at `2026-09-10T05:40:29.500Z`; the reload manifest was
  published at `2026-09-10T05:40:29.0647007Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`; both seed and reload fixture processes exited
  before the observation was published.
- The disposable fixture home, workspace, browser profile, manifests, and
  screenshot remained outside the checkout in the hosted runner temp area.

## Direct rendered observations

The Windows collector drove the normal embedded GUI through the complete
allow/undo/reload sequence. Its observation record set all behavioral
assertions below to true, with `source_tree_modified=false` as the explicit
clean-source assertion:

| Sequence | Direct UI/DOM observation | Corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page rendered with the Undo control disabled. | `seed_initial_undo_disabled=true`; seed manifest recorded the probe as absent. | `PASS` |
| 2 | The Safe permission prompt rendered for the contained recovery probe. | `permission_rendered=true`; the collector checked the probe-specific permission text. | `PASS` |
| 3 | Clicking `Allow` rendered the contained one-file mutation, including `Edited 1 file` and `Changed files (1)`. | `allow_rendered_contained_mutation=true`; the probe content matched the fixture contract. | `PASS` |
| 4 | Clicking Undo rendered restoration and removed the probe. | `undo_rendered_restoration=true`; the probe was absent before reload. | `PASS` |
| 5 | A fresh reload process rendered durable `Changed files (1)` with Undo disabled. | `reload_rendered_durable_history=true`, `reload_undo_disabled=true`, and `probe_absent_after_reload=true`. | `PASS` |

## Retained digest-only artifacts

The platform record and observation record agree on the exact candidate,
platform, architecture, browser, and fixture. Independently captured SHA-256
values are:

```text
observation       f1909b2e885ce109d8e44fa4f598df319082ce38be68ac691c1c0d537d231d0e
platform          e2791efb280abf542d24fe09b65215fc2ed8e8723d849d6ecec229651f00df5e
seed-manifest     271e922ea47a6927d0c21787322052c7ee86cec733e6aa5ffb77ccd026e90261
reload-manifest   0d0ca8fb563a6a28669cff19792d4d6f11d1325162ac4e2da8a1f664edb20491
runtime-matrix    9d80ec0bb837df94ddf7f5c0d478e9ff97303990c5be6030fa37e328a621f52d
screenshot        1b2c675bf221dfb3c952e29f2df42302f0ddf58e9c6f9362d1815ef8fe1cdea1
```

The platform record declares:

```text
candidate_sha=f0e5dd7da8387748b723964e559c7f8a8e300df6
platform=windows
architecture=amd64
environment=task-owned-disposable
browser=chrome-headless-151.0.7922.174
fixture=rendered-recovery
verdict=PASS
source_tree_modified=false
```

Independent hash comparison confirmed that the observation and screenshot
digests in the platform record match the retained files. No DOM transcript,
URL, credential, or screenshot bytes are stored in the repository.

The hosted exact-head runtime matrix reported:

```text
HEAD=PASS  tree=CLEAN  host=windows/amd64
PASS: 7  INCONCLUSIVE: 1  UNVERIFIED: 4
rendered-platform-local=PASS
rendered-cross-platform=UNVERIFIED
live-provider-connectivity=UNVERIFIED
live-provider-quality=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Windows/amd64.
- `PASS`: `rendered-platform-local` for Windows/amd64 at candidate
  `f0e5dd7da8387748b723964e559c7f8a8e300df6`.
- `UNVERIFIED`: Darwin at this candidate and the exact-candidate
  cross-platform rendered claim.
- `UNVERIFIED`: live-provider claims and broad same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
