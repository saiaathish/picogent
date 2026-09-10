# V4 Windows retained-artifact read confinement

Status: **bounded evidence contract implemented; hosted observation pending**.
This record belongs to [#626](https://github.com/saiaathishkarthik/picogent/issues/626)
under the broader runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It does not claim universal Windows filesystem race resistance and does not
upgrade `hostile-filesystem-toctou` to `PASS`.

## Bounded protocol

The focused Windows test starts a separate same-UID helper process. The helper
repeatedly:

1. renames the trusted artifact parent to a private backup;
2. presents a junction at the trusted parent name that targets a separate
   outside directory containing a valid candidate-matching artifact with an
   `outside-marker` reason;
3. records confirmed attacker activity;
4. removes the junction and restores the trusted parent.

The victim performs a bounded sequence of `LoadReport` calls while the helper
cycles. A successful load must never return the outside marker, at least one
trusted in-tree load must succeed, and the outside artifact digest must be
unchanged. Junction creation is probed explicitly; evidence mode fails if the
host cannot create the required reparse fixture instead of silently turning an
unsupported host into a pass.

The test is `TestWindowsSameUIDRetainedArtifactReadConfinement` in
`internal/runtimeboundary/retain_windows_test.go`. Normal package tests do not
retain files or start the helper. Evidence mode writes only the digest-only
JSON record specified by schema
`picogent.v4.hostile-retained-artifact-read-evidence.v1`.

## Hosted execution

The Windows CI lane runs:

```sh
PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_SOURCE_SHA="$CANDIDATE_SHA" \
PICOGENT_HOSTILE_RETAIN_WINDOWS_LOAD_EVIDENCE_OUT="$EVIDENCE_DIR/retained-artifact-read.json" \
  go test -v ./internal/runtimeboundary \
    -run '^TestWindowsSameUIDRetainedArtifactReadConfinement$' -count=1
```

Only the bounded JSON record is uploaded from the runner's temporary evidence
directory. The eventual hosted record must bind `candidate_sha` to the exact
pull-request source SHA and retain `broad_toctou_claim=UNVERIFIED`.

## Explicit limits

This protocol proves only that the current Windows read path rejects the named
junction/reparse replacement under the bounded helper schedule. It does not
prove:

- resistance to a regular-directory replacement, which requires a caller-
  supplied trusted parent identity that `LoadReport` does not currently take;
- behavior for every Windows reparse-point type, filesystem, ACL, or sharing
  mode;
- arbitrary same-UID pathname races after every identity check;
- safety of other Picogent pathname surfaces;
- release authorization or a universal `hostile-filesystem-toctou=PASS` claim.

The runtime matrix and the parent issue must continue to report the broad
`hostile-filesystem-toctou` row as `UNVERIFIED`.
