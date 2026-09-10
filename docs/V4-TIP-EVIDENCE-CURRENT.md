# Current V4 live-provider evidence and continuity

Stable entrypoint for the current exact-head live-provider packet:
[V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).
The post-merge continuity check is recorded in
[V4-LIVE-PROVIDER-CONTINUITY-DB477D1.md](V4-LIVE-PROVIDER-CONTINUITY-DB477D1.md).

The linked packet is bound to behavior SHA
`b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5`. Current merged `main` is
`db477d17ab0f374e5cc9ae473a0613744023743e`, a verified docs-only descendant;
the continuity record confirms that the retained live-provider observations
remain valid at that tip without rebinding their provenance. Rendered,
hostile-runtime, and release-authorization claims remain separately bounded.
This entrypoint is updated only by a later evidence-refresh commit and never
projects an older packet onto a new behavior candidate.
