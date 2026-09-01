# v4 rendered recovery fixture

This build-tagged fixture supports the large rendered-recovery lane in
[#291](https://github.com/saiaathishkarthik/picogent/issues/291), under parent
[#246](https://github.com/saiaathishkarthik/picogent/issues/246). It is an
evidence harness, not a second Picogent workflow and not a live-provider test.

## Run

Use a disposable home and workspace. The seed process serves the normal
embedded GUI with a deterministic `llm.Scripted` provider:

Custom `-home` paths must be below the operating system temp directory and
must be empty for seed; `-workspace` must be inside that home, and
`-manifest` must be a new file inside that home. The fixture rejects existing
seed state and never overwrites a manifest.

```sh
PICOGENT_RENDERED_FIXTURE_SOURCE_SHA="$(git rev-parse HEAD)" \
  go run -tags rendered_fixture ./cmd/picogent-rendered-fixture
```

The command prints a task-owned local URL, workspace, fixed session ID, and a
JSON manifest path. In an owned browser tab:

1. load the printed URL and confirm the initial task state has no undo control;
2. submit the fixture prompt;
3. confirm the Safe-mode permission prompt is visible while the probe path is
   absent, then click Allow;
4. confirm the rendered task shows the contained probe change and the undo
   control is available;
5. click `Undo last change` and confirm the probe path is absent, the undo
   control is hidden, and the task proof is no longer current;
6. stop the seed process and start the reload phase with the printed paths:

```sh
PICOGENT_RENDERED_FIXTURE_SOURCE_SHA="$(git rev-parse HEAD)" \
  go run -tags rendered_fixture ./cmd/picogent-rendered-fixture \
  -phase reload -home '<fixture home>' -workspace '<fixture workspace>'
```

Reload the same task-owned tab at the new URL. The fresh process must project
the durable task state, preserve only the durable transcript, show no stale
completion proof, and keep undo unavailable after the restored workspace has
been loaded.

## Bounded evidence contract

Record the manifest values, source SHA, source verification flags, runtime
identity, browser session and tab ownership, UTC timestamps, and direct
observations for each step. The fixture binds a supplied SHA to Go's compiled
`vcs.revision` when available and marks a dirty build with
`source_tree_modified:true`; `source_sha_verified:true` requires a matching
clean compiled revision. The probe content hash is included in the manifest;
the expected pre- and post-undo state is `absent`. Any provider-quality,
arbitrary hostile-writer, cross-platform-rendered, or unobserved field remains
`UNVERIFIED`. If no compiled revision or valid source SHA is available, the
manifest records `UNRECORDED` with `source_sha_verified:false`.

The fixture uses the existing `server.Handler`, `/api/permission`, `/api/chat`,
SSE events, task store, session store, checkpoint-backed undo, and fresh
`SetTaskSession` load. No fixture-only route bypasses the user-facing recovery
path.
