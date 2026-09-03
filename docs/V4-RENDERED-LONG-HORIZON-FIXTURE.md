# v4 rendered long-horizon fixture

This build-tagged fixture is the medium lane for
[#368](https://github.com/saiaathish/picogent/issues/368), under
[#366](https://github.com/saiaathish/picogent/issues/366),
[#306](https://github.com/saiaathish/picogent/issues/306), and the broader
outcome contract [#246](https://github.com/saiaathish/picogent/issues/246).
It is deterministic evidence plumbing, not a live-provider or release test.

## Run

Use a disposable home and workspace. The source SHA is optional for local
exploration, but required for a verifiable evidence run:

```sh
PICOGENT_RENDERED_FIXTURE_SOURCE_SHA="$(git rev-parse HEAD)" \
  go run -tags rendered_fixture ./cmd/picogent-rendered-fixture \
  -scenario long-horizon
```

The command prints a task-owned loopback URL, manifest, workspace, and fixed
session ID. In an owned browser tab, use these prompts in order:

1. `Create the rendered UI outcome probe`
2. approve the Safe-mode mutation permission, then approve the deterministic
   verifier permission when it appears;
3. `Verify the rendered UI outcome probe`, and approve its verifier permission;
4. `Review the rendered UI outcome after steering its scope`, and approve its
   verifier permission.

The task must keep `completion.ready` false because the deterministic verifier
does not provide direct rendered proof. The first verifier result is
`INCONCLUSIVE`, the second is `PASS`, and the steering recheck is
`INCONCLUSIVE`; this sequence makes a stale or optimistic projection visible.

Stop the seed process after the steering turn. Start the fresh reload phase
with the exact paths printed by the seed manifest:

```sh
PICOGENT_RENDERED_FIXTURE_SOURCE_SHA="$(git rev-parse HEAD)" \
  go run -tags rendered_fixture ./cmd/picogent-rendered-fixture \
  -scenario long-horizon -phase reload \
  -home '<fixture home>' -workspace '<fixture workspace>'
```

Submit `Verify the rendered UI outcome after reload` and approve the verifier
permission. The fresh process loads the same durable session/task, recovers an
active admitted turn when one exists, and keeps the rendered completion gate
closed until direct rendered proof is available.

## What the medium test proves

`TestRenderedLongHorizonFixtureAPIBoundary` runs the same flow in a disposable
`httptest` server so it can validate the normal production boundaries without
requiring a browser:

- `/api/chat` admits each turn and persists the transcript;
- `/api/permission` governs the mutation and verifier decisions;
- `/api/state` agrees with the authoritative task completion check;
- `/api/events` carries session-bound task and permission projections with the
  current durable turn identity; and
- a fresh `SetTaskSession` load records process-restart recovery and does not
  reuse the previous proof.

The test validates a bounded five-observation
`picogent.v4.rendered-long-horizon.v1` report with exact source/runtime
metadata. It records the owned browser DOM, screenshot, and live-provider
boundaries as `UNVERIFIED`; those belong to the conditional large lane.

The fixture uses the existing taskstate, Outcome Engine, GUI SSE/state,
permission, session, and task-store seams. The fixture-only suppression of
extension-agent rebuilding keeps the scripted provider stable; it does not
change normal GUI behavior.
