# Windows owned-browser rendered recovery collector

Task-owned Windows evidence helper for [#507](https://github.com/saiaathish/picogent/issues/507).

## What it proves

A real Chromium process (Playwright, disposable profile) drives the normal
embedded GUI through allow → undo → fresh fixture-process reload. Hosted
API-only tests are not substitutes.

## Local / CI usage

Build the fixture from a clean checkout at the exact behavior SHA, then:

```sh
python scripts/rendered-windows/collect_rendered_recovery.py \
  --sha 18dc4a1ca1137ab78dfd0102848eb99c417653cc \
  --fixture-bin /path/to/picogent-rendered-fixture.exe \
  --out /absolute/path/outside/checkout/windows-evidence \
  --home-root "$TEMP/picogent-rendered-windows-507"
```

The GitHub Actions workflow
`.github/workflows/rendered-windows-owned-browser.yml` performs that flow on
`windows-latest` and uploads digest-only JSON plus operator screenshots.

## Fail-closed rules

- Fixture must embed `vcs.revision=<exact SHA>` and `vcs.modified=false`
- Missing, dirty, or contradictory observations exit non-zero
- Never synthesize a Windows PASS placeholder
