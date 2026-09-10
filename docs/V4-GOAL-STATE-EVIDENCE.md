# V4 durable goal-state confinement

Status: **bounded evidence contract implemented; hosted observation pending**.
This record belongs to [#632](https://github.com/saiaathishkarthik/picogent/issues/632)
under the broader runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It does not claim universal filesystem race resistance and does not upgrade
`hostile-filesystem-toctou` to `PASS`.

## Boundary

Goal state is stored below the configured Picogent home as a per-workspace
record, lock, and optional recovery backup. The subsystem now uses
`securefile.EnsureDir`, `OpenLockFile`/`LockFile`, anchored reads, atomic
publication, exclusive backup recovery, and inode/handle-safe cleanup. This
preserves monotonic revisions, tombstones, legacy migration, and
compare-and-clear behavior while rejecting symlink or reparse-point parents,
targets, and lock entries.

## Bounded protocol

`TestGoalHostileParentSwapConfinement` starts a separate same-UID helper
process. The helper repeatedly:

1. renames the trusted `goals` directory to a private backup;
2. presents a symlink on Unix or a junction on Windows at the trusted name,
   targeting a separate outside directory containing valid state and
   `outside-goal-marker` records;
3. records confirmed attacker activity; and
4. removes the reparse entry and restores the trusted directory.

Before the swap schedule begins, the test recovers a trusted backup so the
recovery path is exercised. During 256 bounded iterations, the victim performs
goal loads and writes while the helper cycles. A passing run must observe
trusted loads, writes, and recovery, must never load the outside marker, and
must leave the outside tree digest unchanged. Evidence mode writes only a
digest-only JSON record using schema
`picogent.v4.goal-state-persistence-evidence.v1`.

## Hosted execution

The Unix and Windows CI lanes run the focused test against the exact
pull-request source SHA and upload only the JSON record from the runner's
temporary directory. Windows probes junction support explicitly; if the
runner cannot create the required reparse fixture, evidence mode fails instead
of silently converting an unsupported host into a pass.

The eventual retained record must bind `candidate_sha` to the reviewed source,
set `source_tree_modified=false`, `verdict=PASS`, and retain
`broad_toctou_claim=UNVERIFIED`.

## Explicit limits

This protocol proves only the named goal-state persistence boundary under the
bounded helper schedule. It does not prove:

- safe behavior for every symlink, reparse-point type, filesystem, ACL, or
  sharing mode;
- protection of unrelated session, checkpoint, workspace, or project-rule
  pathname operations;
- resistance to every same-UID pathname race after every identity check;
- durable power-loss recovery beyond the existing atomic-write contract; or
- release authorization or a universal `hostile-filesystem-toctou=PASS` claim.

The runtime matrix and parent issue must continue to report the broad
`hostile-filesystem-toctou` row as `UNVERIFIED`.
