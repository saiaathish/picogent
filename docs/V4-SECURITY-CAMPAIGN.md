# V4 security campaign

Status: active. This is a bounded manual audit record, not a hosted security
certification or a claim that every hostile-runtime scenario is closed.

## Confirmed hardening

### Network fetch

`web_fetch` previously validated a hostname with DNS and then handed the URL to
the default HTTP transport, which could resolve the hostname again. That left a
DNS-rebinding window between validation and connection.

The focused fix resolves and validates every answer again inside the dialer,
dials the validated IP directly, rejects any mixed public/private answer set,
and disables environment proxy routing for this guarded request. Redirects are
validated independently and use the same dialer.

Coverage on the security branch:

- mixed public/private DNS answers are rejected;
- the dial target is an IP address rather than the hostname;
- private and local literal targets remain rejected;
- `FuzzWebFetchIPBoundary` passed for a two-second local run.

### Prompt trust boundaries

Repository rules, learned memory, and installed skill text are now framed as
untrusted advisory content. The system prompt explicitly keeps system policy,
the user's request, permission gates, and live tool evidence authoritative.
Instruction-like text from those sources cannot authorize secrets, risky
actions, permission bypasses, or unrelated edits. The boundary is covered by a
prompt-construction test.

### Installer safety

The first-run installer no longer runs remote shell text. Codex and Claude
installation uses only compile-time allowlisted package names from the
explicit `https://registry.npmjs.org/` registry, a private
`~/.picogent/tools` prefix, isolated npm config/cache files, and
`--ignore-scripts`, `--no-audit`, and `--no-fund`. The package manager receives
an explicit allowlisted environment rather than the Picogent process's API
keys, auth variables, loader hooks, npm configuration overrides, or ambient
PATH entries. The resolved package manager and provider binaries must come
from known runtime prefixes, and automatic installation refuses an elevated
Unix process or elevated Windows token. Interactive provider login also refuses
to launch from an elevated Picogent process, including when a provider was
already installed. Installer output is capped and credential-shaped values are
redacted before it reaches setup logs.

Managed provider lookup also rejects symlinked tools roots and checks every
ancestor on the path before accepting an installed binary. Windows login
launches pass the validated absolute `cmd.exe` path and disable Command
Processor AutoRun for both command interpreters. The synchronous `picogent
login` route uses the same validated executable and restricted environment, and
installer `PATH` contains only the resolved package-manager and Node runtime
directories; XFCE terminal commands quote the validated Bash path.

OpenCode and Antigravity are now manual-install providers: setup reports their
official documentation locations and login accepts only an argv-shaped,
already-resolved CLI command. No `curl | bash` path or automatic Homebrew
fallback remains. Deterministic hostile tests cover the npm argument boundary,
private prefix, environment filtering, output redaction/capping, and the
absence of remote provider shell installation.

### Verification subprocess boundary

Workspace verification commands now receive the shared sanitized subprocess
environment. Credential-shaped variables, shell startup files, dynamic-loader
hooks, pager settings, and package-manager overrides are removed before the
verifier starts a workspace command. A fresh-process regression test confirms
that a token is not inherited while valid passing-test evidence remains
observable. This is a local boundary test, not proof that every external
provider or hostile runtime path is safe.

The atomic writer's same-UID replacement race after its final identity check
remains deliberately unclaimed because POSIX has no portable
compare-and-rename-by-inode primitive. Deterministic `securefile` tests cover
the supported boundary: a replacement observed before publication is rejected,
and cleanup never unlinks a replacement inode.

## Existing boundaries rechecked

- Workspace MCP configuration is not autoloaded; only user-owned MCP config is
  loaded because connecting a server can execute a command or contact a URL
  before an individual MCP tool reaches the permission gate.
- MCP subprocesses receive a small inherited environment plus explicitly
  configured values; ordinary shell execution filters credential and loader
  variables.
- Static symlink escapes are rejected by permission classification, built-in
  file tools, and checkpoint capture. `FuzzResolveWorkspacePathBoundary` passed
  for a two-second local run.
- `read_file`, `write_file`, and `edit_file` now perform their actual I/O
  through a secure workspace opener. Unix builds walk every root and child
  component through directory descriptors with `openat`/`mkdirat` and
  `O_NOFOLLOW`; Windows builds use `NtCreateFile` RootDirectory handles with
  `OBJ_DONT_REPARSE` and `FILE_OPEN_REPARSE_POINT`. Focused tests cover direct
  outside-symlink use, hard-linked files, root-ancestor rejection, and Unix
  ancestor-swap stress.
  Hosted CI run `33304380869` passed the Windows workspace tests, build, and
  GUI smoke at source commit `ff91cf3`; this is direct hosted runtime evidence
  for that path, not proof of Windows ACL enforcement or hostile races.
