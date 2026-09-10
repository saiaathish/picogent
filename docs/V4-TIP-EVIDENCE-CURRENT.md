# Current V4 evidence and continuity

Stable entrypoint for the latest audited main candidate
`8b022e82be4b2e8ade0a502c5f373eaf6dc61160`:

- Rendered exact-candidate packet:
  [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-8B022E8.md).
- Live-provider continuity packet:
  [V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md](V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md).
- Underlying live behavior observation:
  [V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md](V4-LIVE-PROVIDER-EVIDENCE-B7B5C5C.md).

The rendered packet is bound to behavior SHA `8b022e8`; the live-provider
continuity packet validates retained artifacts from behavior SHA `b7b5c5c`
through a `DOCS_ONLY_DESCENDANT` relationship. These are separate matrices and
must not be unioned into a synthetic release-authorizing result. A later
non-documentation behavior change requires fresh evidence; a later main tip
also needs its own continuity matrix before being described as current.
Hostile-runtime and release-authorization claims remain separately bounded.
