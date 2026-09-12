# v4 rendered cross-platform evidence at exact candidate `e770897`

Status: `PASS` for the rendered cross-platform recovery claim only. This
record belongs to [#704](https://github.com/saiaathishkarthik/picogent/issues/704)
under [#453](https://github.com/saiaathishkarthik/picogent/issues/453). It
does not authorize a release, upgrade the broad hostile-filesystem residual,
or claim v4 completion.

## Candidate and common flow

All three observations used the exact clean `main` candidate
`e770897891541c31d592220ae902c7fbc5b3db70`, the merge of PR #703, and the
same task-owned `rendered-recovery` fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the contained file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

Each platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, environment
`task-owned-disposable`, and verdict `PASS`. The retained records contain
bounded identity and SHA-256 values; no DOM, URL, credential, transcript, or
screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `c5236a06e5b084b129a07ab8c7b04e28961325d3edca35d6b2811395f4d2d142` | `d01978a88405b757f40d0c03b68c5e5bc38cdc0b36f39bc9f99e26b0eb8e6065` | `4675e3726a61b0a46d52e9acf385e84569a974b47021a3937445f839563ebb56` |
| `linux/amd64` | [hosted run 34661541355](https://github.com/saiaathishkarthik/picogent/actions/runs/34661541355) | `playwright-chromium-headless-134.0.6998.35` | `52a74e2da98b105494d5cac83e3de4fbda09a3a9d2092742563ebcae575ac75e` | `2058884e818c8668769d1e751b9d9f7d266869164475a828fcc1b7f1b3502c2f` | `c4ae740cc779f9ca12a1ad35077b1bde0010cb3f6a7b5f2a2b7cc40af795377c` |
| `windows/amd64` | [hosted run 34661541359](https://github.com/saiaathishkarthik/picogent/actions/runs/34661541359) | `chrome-headless-152.0.7977.83` | `f9d4d16313aeb8628ef7a98b45bd026dacf477e8f9f7e32890aaaeec84283ee1` | `1b2c675bf221dfb3c952e29f2df42302f0ddf58e9c6f9362d1815ef8fe1cdea1` | `c8f9af993bff3a683a835500346563916d6a9a9166a6fa85ec698f86296a28f8` |

All three platform records report `source_tree_modified=false` and the common
candidate SHA. The Darwin observation and both hosted collectors returned
`source_sha_verified=true`; the initial Darwin shared-cache build was rejected
by the fixture and is not used as evidence.

## Aggregate

The repository aggregate validator accepted exactly one valid record for each
required platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=e770897891541c31d592220ae902c7fbc5b3db70
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=6587cc334e340ea76e917be1f15f3724f54291e0c381627beb3f03f7aace57d9
```

The aggregate was produced by `cmd/rendered-cross-platform-evidence` from the
three platform records at the same candidate SHA. It retains digest-only
platform rows and does not contain screenshot bytes or provider data.

## Operator retention

The source checkout contains documentation only. The platform evidence and
aggregate artifacts were retained outside the checkout at:

```text
/private/tmp/picogent-v4-evidence-704/darwin/darwin-rendered-evidence-isolated/
/private/tmp/picogent-v4-evidence-704/linux/picogent-linux-rendered-evidence/
/private/tmp/picogent-v4-evidence-704/windows/
/private/tmp/picogent-v4-evidence-704/rendered-cross-platform-evidence.json
```

This closes the rendered cross-platform evidence gap for exact candidate
`e770897` only. A later non-documentation behavior change requires a fresh
three-platform collection. Broad same-UID filesystem TOCTOU, live-provider
quality, and release authorization remain outside this claim; no release or
v4-completion claim follows from this record.
