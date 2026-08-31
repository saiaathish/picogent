# v4 GUI reconnect and recovery evidence

Status: rendered local evidence captured on 2026-08-31 against the focused
`codex/v4-gui-recovery-evidence` branch. This record is evidence for the GUI
history-reconciliation slice; it is not a release-readiness claim.

## Scope and setup

The probe ran from a clean worktree based on `main` at
`42b0b17b9a4ad7d89323d3a7ada684be2aa1a856` on macOS arm64 with Go 1.26.6.
The GUI used a disposable `PICOGENT_HOME` and `PICOGENT_CODEX_HOME`, bound to
`127.0.0.1:7421`, and was opened in a BrowserOS neo task-owned tab.

The provider was a local OpenAI-compatible stub that returned a fixed assistant
message. It was used only to create a durable rendered conversation without
using a real provider account. No live-provider quality or authentication claim
is attached to this evidence.

## Observed rendered behavior

1. A fresh tab loaded the GUI shell with the project selector, Safe/Fast
   permission controls, model picker, composer, Review/Changes/Activity tabs,
   and the initial “What should we work on?” surface.
2. Sending `Say exactly: rendered recovery probe.` rendered the user message,
   the stub assistant response, a `Completed` progress control, and a chat
   history entry. The browser-side `EventSource` was `OPEN` and the composer
   was enabled.
3. Stopping the GUI process left the project shell and existing transcript
   rendered, but the composer became disabled while the event stream was down.
4. Before the fix, restarting the server and creating a new session during the
   reconnect gap changed the browser’s session ID but left the old transcript
   rendered inside that new session. This was the concrete stale-history bug.
5. With the fix, the same sequence recovered with `EventSource.readyState=1`,
   `ready=true`, the new session ID, and an empty transcript. The prior chats
   remained in the history list, so recovery cleared only the stale in-session
   view.
6. A delayed active-turn probe forced a fresh `EventSource` while
   `clientBusy=true`; the submitted prompt remained visible during reconnect.
   When the stub response completed, `Slow reconnect probe completed.` rendered
   as the assistant turn. A full browser reload then showed both the prompt and
   assistant response with the composer enabled, confirming durable completion
   after the active-turn reconnect.

The final patched observation was read directly from the rendered page after a
five-second reconnect wait. It reported `sendDisabled=false`, an `OPEN`
EventSource, the new session ID, and `logText=""`; the history list still held
the two earlier completed conversations. The active-turn follow-up was also
reloaded from the server and retained both rendered turns.

## Implementation boundary

`refresh(true)` now runs when the native `EventSource` opens. The refresh also
detects a server-reported session change and replays the durable transcript (or
clears it when the new session has no messages). While the server is still busy
or a chat request is still being admitted, reconciliation marks a pending replay
so it does not erase the local prompt or partial assistant stream; the durable
transcript is replayed after the turn finishes. An idle server snapshot can
clear a stale client-busy flag once no request remains pending. Repeated busy
refreshes preserve the existing activity evidence. Session-view epochs plus
refresh generations prevent delayed session/project refreshes, including an
older reconnect refresh, from repainting a newer selection or restoring stale
busy state. Ordinary refreshes retain the existing empty-log fast path. The
embedded GUI test asserts that the reconnect refresh, session-change
reconciliation, pending active-turn replay, and async view guards remain wired
together.

Focused validation:

```text
go test ./internal/gui -count=1
ok   github.com/saiaathish/picogent/internal/gui  13.786s
```

## Limits and remaining gaps

- The provider was a local deterministic stub, not Codex or another live
  provider.
- The probe did not mutate files, request a permission decision, exercise an
  undoable change, or prove a full verification/recovery journey.
- The native folder-picker interaction was not treated as observed because a
  BrowserOS click produced no visible picker transition.
- This does not prove multi-hour stability, hosted cross-platform behavior, or
  release readiness. Those remain `UNVERIFIED` until separately observed.
