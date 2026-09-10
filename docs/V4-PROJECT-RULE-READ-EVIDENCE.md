# V4 project-rule read confinement

Status: **bounded evidence contract implemented; hosted observation pending**.
This record belongs to [#628](https://github.com/saiaathishkarthik/picogent/issues/628)
under the broader runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It does not claim universal filesystem race resistance and does not upgrade
`hostile-filesystem-toctou` to `PASS`.

## Boundary

Picogent reads `AGENTS.md`, `CLAUDE.md`, and `.picogent/rules.md` from the
selected workspace before constructing the model context. The loader now uses
`securefile.ReadFilePrefix`, which opens each parent through the shared
descriptor/handle-anchored boundary, rejects symlink or reparse-point entries,
and reads no more than the existing 24 KiB prefix. Missing, non-regular, and
reparse-backed entries remain omitted as they were for other unreadable files.

## Bounded protocol

`TestProjectRuleHostileParentSwapConfinement` starts a separate same-UID helper
process. The helper repeatedly:

1. renames the trusted `.picogent` directory to a private backup;
2. presents a symlink on Unix or a junction on Windows at the trusted name,
   targeting a separate outside directory containing an
   `outside-project-rule-marker`;
3. records confirmed attacker activity; and
4. removes the reparse entry and restores the trusted directory.

The victim performs 256 bounded `Load` calls while the helper cycles. A
successful load must never return the outside marker, at least one trusted
in-tree load must succeed, the helper must confirm a swap, and the outside
file digest must remain unchanged. Evidence mode writes only a digest-only JSON
record using schema `picogent.v4.project-rule-read-evidence.v1`.

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

This protocol proves only the named project-rule read boundary under the
bounded helper schedule. It does not prove:

- safe behavior for every symlink, reparse-point type, filesystem, ACL, or
  sharing mode;
- protection of unrelated session, goal, checkpoint, or workspace pathname
  operations;
- resistance to every same-UID pathname race after every identity check;
- model safety for arbitrary instruction content once a legitimate rule file
  has been read; or
- release authorization or a universal `hostile-filesystem-toctou=PASS` claim.

The runtime matrix and parent issue must continue to report the broad
`hostile-filesystem-toctou` row as `UNVERIFIED`.
