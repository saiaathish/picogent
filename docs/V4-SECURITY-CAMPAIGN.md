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
  outside-symlink use, root-ancestor rejection, and Unix ancestor-swap stress.
  Hosted Windows runtime evidence is still pending.
- Checkpoint capture, seal, and preflight fingerprint reads use the same
  secure opener. Restore staging, publication, deletion, and rollback remain
  path-based and are intentionally not covered by this checkpoint.

## Open or unrecorded risks

- Checkpoint restore now uses descriptor-relative secure writes and deletion,
  with in-memory post-turn states for best-effort rollback. The operation is
  intentionally not a multi-file atomic transaction; hostile-runtime restore
  stress and cross-platform deletion evidence remain required before this is a
  broad checkpoint safety claim.
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
