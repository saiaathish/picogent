# v4 Windows rendered recovery evidence at tip f8a78c6

This record adds the direct Windows owned-browser observation at exact behavior
SHA `f8a78c646877164009953401772acdb4d346175d` under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves the `windows/amd64` rendered recovery path in a task-owned disposable
Playwright Chromium profile on GitHub Actions `windows-latest`. Combined with
the matching Darwin and Linux observations at the same SHA, the three-input
packager produced a tip PASS aggregate. This record does not upgrade
hostile-filesystem TOCTOU or release authorization.

## Provenance

- Source: clean detached checkout at
  `f8a78c646877164009953401772acdb4d346175d` inside the hosted Windows runner.
- Fixture binary:
  `vcs.revision=f8a78c646877164009953401772acdb4d346175d`,
  `vcs.modified=false` (verified via `go version -m` / fixture-buildinfo).
- Runtime: GitHub Actions `windows-latest`, Go `1.25.x`.
- Owned browser: Playwright Chromium `134.0.6998.35`, headless, disposable
  persistent profile under Local Temp.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T10:43:09.21033Z`; reload started in a fresh
  fixture process at `2026-09-07T10:43:14.8710774Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Collection run:
  [34112789982](https://github.com/saiaathish/picogent/actions/runs/34112789982)
  (Run 12; artifact
  `rendered-windows-owned-browser-f8a78c646877164009953401772acdb4d346175d`,
  zip digest
  `sha256:14f9d2fd88c03b5cc7c7219edafb990989bb1c4c14bb447f36e6cc716e9a0bb7`).

The source checkout, compiled fixture, fixture home, workspace, browser
profile, and browser process were task-owned and disposable. Chromium drove the
normal embedded GUI permission, chat, undo, task-store, and session-store
paths. Hosted API-only fixture tests were not used as browser evidence.

## Direct rendered observations

1. The seed page rendered in Safe mode without Undo; the probe was absent.
2. After prompt submission, Chromium rendered the permission controls while the
   probe remained absent.
3. Allow rendered `Edited 1 file`, `Changed files (1)`, and
   `Undo last change`; the probe digest matched the fixture contract.
4. Undo removed the probe and made Undo unavailable while
   `Changed files (1)` remained visible.
5. After the seed process stopped, the same owned Chromium process loaded a
   fresh reload process and rendered durable `Changed files (1)` without stale
   Undo; the probe remained absent.

The observation verdict is `PASS`.

## Host-local / retained artifacts

Artifacts remain outside the checkout. Operator copies used for packaging:

```text
/private/tmp/picogent-evidence-f8a78c6/windows/
/private/tmp/picogent-evidence-f8a78c6/aggregate/rendered-cross-platform-evidence.json
```

Artifact SHA-256:

```text
observation       68e7c8e9c6703391a41854b8837ba51d6a585810b929cfc4ecdb0a072de21941
platform          9c54b13f123aeffc9c0292964fc66015e598af7378bdf3799cbe53c6173be6b7
seed-manifest     7fc957991e0085cc03d4ef4a44f3c8ff59486f372264842581cdf58acc550d10
reload-manifest   2dba591d1fee4a254a0d5ddcb57d6e5f59e689b64bf6c8ab16ee8d7d2de8c39e
screenshot-set    10b14a30f2cfb7a32f712198d97d801672f829b986bcb8a35e20b1fcaea30639
initial           c2df5cf8932b3e37aa40c0580a1d1158a38a8e9321d6b9a013bea99722367c39
permission        2c028a390c94a4dd321c9d46821a5fa317c14c3070a9b5019048dd7e8380ba91
allowed           418e70558600c86fa7678f0798ade5e6c71709ccee7203add4ea72d2cf3604e9
undone            2a3a0ca44abb18b3a03d043767126711a6314501e62035397a05e736674b09b9
reload            7aa0a378297a39e25c3afa19c4ce8adfc50266b1bf02516f07bd43d363d23b05
cross-aggregate   5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd
matrix-full       85847718b56306f38ef2f78cf5a7d5b425f28917429b458944ae2e9cce2f856d
```

The digest-only platform record contains:

```text
candidate_sha=f8a78c646877164009953401772acdb4d346175d
platform=windows
architecture=amd64
environment=task-owned-disposable
browser=playwright-chromium-headless-134.0.6998.35
fixture=rendered-recovery
observation_sha256=68e7c8e9c6703391a41854b8837ba51d6a585810b929cfc4ecdb0a072de21941
screenshot_sha256=10b14a30f2cfb7a32f712198d97d801672f829b986bcb8a35e20b1fcaea30639
verdict=PASS
source_tree_modified=false
```

## Cross-platform aggregate

Packaged from a clean detached checkout at
`f8a78c646877164009953401772acdb4d346175d` with the matching Darwin and Linux
platform records. Aggregate `verdict=PASS`. Exact-SHA matrix validation at that
clean checkout projected `rendered-cross-platform=PASS`
(PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3).

Docs-only descendants of this tip retain live/local rendered and this aggregate
with `--behavior-sha f8a78c646877164009953401772acdb4d346175d`; they do not
rebind the observation to a later non-docs tip without recollection.

## Explicit non-claims

- does not upgrade `hostile-filesystem-toctou` or `release-authorization`
- does not treat API-boundary fixture tests as owned-browser proof
- does not authorize a release by itself
