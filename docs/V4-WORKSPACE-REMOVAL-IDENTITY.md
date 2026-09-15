# V4 workspace removal identity boundary

Issue [#750](https://github.com/saiaathish/picogent/issues/750) narrows the
workspace removal boundary without claiming that hostile same-UID filesystem
races are solved universally.

`Remove` and `RemoveIfUnchanged` keep the regular-file handle used for the
initial check alive through the final deletion decision. The operation compares
that opened identity with the current final entry through the descriptor- or
handle-anchored parent. A changed or missing identity returns the stable
`workspace.ErrTargetChanged` conflict and leaves the replacement in place.

On Unix, the final `unlinkat` is still a pathname operation after the identity
comparison. On Windows, deletion uses the identity-checked delete-capable
handle. Both paths continue to reject symlinks, reparse points, non-regular
entries, and hard-linked files.

This is bounded removal hardening and deterministic replacement-preservation
coverage. The broader `hostile-filesystem-toctou` claim remains
**UNVERIFIED**, and this slice does not close parent issue #453 or authorize a
v4 release.