- Checkpoint capture, seal, preflight fingerprint reads, restore publication,
  deletion, and rollback use the secure workspace primitives. Each restored
  file's complete bytes and mode are published atomically; the sequence across
  multiple files is intentionally not a multi-file transaction, and hostile
  same-UID pathname races remain outside the helper's guarantee.
- A Unix checkpoint stress test now exercises both restore writes and deletion
  while an ancestor directory is repeatedly replaced; outside sentinel files
  remain unchanged. This is local hosted-Linux evidence, not Windows hostile
  runtime evidence or a general same-UID race guarantee.
- Secure opens, atomic writes, and removals reject regular files with multiple
  hard links on Unix and Windows. Checkpoint restore also rejects a post-seal
  replacement hard link before mutating any path; this prevents a workspace
  name from being used to read or write an outside inode through a hard link.
- A cross-platform checkpoint test also rejects a post-seal replacement hard
  link at a turn-created path before deletion; both the outside sentinel and
  the replacement path remain unchanged.
- A Windows-only stress test races checkpoint restore against repeated
  replacement hard links and verifies that an outside sentinel remains
  unchanged. Hosted PR #132 CI run `33310394597` and post-merge `main` run
  `33310553558` passed the Windows, Ubuntu, macOS, and release matrix. This is
  direct hosted Windows stress evidence, not a general same-UID race guarantee.

## Hosted evidence ledger

These merged slices have recorded source, merge, pull-request CI, and
post-merge CI provenance. Every listed CI run passed Ubuntu, Windows, macOS,
and release evidence. This ledger records delivery evidence; it does not
close the open or unrecorded risks below.

| PR | Slice | Source | Merge | PR CI | Post-merge CI |
| --- | --- | --- | --- | --- | --- |
| #127 | Mode-aware atomic workspace writes | `76b5e06` | `8746d56` | `33308318250` | `33308456626` |
| #128 | Atomic checkpoint restore publication and rollback reporting | `59787da` | `1e6188e` | `33308507232` | `33308634597` |
| #129 | Unix ancestor-swap deletion stress coverage | `ec22494` | `d77a4f8` | `33308942589` | `33309050782` |
| #130 | Cross-platform hardlink protection for created-path deletion | `aba55a5` | `b2fd72f` | `33309325110` | `33309483503` |
| #131 | Security evidence ledger | `96ac210` | `703016d1` | `33309894324` | `33310008305` |
| #132 | Windows hostile-runtime restore stress coverage | `b7f612b` | `d1572a2` | `33310394597` | `33310553558` |
| #133 | Recorded Windows restore stress evidence | `5d4d695` | `2d57d76` | `33310730875` | `33310844052` |
| #135 | Transient Windows session replacement retry | `be5b6db` | `556da3e` | `33311631871` | `33311763251` |

## Open or unrecorded risks

- Checkpoint restore now publishes each restored file's complete bytes and mode
  atomically and only marks a mutation applied after that publication or
  deletion succeeds. Rollback still uses in-memory post-turn states and is
  best-effort. Hosted Windows hostile-runtime restore stress now covers
  replacement-hardlink pressure, but the operation is intentionally not a
  multi-file atomic transaction and cross-process deletion races remain
  required before this is a broad checkpoint safety claim.
- Trace events clip values but do not provide a complete secret-redaction
  policy for prompts, tool arguments, MCP output, or crash diagnostics.
- The npm provider packages and their platform-specific optional packages are
  now backed by the reviewed `internal/setup/provider-package-lock.json`; setup
  materializes that lock and uses npm's integrity-checked lock resolution with
  lifecycle scripts disabled. A pre-existing project `.npmrc` in the managed
  prefix is rejected so scoped registries and transport settings cannot silently
  override the reviewed install policy. This is provenance evidence, not a
  signed release attestation or a complete external SBOM, and it remains
  dependent on the trusted npm/Node client and registry transport.
- Managed and external executable paths are canonicalized and revalidated at
  launch. This rejects path changes observed before process start, but the
  portable path-based launcher cannot eliminate an OS-level replacement race
  between its final check and `execve`/`CreateProcess`; live hostile runtime
  coverage for that gap remains open. Unix lookup also rejects writable
  ancestors except protected sticky system temporary directories; Windows ACL
  enforcement remains unverified.
- macOS Terminal.app launch now prefixes the provider command with an explicit
  `/usr/bin/env -i` allowlist, but Terminal profile startup and Apple Event
  behavior remain live-runtime proof gaps.
- Git status/diff and external MCP responses need adversarial hook, textconv,
  prompt-injection, and secret-leakage runtime tests.
- The hosted deep security scan was unavailable in the earlier campaign; no
  scan ID, manifest, or no-findings result is claimed.

The v4 release gate must keep these items visible and must not convert the
confirmed tests above into a general security-ready claim.
