# v4 rendered recovery API-boundary evidence

This records the deterministic HTTP API-boundary coverage for the rendered
recovery fixture under [#453](https://github.com/saiaathish/picogent/issues/453).

## Automated observation

Command:

```sh
go test -tags rendered_fixture ./internal/gui -run '^TestRenderedRecoveryFixtureAPIBoundary$' -count=1
```

The test drives the real embedded GUI handler with `llm.Scripted` only:

1. seed chat creates a Safe-mode permission prompt;
2. allow writes the contained recovery probe and projects `undo_available`;
3. `/undo` removes the probe and clears undo availability;
4. a fresh reload process projects the durable task with the probe still absent
   and undo unavailable.

## Explicit non-claims

- Not a browser DOM or owned-tab observation.
- Not a live-provider quality claim.
- Not cross-platform rendered coverage.
- Not arbitrary same-UID hostile filesystem proof.
- Does not authorize a release.
