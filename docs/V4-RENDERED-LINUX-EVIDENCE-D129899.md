# v4 Linux rendered recovery evidence at behavior SHA d129899

This record adds the direct Linux owned-browser observation required by
[#507](https://github.com/saiaathish/picogent/issues/507), under parent
[#453](https://github.com/saiaathish/picogent/issues/453). It uses the same
exact behavior SHA as the retained Darwin record:
`d1298998cc000c40ad2fdaf71a24e17649e09e19`.

It proves only the `linux/arm64` rendered recovery path. Windows remains
unobserved, so `rendered-cross-platform` remains `UNVERIFIED`; no aggregate
artifact was produced. This record does not upgrade hostile-filesystem TOCTOU
or release authorization.

## Provenance

- Source SHA: `d1298998cc000c40ad2fdaf71a24e17649e09e19`.
- Source: fresh detached clone inside a task-owned Linux container.
- Fixture binary:
  `vcs.revision=d1298998cc000c40ad2fdaf71a24e17649e09e19`,
  `vcs.modified=false`.
- Runtime: Docker Desktop Linux VM, `linux/arm64`; Go `1.25.14`.
- Owned browser: Chromium `152.0.7977.82`, headless mode, with a disposable
  task-owned profile inside the same Linux container.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T01:44:45.421339841Z`; reload started in a fresh
  fixture process at `2026-09-07T01:44:56.682074804Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The source clone, compiled fixture, fixture home, workspace, browser profile,
and browser process were all task-owned and disposable. Chromium drove the
normal embedded GUI and its real permission, chat, undo, task-store, and
session-store paths. No hosted fixture/API test was used as browser evidence.

## Direct rendered observations

1. The fresh seed page rendered in Safe mode with no available Undo control;
   the probe was absent.
2. After prompt submission, Chromium rendered Deny, This turn, Always allow,
   and Allow while the probe remained absent.
3. Allow rendered `Edited 1 file`, `Changed files (1)`, and
   `Undo last change`; the probe digest matched the fixture contract.
4. Undo removed the probe and made Undo unavailable while
   `Changed files (1)` remained visible.
5. The seed fixture process was terminated. The same owned Chromium process
   loaded a new URL from a fresh reload fixture process and rendered the
   durable `Changed files (1)` state without stale Undo; the probe remained
   absent.

The observation verdict is `PASS`.

## Host-local artifacts

Artifacts remain outside the checkout:

```text
/private/tmp/picogent-evidence-linux-d129899/linux-rendered-recovery-observation.json
/private/tmp/picogent-evidence-linux-d129899/linux-rendered-platform-evidence.json
/private/tmp/picogent-evidence-linux-d129899/linux-rendered-seed-manifest.json
/private/tmp/picogent-evidence-linux-d129899/linux-rendered-reload-manifest.json
/private/tmp/picogent-evidence-linux-d129899/linux-{initial,permission,allowed,undone,reload}.png
```

Artifact SHA-256:

```text
observation       d6f407ba4327b4e49e09642d52bcae84d90fbcf15ccf39d4f2f2b56f653dcaef
platform          bf893a448e3c603a9b459ddfd896861960be2d9fb555114e58c74bb26d353a7b
seed-manifest     35fbb22a625f0220f87013b019440d57125ba40db000c4ef267bffa3220ea65f
reload-manifest   9c5a9be6b5ab311c16581998ea796957a678af522874ca3cb1dcc47ae2744a36
screenshot-set    29a04ebb46b0ebc6ef99d4793a258ea50ced128164d916307b211dbf63a5b59b
initial           11aee545e14719e1a0dff36fbb2c3dc22e46f7f118357c6759ecbf5faee9a44f
permission        c1f3a67ab12e84d811af18f6afebc2469317f2af1b24163e4375be79afc55009
allowed           17e76116c8d0bef21ac298f278737bb70e4980aed024907e91d4d744a05ad5b9
undone            441f2ecd9594461400da4fce5fbd28a07e9ff5f166df3122391815f3b897b525
reload            85a2da522dd5fcaf5032b952a892065ac49049b2de0499a054380f4d29f592e9
```

The digest-only platform record contains:

```text
candidate_sha=d1298998cc000c40ad2fdaf71a24e17649e09e19
platform=linux
architecture=aarch64
environment=task-owned-disposable
browser=chromium-headless-152.0.7977.82
fixture=rendered-recovery
observation_sha256=d6f407ba4327b4e49e09642d52bcae84d90fbcf15ccf39d4f2f2b56f653dcaef
screenshot_sha256=29a04ebb46b0ebc6ef99d4793a258ea50ced128164d916307b211dbf63a5b59b
verdict=PASS
source_tree_modified=false
```

## Validation and remaining boundary

The JSON artifacts parsed successfully. On the clean docs-only descendant
`d9c1f16a2433b019591f813cb4996f5046826aa4`, these focused suites passed:

```text
go test ./cmd/rendered-cross-platform-evidence ./internal/runtimeboundary
ok github.com/saiaathish/picogent/cmd/rendered-cross-platform-evidence
ok github.com/saiaathish/picogent/internal/runtimeboundary
```

The retained Darwin platform artifact and this Linux artifact agree on
`d1298998cc000c40ad2fdaf71a24e17649e09e19`. A real Windows owned-browser
artifact at that exact SHA is still required before running the three-input
packager and validating the aggregate through
`PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1`.
