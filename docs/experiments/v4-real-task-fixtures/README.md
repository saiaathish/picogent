# v4 real-task comparison fixtures

Status: bounded synthetic task catalog for issues [#807](https://github.com/saiaathish/picogent/issues/807) and [#811](https://github.com/saiaathish/picogent/issues/811). These fixtures define inputs and oracles; they do not measure either Picogent version and do not authorize a release.

No provider is called by this fixture package. In particular, preparing workspaces and running the deterministic Go/Python checks make no model or paid API calls. Live v3/v4 execution belongs to the separately scoped runner in #812 and must use the existing Codex login only.

## Isolation and reproduction

Each task's `workspace/` is the entire candidate project tree. The task prompt and allowed paths are in `manifest.json`. The tree digest covers every regular file by relative path and exact bytes; symlinks are rejected. `fixture.py verify` checks all three committed pristine trees without consulting the system temp directory. `fixture.py prepare` copies one verified tree into a fresh, unpredictable directory under the operating system's temporary directory on every invocation. The run ID is included in the directory name for tracking, not used as a shared path. POSIX creates it with mode `0700`; Windows inherits access controls from the configured temp directory. Do not use the repository, Desktop, or a user's project as a candidate workspace.

```sh
python3 docs/experiments/v4-real-task-fixtures/fixture.py verify
python3 docs/experiments/v4-real-task-fixtures/fixture.py prepare go-deadline --run-id fixture-smoke-01
```

`prepare` prints the absolute candidate workspace path. Run the task's acceptance command from that directory. Give every variant/repetition a distinct run ID for result tracking; every invocation also gets a new directory, so a repeated label cannot reuse or overwrite a candidate tree. Preserve each run until its result is scored, then remove only the exact run directory created for that run. The helper never deletes candidate data.

## Tasks

### Go debugging: inclusive deadline boundary

Prompt: **Fix `due.IsDue` so an item is due at its exact deadline as well as after it. An item before the deadline must remain not due. Keep the exported function signature and change only `due/due.go`. Run the existing tests.**

Allowed path: `due/due.go`.

Acceptance command (from the fixture workspace):

```sh
env GOMAXPROCS=1 GOMEMLIMIT=1GiB GOFLAGS=-p=1 GOTOOLCHAIN=local GOPROXY=off go test ./... -count=1
```

The table covers before, exact equality, after, and equivalent instants with different time-zone offsets. The bounded near-miss is retaining strict `After` semantics (equality remains false) or making every instant due (the before case fails).

### Frontend journey: persist a display name safely

Prompt: **Make the display-name form save a trimmed, non-empty name and restore it after reload. A whitespace-only submission must show an error and leave the last saved name unchanged. Keep the existing page and change only `app.js`; use browser storage, not a server or new dependency.**

Allowed path: `app.js`.

Start the disposable page from the fixture workspace:

```sh
python3 -m http.server 0 --bind 127.0.0.1
```

Use the ephemeral port printed by the server and BrowserOS neo (a real browser) at `http://<run-id>.localhost:<port>/` for the acceptance journey. The unique run ID hostname (under the reserved `.localhost` domain) plus the ephemeral port isolates browser storage by origin even if the operating system later reuses a port. Record both with that run:

1. Fill **Display name** with `  Mira Chen  ` and submit **Save**.
2. Confirm the status says `Saved Mira Chen.` and the field is normalized to `Mira Chen`.
3. Reload the page; confirm the field is still `Mira Chen`.
4. Submit three spaces; confirm `Display name cannot be blank.`.
5. Reload again; confirm the previously saved `Mira Chen` remains.

This is the real-browser oracle; merely serving the page, inspecting its source, or checking a mocked DOM is not a pass. The bounded near-miss is saving raw whitespace or allowing a blank submission to replace the persisted value. Record this lane `UNVERIFIED` if BrowserOS cannot reach the loopback page; do not substitute another browser or a DOM mock.

### Python modernization: single-pass event summary

Prompt: **Modernize `ledger.summarize` to aggregate events in one pass without materializing or mutating the input. Preserve first-seen category order and keep categories whose final total is zero. Change only `ledger.py` and run the tests.**

Allowed path: `ledger.py`.

Acceptance command (from the fixture workspace):

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v test_ledger.py
```

The one-shot iterable makes a second traversal fail deterministically. The bounded near-miss is omitting a category whose positive and negative events cancel to zero; the tests also check ordering and input immutability.

## Blind scoring rubric

Score candidate outputs as anonymous A/B, with candidate order randomized independently for each task/repetition. The scorer must not see the v3/v4 label until scores are recorded. Use the same rubric and task prompt for both candidates.

- **Correctness (0–2):** 0 neither the primary oracle nor near-miss passes; 1 only one passes; 2 both pass.
- **Unnecessary edits:** count changed paths outside the allow-list, and separately note in-scope edits unrelated to the requested behavior. Do not hide the raw counts in a single subjective score.
- **Unnecessary questions:** count questions already answered by the fixed prompt, fixture, or manifest. Questions about genuinely missing information are not penalized.
- **Repairs:** count post-first-attempt edit/retest cycles needed before both oracle checks pass; report the raw count.
- **Verification (0–2):** 0 no relevant check reported; 1 check is claimed but output/result is incomplete; 2 the exact oracle is run and its outcome is accurately reported. A command not run is not a pass.

Keep the raw diff, oracle output, question/repair counts, and browser verification state with each anonymous result. Do not infer success from plausible code or from one passing lane. These small synthetic tasks are a bounded comparison sample, not proof of general SWE quality.

## Local oracle smoke check

On 2026-10-11, all three pristine tree digests verified. The Go baseline failed the exact-deadline and equivalent-time-zone boundary cases; a one-line inclusive-boundary reference change passed the fixture tests. The Python baseline failed because it traversed a one-shot event stream more than once; a one-pass accumulator passed both tests, including zero-total preservation and input immutability. Neither command called a provider.

The frontend baseline was served from a prepared temporary copy and exercised in BrowserOS neo at loopback: submitting the padded name and reloading restored the default `Ada Lovelace`, not the submitted value. A separate pristine copy with a local reference change passed the real-browser journey at ephemeral port `51168`: normalized save, reload restore, whitespace-only rejection, and reload preservation of the previous value. This is only a fixture-oracle check, not a v3/v4 run or product-quality result.

## Fixture integrity

`manifest.json` binds each task to its pristine tree digest, fixed prompt, allowed paths, and oracle. Recompute the digests after any intentional fixture edit and review that edit as a task-definition change; do not silently update a digest to make verification green. A modified candidate's digest is expected to differ from the pristine digest and must not be reused for another variant or repetition.

## Live v3/v4 runner (#812)

Run from a terminal on macOS so the browser lane can pause for a real BrowserOS check:

```sh
python3 scripts/v4_live_task_runner.py --live --repetitions 2
```

Use `--task go-deadline` (repeatable) to bound a run to selected fixtures. The runner builds the exact pinned source worktrees serially with one Go build worker, one `GOMAXPROCS`, and a 1 GiB runtime memory limit. Before any provider request it verifies the fixture digests, source availability, existing Codex access-token expiry for the full configured run budget, and macOS sandbox canaries.

Authentication uses the existing ChatGPT Codex login only. The temporary private `PICOGENT_CODEX_HOME/auth.json` contains only the current access token and account ID; the refresh token and ID token are never copied. The model process can read that per-run auth copy, cannot read the host `~/.codex` directory, and cannot write either credential directory. Shell children can neither read nor write either Codex directory. Shell commands run through a nested sandbox that also denies network access and restricts persistent writes to the task-owned temporary root (with `/dev/null` allowed for ordinary redirection). The model process alone receives outbound network and a single `com.apple.trustd.agent` lookup required by Go's macOS TLS verification. No API key or paid API endpoint is used.

The artifact keeps hashes, paths, exit/acceptance status, observed latency, tool-start markers, and a coarse error category only; raw model/provider output and auth data are not retained. Picogent's exact known runtime metadata paths under `.picogent/` are listed separately from candidate file changes; any other out-of-allowlist path remains a failure. Provider HTTP request counts, actual served model, and token totals are `UNVERIFIED` because the CLI does not persist those per-run values. The tool-start threshold is a reactive stop signal—not a hard pre-dispatch call cap—so that dimension also remains `UNVERIFIED`; do not report an overall `PASS` from this runner. Subjective question/repair scoring is not implemented and remains `UNVERIFIED` rather than being reported as complete.

The browser lane uses a unique `<run-id>.localhost` origin and requires the exact real-browser journey above. Entering anything other than `PASS`/`FAIL`, running without an interactive terminal, or timing out leaves that lane `UNVERIFIED`; no DOM mock or alternate browser is accepted. An optional `--output <new-path>` writes only the sanitized JSON artifact with mode `0600` and refuses to overwrite an existing file.
