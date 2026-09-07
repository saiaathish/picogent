# Tip evidence rebound after `#551` (`f8a78c6`)

## Why rebound

`#551` squash-merged to `main` as
`f8a78c646877164009953401772acdb4d346175d` and changed non-docs paths:

- `internal/ctxmgr/retention.go`
- `internal/session/session.go`
- `docs/V4-PERFORMANCE-CAMPAIGN.md` (alloc before/after)

Behavior-SHA retention from `423d047…` is **invalid**. Docs tip `#550`
long-horizon evidence bound to `22b1b1b…` is **STALE** for this tip.

Alloc gains are **PROVED** by `#551` same-host before/after numbers in
`V4-PERFORMANCE-CAMPAIGN.md`. `#552` landed docs-only tip
`71c31cadf0538610bd1637621a028f9b4cbb4bda` packaging the tip cross-platform
PASS; retain behavior-bound rows with
`--behavior-sha f8a78c646877164009953401772acdb4d346175d`.

## Tip recollection completed (this package)

| Artifact | Path / digest | Verdict |
| --- | --- | --- |
| Darwin rendered recovery | `/private/tmp/picogent-evidence-f8a78c6/darwin/` · platform `7e3a0e1a…` | **PASS** |
| Linux rendered recovery | `/private/tmp/picogent-evidence-f8a78c6/linux/` · platform `69928933…` | **PASS** |
| Windows rendered recovery | `/private/tmp/picogent-evidence-f8a78c6/windows/` · platform `9c54b13f…` (GHA Run 12 `34112789982`) | **PASS** |
| Cross-platform aggregate | `…/aggregate/rendered-cross-platform-evidence.json` · `5123c9cc…` | **PASS** |
| Live provider connectivity + quality | [V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md](V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md) · `df823bd9…` / `2123344d…` | **PASS** |
| Exact-head full matrix (+ live + cross) | `…/aggregate/runtime-boundary-matrix-full.json` · `f0b4bca1…` | PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1 |
| Darwin long-horizon | `/private/tmp/picogent-evidence-f8a78c6/long-horizon/darwin/` · observation `3d1b2bde…` | **PASS** |

Details:

- [V4-RENDERED-RECOVERY-EVIDENCE-F8A78C6.md](V4-RENDERED-RECOVERY-EVIDENCE-F8A78C6.md)
- [V4-RENDERED-LINUX-EVIDENCE-F8A78C6.md](V4-RENDERED-LINUX-EVIDENCE-F8A78C6.md)
- [V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md](V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md)
- [V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md)
- [V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md](V4-LIVE-PROVIDER-EVIDENCE-F8A78C6.md)
- Long-horizon tip section in [V4-RENDERED-LONG-HORIZON-EVIDENCE.md](V4-RENDERED-LONG-HORIZON-EVIDENCE.md)

`rendered-cross-platform=PASS` and live-provider rows `PASS` at exact behavior
tip. Docs-only descendants retain with
`--behavior-sha f8a78c646877164009953401772acdb4d346175d`.

## Still open (not PASS)

- Operator residual acceptance / release authorization (human)
- `hostile-filesystem-toctou` remains UNVERIFIED by design
- `release-authorization=INCONCLUSIVE` without operator approval
