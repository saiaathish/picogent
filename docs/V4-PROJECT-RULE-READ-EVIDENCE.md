# V4 project-rule read confinement

Status: **PASS for the exact post-merge Darwin main observation; the Windows
record remains bound to the pre-merge pull-request SHA**.
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

## Post-merge exact-main observation

The focused campaign was rerun from a clean checkout at the post-merge `main`
tip after #629:

```text
source:                 167155027caafb40894bb49e46eff6ac5b6e0294
host:                   darwin/arm64
attempts:               256
successful_loads:       47
confirmed_attacker_swaps: true
outside_marker_observed: false
outside_before_sha256:  3efd9b5dbef128bbba2b62bb4549bca6b7b3abc8e9189d9f59c68311e277ef1f
outside_after_sha256:   3efd9b5dbef128bbba2b62bb4549bca6b7b3abc8e9189d9f59c68311e277ef1f
source_tree_modified:   false
verdict:                PASS
broad_toctou_claim:     UNVERIFIED
artifact_sha256:        1455f2ed98f03cd2d0f1f678f5467b46e205897d3270ebf4dfd6d49acc777cf1
observed:               2026-09-10, local exact-main run
```

The retained JSON record was written outside the checkout under
`/private/tmp/picogent-project-rule-evidence-1671550/`. It contains only the
schema, source identity, categorical counts, and digests; the outside rule
contents and helper output were not retained. The hosted PR run also passed
the Windows junction campaign at its exact review SHA, but that record is not
rebound to this later merge SHA.

## Hosted execution

The Unix and Windows CI lanes run the focused test against the exact
pull-request source SHA and upload only the JSON record from the runner's
temporary directory. Windows probes junction support explicitly; if the
runner cannot create the required reparse fixture, evidence mode fails instead
of silently converting an unsupported host into a pass.

Any later retained record must bind `candidate_sha` to its exact reviewed
source, set `source_tree_modified=false`, `verdict=PASS`, and retain
`broad_toctou_claim=UNVERIFIED`. The exact-main Darwin record above is the
current post-merge observation for this named boundary; it does not make the
stale global live-provider or rendered-platform packet current.

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
