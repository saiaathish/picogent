# v4 rendered cross-platform evidence at exact candidate 592b07a

Status: PASS for the rendered cross-platform recovery claim only. This
record satisfies [#646](https://github.com/saiaathishkarthik/picogent/issues/646)
under the runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It does not
authorize a release, upgrade the hostile-filesystem residual, or claim v4
completion.

## Candidate and common flow

All three observations were bound to the exact clean candidate
592b07a633d9683354c4abeee09b767bc071ab35 and the same task-owned
rendered-recovery fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click Allow;
4. observe the one-file change and click Undo last change;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

Each platform record uses schema
picogent.v4.rendered-platform-evidence.v1, environment
task-owned-disposable, and verdict=PASS. The retained records contain
digests and bounded identity fields only; no DOM, URL, credential, transcript,
or screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| darwin/arm64 | local task-owned collection | playwright-chromium-headless-151.0.7922.34 | bfbfc4db9536bdd63287b2deae7227e4951f0f0f286351041e8baeaeacc4712c | 0dc13ab3a3bdc7b4c9c699fa44e5db8809a8a238b562a5e8633e6cad6e7572d2 | 936c8db5ba78026019ee5bac78b0f8cd10b55f6ef2e1eac1596c8f29321e1a45 |
| linux/amd64 | [hosted run 34555984326](https://github.com/saiaathish/picogent/actions/runs/34555984326) | playwright-chromium-headless-134.0.6998.35 | dce0fbd85f3f9243cfe73e8f00ed43792c0cb3bfd38e70ac5c8c8f221b402641 | 73f08f941ba6698c3d54f0b9c5cfc7f16d952cbd703313a578eb00c27654b5b6 | e94f945279964d36e8f1d2b34ff48c492283ddee4998cfb2e378816c3694382e |
| windows/amd64 | [hosted run 34555992474](https://github.com/saiaathishkarthik/picogent/actions/runs/34555992474) | chrome-headless-152.0.7977.83 | 3931d6de5f03162ca87c4b5747b09df3f596485dbaa69973fc5051ace9abc414 | 90c1b715768c3c11b475796e2ef9712c5b62285de9db5b44f7cab2d507ff3d4f | 4326dd87b4f901a027cc9c72aaf31d21f323ab1a967bf7511027146e5c37a2ba |

All three records report source_tree_modified=false and a clean exact
candidate. The local Darwin collector passed all 18 required checks, including
source verification, Safe permission, Allow, Undo, fresh-process reload, and
both fixture exits. The hosted Linux and Windows collectors completed the same
flow and uploaded their digest-only records and screenshots.

## Aggregate and current-tip matrix

Picogent's aggregator accepted exactly one valid record for each required
platform:

~~~
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=592b07a633d9683354c4abeee09b767bc071ab35
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=71bfe03c0afb889c9a548e9264cf0ab4509d80f69a85e346fb4501d85a626b05
~~~

The aggregate was retained outside the checkout at
/private/tmp/picogent-rendered-current-592-aggregate.json. Its observed
platform rows are Darwin/arm64, Linux/amd64, and Windows/amd64, each with
verdict=PASS and the common candidate SHA.

The current runtime-boundary matrix was generated from a clean checkout at
candidate 592b07a. Live-provider and local-platform records remain bound to
behavior candidate 1865a4c under the proven DOCS_ONLY_DESCENDANT continuity
rule; the cross-platform aggregate itself matches the current candidate
exactly:

~~~
candidate_sha=592b07a633d9683354c4abeee09b767bc071ab35
behavior_sha=1865a4c00356ac9b871b35b3a08f4e983f2af0e1
behavior_provenance=DOCS_ONLY_DESCENDANT
head_match=PASS
tree=CLEAN
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-platform-local=PASS
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
summary=PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1
runtime_matrix_sha256=6048c4244818db694b96b5875f8893f1d84aaf731a4caebc4493bfbff4213817
~~~

The retained matrix is
/private/tmp/picogent-current-592-runtime-boundary-matrix-final.json.
The current exact aggregate is intentionally kept separate from the
behavior-bound live/local records; no synthetic union is used.

## Operator retention

The platform records, screenshots, aggregate, and matrix remain outside the
checkout:

~~~
/private/tmp/picogent-rendered-current-592-darwin-clone/
/private/tmp/picogent-rendered-current-592-linux/
/private/tmp/picogent-rendered-current-592-windows/
/private/tmp/picogent-rendered-current-592-aggregate.json
/private/tmp/picogent-current-592-runtime-boundary-matrix-final.json
~~~

This closes the current rendered cross-platform evidence gap for candidate
592b07a only. A later non-documentation behavior change requires fresh
collection. Broad same-UID filesystem TOCTOU remains UNVERIFIED, and
release authorization remains INCONCLUSIVE until an operator records a
decision. No v4-completion or release claim follows from this record.
