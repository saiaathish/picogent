# Owned-browser rendered recovery collector

Task-owned desktop evidence helper for [#507](https://github.com/saiaathish/picogent/issues/507).

## What it proves

A real Chromium process (Playwright, disposable profile) drives the normal
embedded GUI through allow → undo → fresh fixture-process reload. Hosted
API-only tests are not substitutes. The collector retains the seed/reload
manifests and screenshot outside the checkout, with SHA-256 links in the
observation artifact, and verifies both fixture processes exited before
publishing PASS.

## Local / CI usage

Build the fixture from a clean checkout at the exact behavior SHA, then:

```sh
python scripts/rendered-recovery/collect_owned_browser.py \
  --platform windows \
  --sha 18dc4a1ca1137ab78dfd0102848eb99c417653cc \
  --fixture-bin /path/to/picogent-rendered-fixture.exe \
  --out /absolute/path/outside/checkout/windows-evidence \
  --home-root "$TEMP/picogent-rendered-windows-507"
```

Pass `--platform linux` or `--platform darwin` for the corresponding owned
desktop runtime. The GitHub Actions workflows perform the Windows and Linux
flows on fresh hosted runners and upload digest-only JSON plus screenshots.

## Fail-closed rules

- Fixture must embed `vcs.revision=<exact SHA>` and `vcs.modified=false`
- Missing, dirty, or contradictory observations exit non-zero
- Output and disposable-home paths must be absolute, outside the checkout,
  and free of symbolic-link components
- Platform must be one of `darwin`, `linux`, or `windows`; the artifact name
  and record platform are derived from that explicit value
- The rendered runtime matrix must report `rendered-platform-local=PASS`
- Never synthesize a Windows PASS placeholder
