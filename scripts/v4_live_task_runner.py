#!/usr/bin/env python3
"""Run bounded, digest-only live task comparisons against exact Picogent SHAs.

Live calls are opt-in. Only the existing Codex access token and account ID are
copied into private per-run state; refresh credentials and provider output are
never persisted. Model requests run in the disposable root, while shell tools
run under a nested no-network sandbox that cannot read either Codex auth file.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import secrets
import select
import selectors
import shlex
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import time
from typing import BinaryIO, TextIO


REPO_ROOT = pathlib.Path(__file__).resolve().parents[1]
FIXTURE_DIR = REPO_ROOT / "docs" / "experiments" / "v4-real-task-fixtures"
V3_SHA = "a07943b31044049afb0142f39198244cd3c75218"
V4_SHA = "c9a77f0e09c2ec7b45369a9cd08359079fa35f0f"
DEFAULT_MODEL = "gpt-5.6-luna"
TEMP_PREFIX = "picogent-v4-live-task-"
ROOT_MARKER = ".picogent-live-task-root"
MAX_OUTPUT_BYTES = 4 * 1024 * 1024
MAX_LINE_BYTES = 64 * 1024
MAX_ERROR_CLASSIFICATION_BYTES = 8 * 1024


class RunnerError(RuntimeError):
    """A sanitized runner failure; never includes provider output or secrets."""


def load_fixture_module():
    fixture_path = FIXTURE_DIR / "fixture.py"
    if fixture_path.is_symlink() or not fixture_path.is_file():
        raise RunnerError("task fixture helper is unavailable")
    spec = importlib.util.spec_from_file_location("picogent_v4_task_fixtures", fixture_path)
    if spec is None or spec.loader is None:
        raise RunnerError("task fixture helper could not be loaded")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_digest(value: object) -> str:
    raw = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("utf-8")
    return sha256_bytes(raw)


def sbpl_quote(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def sandbox_profile(
    write_root: pathlib.Path,
    *,
    denied_reads: tuple[pathlib.Path, ...] | list[pathlib.Path] = (),
    denied_writes: tuple[pathlib.Path, ...] | list[pathlib.Path] = (),
    allow_network: bool = True,
    allow_macos_trust: bool = False,
) -> str:
    root = write_root.resolve(strict=True)
    if not root.is_dir() or root.is_symlink():
        raise RunnerError("sandbox write root is not a real directory")
    if allow_macos_trust and not allow_network:
        raise RunnerError("macOS trust lookup requires network access")
    rules = ["(version 1)", "(deny default)"]
    for path in sorted({pathlib.Path(item).resolve(strict=False) for item in denied_reads}):
        # SBPL uses first-match semantics: sensitive-file denies must precede
        # the broad read allowance needed by the local toolchain.
        selector = "subpath" if path.is_dir() else "literal"
        rules.append(f"(deny file-read* ({selector} {sbpl_quote(str(path))}))")
    rules.extend(
        (
            "(allow file-read*)",
            f"(allow file-write* (literal {sbpl_quote('/dev/null')}))",
            f"(allow file-write* (subpath {sbpl_quote(str(root))}))",
            "(allow process*)",
        )
    )
    # Write denials must follow the broad root allowance on this macOS policy.
    for path in sorted({pathlib.Path(item).resolve(strict=False) for item in denied_writes}):
        selector = "subpath" if path.is_dir() else "literal"
        rules.append(f"(deny file-write* ({selector} {sbpl_quote(str(path))}))")
    if allow_network:
        if allow_macos_trust:
            rules.append('(allow mach-lookup (global-name "com.apple.trustd.agent"))')
        rules.append("(allow network*)")
    rules.append("(allow sysctl-read)")
    return "\n".join(rules)


def safe_relative_paths(paths: list[str]) -> list[str]:
    normalized: list[str] = []
    for raw in paths:
        path = pathlib.PurePosixPath(raw)
        if path.is_absolute() or not path.parts or ".." in path.parts or "\\" in raw:
            raise RunnerError("fixture contains an unsafe allowed path")
        normalized.append(path.as_posix())
    if len(normalized) != len(set(normalized)):
        raise RunnerError("fixture contains duplicate allowed paths")
    return normalized


def file_digests(root: pathlib.Path) -> dict[str, str]:
    root = pathlib.Path(root)
    if root.is_symlink():
        raise RunnerError("workspace root became a symlink")
    root = root.resolve(strict=True)
    result: dict[str, str] = {}
    for path in sorted(root.rglob("*"), key=lambda item: item.relative_to(root).as_posix()):
        if path.is_symlink():
            raise RunnerError("workspace contains a symlink")
        if path.is_file():
            result[path.relative_to(root).as_posix()] = sha256_bytes(path.read_bytes())
        elif not path.is_dir():
            raise RunnerError("workspace contains an unsupported filesystem entry")
    return result


def changed_paths(before: dict[str, str], after: dict[str, str]) -> list[str]:
    return sorted(path for path in before.keys() | after.keys() if before.get(path) != after.get(path))


def is_runtime_metadata_path(path: str) -> bool:
    exact = {
        ".picogent/project-run/.run.lock",
        ".picogent/workspace-instance",
        ".picogent/workspace-instance.lock",
    }
    return path in exact or re.fullmatch(r"\.picogent/undo/headless-[0-9a-f]{16}\.json", path) is not None


def source_state(repo: pathlib.Path, expected_sha: str) -> dict[str, object]:
    head = subprocess.run(
        ["git", "-C", str(repo), "rev-parse", "HEAD"],
        capture_output=True,
        check=False,
        timeout=10,
    )
    status = subprocess.run(
        ["git", "-C", str(repo), "status", "--porcelain=v1", "--untracked-files=all"],
        capture_output=True,
        check=False,
        timeout=10,
    )
    if head.returncode != 0 or status.returncode != 0:
        raise RunnerError("could not verify a pinned source worktree")
    actual = head.stdout.decode("ascii", errors="replace").strip()
    clean = status.stdout == b""
    return {"expected_sha": expected_sha, "head": actual, "head_matches": actual == expected_sha, "clean": clean}


def run_checked(argv: list[str], *, cwd: pathlib.Path, env: dict[str, str], timeout: int) -> None:
    try:
        result = subprocess.run(
            argv,
            cwd=cwd,
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
            timeout=timeout,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise RunnerError(f"command could not complete: {pathlib.Path(argv[0]).name}") from error
    if result.returncode != 0:
        raise RunnerError(f"command failed: {pathlib.Path(argv[0]).name} exit={result.returncode}")


def limited_go_env(root: pathlib.Path, module_cache: pathlib.Path) -> dict[str, str]:
    home = root / "home"
    tmp = root / "tmp"
    cache = root / "go-cache"
    for directory in (home, tmp, cache):
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    env = {
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "HOME": str(home),
        "TMPDIR": str(tmp),
        "LANG": "C.UTF-8",
        "GOMAXPROCS": "1",
        "GOMEMLIMIT": "1GiB",
        "GOFLAGS": "-p=1",
        "GOTOOLCHAIN": "local",
        "GOPROXY": "off",
        "GOSUMDB": "off",
        "GOCACHE": str(cache),
        "GOMODCACHE": str(module_cache),
        "PYTHONDONTWRITEBYTECODE": "1",
    }
    return env


def run_env(root: pathlib.Path, model: str, module_cache: pathlib.Path) -> dict[str, str]:
    env = limited_go_env(root, module_cache)
    env.update(
        {
            "PICOGENT_HOME": str(root / "picogent-home"),
            "PICOGENT_CODEX_HOME": str(root / "codex-home"),
            "PICOGENT_PROVIDER": "codex",
            "PICOGENT_MODEL": model,
            "PICOGENT_ROUTER": "off",
        }
    )
    # Deliberately do not pass CODEX_HOME or any provider API-key variable.
    return env


def read_codex_credential(source: pathlib.Path, *, minimum_validity_seconds: int) -> dict[str, object]:
    try:
        info = source.lstat()
        if not source.is_file() or source.is_symlink() or info.st_size <= 0 or info.st_mode & 0o077:
            raise RunnerError("existing Codex login file is unavailable")
        payload = json.loads(source.read_text(encoding="utf-8"))
        tokens = payload.get("tokens") if isinstance(payload, dict) else None
        access_token = tokens.get("access_token") if isinstance(tokens, dict) else None
        account_id = tokens.get("account_id") if isinstance(tokens, dict) else None
        if not isinstance(access_token, str) or not access_token or not isinstance(account_id, str):
            raise RunnerError("existing Codex login lacks the required access credential")
        parts = access_token.split(".")
        if len(parts) < 2:
            raise RunnerError("existing Codex access credential has no verifiable expiry")
        encoded = parts[1]
        claims = json.loads(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))
        expires_at = claims.get("exp") if isinstance(claims, dict) else None
        if not isinstance(expires_at, (int, float)):
            raise RunnerError("existing Codex access credential has no verifiable expiry")
        remaining = int(expires_at - time.time())
        if remaining < minimum_validity_seconds:
            raise RunnerError("existing Codex access credential expires before the bounded run can finish")
        return {
            "access_token": access_token,
            "account_id": account_id,
            "expires_at": int(expires_at),
        }
    except RunnerError:
        raise
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError, TypeError) as error:
        raise RunnerError("existing Codex login file is unavailable or invalid") from error


def write_access_only_auth_file(credential: dict[str, object], destination: pathlib.Path) -> None:
    destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with destination.open("x", encoding="utf-8") as output:
        json.dump(
            {
                "tokens": {
                    "access_token": credential["access_token"],
                    "account_id": credential["account_id"],
                }
            },
            output,
            separators=(",", ":"),
        )
    os.chmod(destination, 0o600)
    if destination.is_symlink() or not destination.is_file() or destination.stat().st_mode & 0o077:
        raise RunnerError("private Codex login copy failed its permission check")


def host_codex_auth_path(home: pathlib.Path | None = None) -> pathlib.Path:
    home = pathlib.Path.home() if home is None else pathlib.Path(home)
    auth_dir = home / ".codex"
    try:
        mode = auth_dir.lstat().st_mode
    except OSError as error:
        raise RunnerError("existing Codex login directory is unavailable") from error
    if stat.S_ISLNK(mode) or not stat.S_ISDIR(mode):
        raise RunnerError("existing Codex login directory must be a real directory")
    auth_path = auth_dir / "auth.json"
    if auth_path.is_symlink():
        raise RunnerError("existing Codex login file must not be a symlink")
    return auth_path


def probe_sandbox(
    write_root: pathlib.Path,
    *,
    env: dict[str, str],
    sandbox_exec: str,
    denied_reads: tuple[pathlib.Path, ...] | list[pathlib.Path] = (),
    denied_writes: tuple[pathlib.Path, ...] | list[pathlib.Path] = (),
) -> bool:
    profile = sandbox_profile(write_root, denied_reads=denied_reads, denied_writes=denied_writes)
    inside = write_root / ".containment-probe" / "allowed"
    outside = write_root.parent / f"{write_root.name}-outside-probe-{secrets.token_hex(5)}"
    inside.parent.mkdir(mode=0o700)
    if outside.exists() or outside.is_symlink():
        raise RunnerError("sandbox probe target unexpectedly exists")
    command = f'touch "{inside}" && touch "{outside}"'
    result = subprocess.run(
        [sandbox_exec, "-p", profile, "/bin/sh", "-c", command],
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
        timeout=10,
    )
    allowed = inside.is_file()
    denied = outside.exists() or outside.is_symlink()
    if allowed:
        inside.unlink()
        inside.parent.rmdir()
    if not (allowed and not denied and result.returncode != 0):
        raise RunnerError("macOS sandbox did not prove the exact write boundary")
    return True


def shell_tool_env(env: dict[str, str]) -> dict[str, str]:
    """Build the minimal environment passed from a shell wrapper to bash."""
    child = {
        key: env[key]
        for key in (
            "HOME",
            "TMPDIR",
            "LANG",
            "GOMAXPROCS",
            "GOMEMLIMIT",
            "GOFLAGS",
            "GOTOOLCHAIN",
            "GOPROXY",
            "GOSUMDB",
            "GOCACHE",
            "GOMODCACHE",
            "PYTHONDONTWRITEBYTECODE",
        )
        if key in env
    }
    path_dirs = ["/usr/bin", "/bin", "/usr/sbin", "/sbin"]
    for command in ("go", "git", "python3", "rg", "node", "npm", "make", "clang"):
        located = shutil.which(command)
        if located:
            path_dirs.extend((str(pathlib.Path(located).parent), str(pathlib.Path(located).resolve().parent)))
    child["PATH"] = os.pathsep.join(dict.fromkeys(path_dirs))
    return child


def write_shell_wrapper(
    wrapper: pathlib.Path,
    *,
    sandbox_exec: str,
    profile_path: pathlib.Path,
    env: dict[str, str],
) -> None:
    """Route every Picogent bash tool call through a stricter nested sandbox."""
    if wrapper.exists() or wrapper.is_symlink():
        raise RunnerError("shell sandbox wrapper target unexpectedly exists")
    assignments = " \\\n".join(
        f"  {shlex.quote(key + '=' + value)}" for key, value in sorted(shell_tool_env(env).items())
    )
    script = (
        "#!/bin/sh\n"
        f"exec {shlex.quote(sandbox_exec)} -f {shlex.quote(str(profile_path))} "
        f"/usr/bin/env -i \\\n{assignments} \\\n  /bin/bash \"$@\"\n"
    )
    wrapper.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with wrapper.open("x", encoding="utf-8") as output:
        output.write(script)
    os.chmod(wrapper, 0o700)


def probe_shell_sandbox(
    write_root: pathlib.Path,
    *,
    wrapper: pathlib.Path,
    env: dict[str, str],
    auth_paths: tuple[pathlib.Path, ...],
) -> bool:
    """Prove nested shell calls cannot write out, read auth, or use the network."""
    inside = write_root / f".shell-probe-inside-{secrets.token_hex(5)}"
    outside = write_root.parent / f"{write_root.name}-shell-probe-{secrets.token_hex(5)}"
    read_markers = [write_root / f".shell-probe-read-{secrets.token_hex(5)}" for _ in auth_paths]
    execution_marker = write_root / f".shell-probe-exec-{secrets.token_hex(5)}"
    network_marker = write_root / f".shell-probe-network-{secrets.token_hex(5)}"
    private_auth_dir = auth_paths[-1].parent.resolve(strict=True) if auth_paths else None
    if private_auth_dir is None or not private_auth_dir.is_relative_to(write_root.resolve(strict=True)):
        raise RunnerError("nested sandbox write probe does not have a task-owned credential directory")
    auth_write_canary = private_auth_dir / f".shell-probe-write-{secrets.token_hex(5)}"
    auth_write_marker = write_root / f".shell-probe-auth-write-{secrets.token_hex(5)}"
    if outside.exists() or outside.is_symlink():
        raise RunnerError("nested sandbox probe target unexpectedly exists")
    if auth_write_canary.exists() or auth_write_canary.is_symlink():
        raise RunnerError("nested sandbox auth-write probe target unexpectedly exists")
    if any(not path.is_file() or path.is_symlink() for path in auth_paths):
        raise RunnerError("nested sandbox auth-denial probe lacks a regular auth file")

    listener = socket.socket()
    try:
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        listener.setblocking(False)
        port = listener.getsockname()[1]
        command_parts = [
            f"/usr/bin/touch {shlex.quote(str(inside))}",
            f"if {shlex.quote(sys.executable)} -c pass >/dev/null 2>&1; "
            f"then /usr/bin/touch {shlex.quote(str(execution_marker))}; fi",
        ]
        for auth_path, marker in zip(auth_paths, read_markers):
            command_parts.append(
                f"if /bin/cat {shlex.quote(str(auth_path))} >/dev/null 2>&1; "
                f"then /usr/bin/touch {shlex.quote(str(marker))}; fi"
            )
        command_parts.append(
            f"if /usr/bin/touch {shlex.quote(str(auth_write_canary))} >/dev/null 2>&1; "
            f"then /usr/bin/touch {shlex.quote(str(auth_write_marker))}; fi"
        )
        connect = (
            "import socket; s=socket.socket(); s.settimeout(1); "
            f"s.connect(('127.0.0.1', {port}))"
        )
        command_parts.append(
            f"if {shlex.quote(sys.executable)} -c {shlex.quote(connect)} >/dev/null 2>&1; "
            f"then /usr/bin/touch {shlex.quote(str(network_marker))}; fi"
        )
        command_parts.append(f"/usr/bin/touch {shlex.quote(str(outside))} >/dev/null 2>&1 || true")
        command = "; ".join(command_parts)
        try:
            result = subprocess.run(
                [str(wrapper), "--noprofile", "--norc", "-lc", command],
                cwd=write_root,
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=False,
                timeout=10,
            )
        except (OSError, subprocess.TimeoutExpired) as error:
            raise RunnerError("nested macOS shell sandbox probe could not complete") from error
        network_allowed = network_marker.exists() or network_marker.is_symlink()
        try:
            connection, _ = listener.accept()
            connection.close()
            network_allowed = True
        except BlockingIOError:
            pass
        readable = any(marker.exists() or marker.is_symlink() for marker in read_markers)
        checks = {
            "shell_started": result.returncode == 0,
            "inside_write": inside.is_file(),
            "python_exec": execution_marker.is_file(),
            "outside_write_denied": not outside.exists() and not outside.is_symlink(),
            "auth_reads_denied": not readable,
            "auth_writes_denied": not auth_write_canary.exists() and not auth_write_marker.exists(),
            "network_denied": not network_allowed,
        }
        failed = [name for name, passed in checks.items() if not passed]
        if failed:
            raise RunnerError("nested shell sandbox probe failed checks: " + ",".join(failed))
        return True
    finally:
        listener.close()
        for path in (inside, *read_markers, execution_marker, network_marker, auth_write_canary, auth_write_marker):
            if path.exists() and path.is_file() and not path.is_symlink():
                path.unlink()


def terminate_process_group(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is not None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=1.5)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait(timeout=2)


def classify_agent_error(stderr: bytes) -> str:
    text = stderr[:MAX_ERROR_CLASSIFICATION_BYTES].decode("utf-8", errors="ignore").lower()
    if not text.strip():
        return "no_diagnostic"
    if "http 429" in text or "rate limit" in text or "usage limit" in text:
        return "provider_rate_limited"
    if "http 401" in text or "unauthorized" in text or "invalid_token" in text:
        return "provider_unauthorized"
    if "http 403" in text or "forbidden" in text:
        return "provider_access_denied"
    if "http 400" in text or "unsupported model" in text or "invalid model" in text:
        return "provider_request_rejected"
    if "codex request failed" in text or "connection refused" in text or "no such host" in text:
        return "provider_transport_failure"
    if "no codex login" in text or "auth.json" in text:
        return "local_auth_unavailable"
    return "unclassified"


def observe_child(
    argv: list[str],
    *,
    cwd: pathlib.Path,
    env: dict[str, str],
    timeout_seconds: int,
    observed_tool_start_stop_threshold: int | None = None,
) -> dict[str, object]:
    started = time.monotonic()
    process = subprocess.Popen(
        argv,
        cwd=cwd,
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        bufsize=0,
        start_new_session=True,
    )
    assert process.stdout is not None and process.stderr is not None
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ, "stdout")
    selector.register(process.stderr, selectors.EVENT_READ, "stderr")
    pending: dict[str, bytearray] = {"stdout": bytearray(), "stderr": bytearray()}
    counts = {"stdout": 0, "stderr": 0}
    stderr_diagnostic = bytearray()
    tool_starts = 0
    reason = ""
    deadline = started + timeout_seconds

    def count_lines(stream_name: str, chunk: bytes, *, final: bool = False) -> None:
        nonlocal tool_starts, reason
        if stream_name != "stderr":
            return
        buffer = pending[stream_name]
        buffer.extend(chunk)
        while b"\n" in buffer:
            line, _, rest = buffer.partition(b"\n")
            buffer[:] = rest
            if line.startswith("→ ".encode("utf-8")):
                tool_starts += 1
                if observed_tool_start_stop_threshold is not None and tool_starts > observed_tool_start_stop_threshold:
                    reason = "observed_tool_start_stop_threshold_exceeded"
        if len(buffer) > MAX_LINE_BYTES:
            reason = "output_line_limit_exceeded"
        if final and buffer:
            if buffer.startswith("→ ".encode("utf-8")):
                tool_starts += 1
                if observed_tool_start_stop_threshold is not None and tool_starts > observed_tool_start_stop_threshold:
                    reason = "observed_tool_start_stop_threshold_exceeded"
            buffer.clear()

    try:
        while selector.get_map():
            if reason:
                terminate_process_group(process)
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                reason = "process_timeout"
                terminate_process_group(process)
            events = selector.select(min(max(remaining, 0.05), 0.2))
            for key, _ in events:
                stream_name = str(key.data)
                try:
                    chunk = os.read(key.fileobj.fileno(), 64 * 1024)
                except OSError:
                    chunk = b""
                if not chunk:
                    count_lines(stream_name, b"", final=True)
                    selector.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                counts[stream_name] += len(chunk)
                if stream_name == "stderr" and len(stderr_diagnostic) < MAX_ERROR_CLASSIFICATION_BYTES:
                    remaining_diagnostic = MAX_ERROR_CLASSIFICATION_BYTES - len(stderr_diagnostic)
                    stderr_diagnostic.extend(chunk[:remaining_diagnostic])
                if counts["stdout"] + counts["stderr"] > MAX_OUTPUT_BYTES:
                    reason = "output_limit_exceeded"
                    terminate_process_group(process)
                count_lines(stream_name, chunk)
                if reason:
                    terminate_process_group(process)
            if process.poll() is not None and selector.get_map():
                continue
    except BaseException:
        terminate_process_group(process)
        raise
    finally:
        selector.close()
    returncode = process.wait()
    return {
        "exit_code": returncode,
        "latency_ms": round((time.monotonic() - started) * 1000),
        "stdout_bytes": counts["stdout"],
        "stderr_bytes": counts["stderr"],
        "error_category": classify_agent_error(bytes(stderr_diagnostic)) if returncode != 0 else None,
        "tool_starts_observed": tool_starts,
        "stop_reason": reason or None,
    }


def latest_agent_rounds(picogent_home: pathlib.Path) -> dict[str, object]:
    found: list[tuple[float, int, str]] = []
    for path in (picogent_home / "tasks").rglob("*.json") if (picogent_home / "tasks").exists() else ():
        try:
            payload = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, UnicodeError, json.JSONDecodeError):
            continue
        turns = payload.get("turns") if isinstance(payload, dict) else None
        if not isinstance(turns, list) or not turns:
            continue
        last = turns[-1]
        if not isinstance(last, dict) or not isinstance(last.get("tool_rounds"), int):
            continue
        finished = last.get("finished_at")
        found.append((path.stat().st_mtime, last["tool_rounds"], str(last.get("state", "unknown"))))
    if not found:
        return {"value": None, "status": "UNVERIFIED", "source": "taskstate"}
    _, count, state = max(found)
    return {"value": count, "status": "OBSERVED", "source": "taskstate.tool_rounds", "turn_state": state}


def task_input_digest(
    task: dict[str, object],
    model: str,
    budgets: dict[str, int],
    *,
    initial_tree_sha256: str,
) -> str:
    return canonical_digest(
        {
            "task_id": task["id"],
            "prompt": task["prompt"],
            "allowed_paths": safe_relative_paths(task["allowed_paths"]),
            "model": model,
            "provider": "codex",
            "budgets": budgets,
            "initial_tree_sha256": initial_tree_sha256,
        }
    )


def evaluate_command(
    command: str,
    *,
    workspace: pathlib.Path,
    shell_wrapper: pathlib.Path,
    env: dict[str, str],
    timeout_seconds: int,
) -> dict[str, object]:
    observed = observe_child(
        [str(shell_wrapper), "--noprofile", "--norc", "-lc", command],
        cwd=workspace,
        env=env,
        timeout_seconds=timeout_seconds,
    )
    status = "PASS" if observed["exit_code"] == 0 and observed["stop_reason"] is None else "FAIL"
    return {"kind": "command", "status": status, "exit_code": observed["exit_code"], "latency_ms": observed["latency_ms"]}


def browser_result_from_input(stdin: TextIO, stdout: TextIO, *, timeout_seconds: int) -> str:
    if not stdin.isatty():
        return "UNVERIFIED"
    stdout.write("Verify the listed BrowserOS acceptance steps, then type PASS or FAIL: ")
    stdout.flush()
    ready, _, _ = select.select([stdin], [], [], timeout_seconds)
    if not ready:
        return "UNVERIFIED"
    answer = stdin.readline().strip().upper()
    return answer if answer in {"PASS", "FAIL"} else "UNVERIFIED"


def evaluate_browser(
    acceptance: dict[str, object],
    *,
    candidate: str,
    task_id: str,
    repetition: int,
    workspace: pathlib.Path,
    run_root: pathlib.Path,
    env: dict[str, str],
    stdin: TextIO,
    stdout: TextIO,
    timeout_seconds: int,
    sandbox_exec: str,
    denied_reads: tuple[pathlib.Path, ...] = (),
    denied_writes: tuple[pathlib.Path, ...] = (),
) -> dict[str, object]:
    profile = sandbox_profile(run_root, denied_reads=denied_reads, denied_writes=denied_writes)
    try:
        server = subprocess.Popen(
            [
                sandbox_exec,
                "-p",
                profile,
                sys.executable,
                "-u",
                "-m",
                "http.server",
                "0",
                "--bind",
                "127.0.0.1",
                "--directory",
                str(workspace),
            ],
            cwd=workspace,
            env=shell_tool_env(env),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=True,
        )
    except OSError:
        return {"kind": "real-browser", "status": "UNVERIFIED", "reason": "server_start_failed"}
    try:
        assert server.stderr is not None
        ready, _, _ = select.select([server.stderr], [], [], 5)
        line = server.stderr.readline() if ready else ""
        match = re.search(r"\bport (\d+)\b", line)
        if not match:
            return {"kind": "real-browser", "status": "UNVERIFIED", "reason": "server_ready_unobserved"}
        host = f"{run_root.name}.localhost"
        url = f"http://{host}:{match.group(1)}/"
        stdout.write(
            f"\nBROWSER_ORACLE candidate={candidate} task={task_id} repetition={repetition} url={url}\n"
        )
        steps = acceptance.get("steps", [])
        if isinstance(steps, list):
            stdout.write("BrowserOS acceptance steps:\n")
            for number, step in enumerate(steps, start=1):
                stdout.write(f"{number}. {step}\n")
        stdout.flush()
        status = browser_result_from_input(stdin, stdout, timeout_seconds=timeout_seconds)
        return {
            "kind": "real-browser",
            "status": status,
            "url_origin": f"http://{host}:{match.group(1)}",
        }
    finally:
        terminate_process_group(server)
        if server.stderr is not None:
            server.stderr.close()


def make_temp_root(parent: pathlib.Path) -> tuple[pathlib.Path, str]:
    resolved_parent = parent.resolve(strict=True)
    if not resolved_parent.is_dir():
        raise RunnerError("temporary parent is not a directory")
    root = pathlib.Path(tempfile.mkdtemp(prefix=TEMP_PREFIX, dir=resolved_parent))
    os.chmod(root, 0o700)
    if root.is_symlink() or root.resolve(strict=True).parent != resolved_parent:
        raise RunnerError("task temporary root failed its path check")
    token = secrets.token_hex(16)
    marker = root / ROOT_MARKER
    marker.write_text(token, encoding="ascii")
    os.chmod(marker, 0o600)
    return root, token


def validate_temp_root(root: pathlib.Path, parent: pathlib.Path, token: str) -> None:
    parent = parent.resolve(strict=True)
    if root.is_symlink() or not root.is_dir() or root.name.startswith(TEMP_PREFIX) is False:
        raise RunnerError("refusing cleanup: temporary root identity changed")
    resolved = root.resolve(strict=True)
    if resolved.parent != parent or resolved == parent:
        raise RunnerError("refusing cleanup: temporary root escaped its parent")
    marker = resolved / ROOT_MARKER
    if marker.is_symlink() or not marker.is_file() or marker.read_text(encoding="ascii") != token:
        raise RunnerError("refusing cleanup: task-owned marker mismatch")


def git_worktree_add(repo: pathlib.Path, target: pathlib.Path, sha: str) -> None:
    result = subprocess.run(
        ["git", "-C", str(repo), "worktree", "add", "--detach", str(target), sha],
        capture_output=True,
        stdin=subprocess.DEVNULL,
        check=False,
        timeout=120,
    )
    if result.returncode != 0:
        raise RunnerError("could not create the requested exact-SHA worktree")


def module_cache_path() -> pathlib.Path:
    result = subprocess.run(["go", "env", "GOMODCACHE"], capture_output=True, check=False, timeout=10)
    if result.returncode != 0:
        raise RunnerError("Go module cache could not be resolved")
    path = pathlib.Path(result.stdout.decode("utf-8", errors="replace").strip()).resolve(strict=True)
    if not path.is_dir():
        raise RunnerError("Go module cache is unavailable")
    return path


def build_binary(source: pathlib.Path, output: pathlib.Path, root: pathlib.Path, module_cache: pathlib.Path) -> None:
    env = limited_go_env(root, module_cache)
    output.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    run_checked(
        ["go", "build", "-trimpath", "-buildvcs=false", "-p=1", "-o", str(output), "./cmd/picogent"],
        cwd=source,
        env=env,
        timeout=240,
    )
    if not output.is_file() or output.stat().st_size == 0:
        raise RunnerError("Picogent build produced no binary")


def create_picogent_config(
    home: pathlib.Path,
    *,
    workspace: pathlib.Path,
    model: str,
    rounds: int,
    request_timeout: int,
) -> None:
    home.mkdir(mode=0o700, parents=True, exist_ok=True)
    config = (
        f"workspace: {json.dumps(str(workspace))}\n"
        "mode: fast\n"
        "task_mode: agent\n"
        "provider: codex\n"
        f"model: {model}\n"
        f"max_tool_rounds: {rounds}\n"
        f"llm_timeout_sec: {request_timeout}\n"
        "bash_timeout_sec: 25\n"
        "setup_complete: true\n"
        "router:\n"
        "  enabled: false\n"
        "  use_llm_advisor: false\n"
    )
    path = home / "config.yaml"
    with path.open("x", encoding="utf-8") as destination:
        destination.write(config)
    os.chmod(path, 0o600)


def prepare_workspace(task: dict[str, object], destination: pathlib.Path) -> tuple[dict[str, str], str]:
    fixture = load_fixture_module()
    manifest = fixture.load_manifest()
    entry = fixture.task_by_id(manifest, str(task["id"]))
    source = fixture.fixture_path(entry)
    expected = str(entry["initial_tree_sha256"])
    initial_digest = fixture.tree_digest(source)
    if initial_digest != expected:
        raise RunnerError("pristine task fixture digest mismatch")
    shutil.copytree(source, destination, symlinks=False)
    actual = file_digests(destination)
    copied_digest = fixture.tree_digest(destination)
    if copied_digest != expected:
        raise RunnerError("disposable workspace copy digest mismatch")
    return actual, copied_digest


def run_one(
    *,
    task: dict[str, object],
    variant: str,
    candidate: str,
    repetition: int,
    source: pathlib.Path,
    binary: pathlib.Path,
    expected_sha: str,
    top_root: pathlib.Path,
    auth_source: pathlib.Path,
    minimum_auth_validity_seconds: int,
    sandbox_exec: str,
    module_cache: pathlib.Path,
    model: str,
    budgets: dict[str, int],
    stdin: TextIO,
    stdout: TextIO,
) -> dict[str, object]:
    safe_paths = safe_relative_paths(task["allowed_paths"])
    run_tag = f"{task['id']}-r{repetition}-{candidate.lower()}-{secrets.token_hex(4)}"
    run_root = top_root / f"run-{run_tag}"
    run_root.mkdir(mode=0o700)
    marker = run_root / ".run-marker"
    marker.write_text(secrets.token_hex(12), encoding="ascii")
    marker_value = marker.read_text(encoding="ascii")
    workspace = run_root / "workspace"
    initial_files, initial_tree = prepare_workspace(task, workspace)
    picogent_home = run_root / "picogent-home"
    codex_home = run_root / "codex-home"
    codex_home.mkdir(mode=0o700)
    credential = read_codex_credential(auth_source, minimum_validity_seconds=minimum_auth_validity_seconds)
    auth_copy = codex_home / "auth.json"
    write_access_only_auth_file(credential, auth_copy)
    create_picogent_config(
        picogent_home,
        workspace=workspace,
        model=model,
        rounds=budgets["max_tool_rounds"],
        request_timeout=budgets["request_timeout_seconds"],
    )
    env = run_env(run_root, model, module_cache)
    profile_dir = top_root / "tool-profiles"
    profile_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    tool_profile_path = profile_dir / f"{run_tag}.sbpl"
    tool_profile = sandbox_profile(
        run_root,
        denied_reads=(auth_source.parent.resolve(strict=True), auth_copy.parent.resolve(strict=True)),
        denied_writes=(auth_source.parent.resolve(strict=True), auth_copy.parent.resolve(strict=True)),
        allow_network=False,
    )
    with tool_profile_path.open("x", encoding="utf-8") as output:
        output.write(tool_profile)
    os.chmod(tool_profile_path, 0o600)
    wrapper = top_root / "tool-bins" / run_tag / "bash"
    write_shell_wrapper(
        wrapper,
        sandbox_exec=sandbox_exec,
        profile_path=tool_profile_path,
        env=env,
    )
    env["PATH"] = str(wrapper.parent) + os.pathsep + env.get("PATH", "/usr/bin:/bin")
    sandbox_ok = probe_sandbox(
        run_root,
        env=env,
        sandbox_exec=sandbox_exec,
        denied_reads=(auth_source.parent.resolve(strict=True),),
        denied_writes=(auth_source.parent.resolve(strict=True), auth_copy.parent.resolve(strict=True)),
    )
    shell_sandbox_ok = probe_shell_sandbox(
        run_root,
        wrapper=wrapper,
        env=env,
        auth_paths=(auth_source.resolve(strict=True), auth_copy.resolve(strict=True)),
    )
    before = source_state(source, expected_sha)
    if before["head_matches"] is not True or before["clean"] is not True:
        raise RunnerError(f"{variant} source was not exact and clean before the run")
    profile = sandbox_profile(
        run_root,
        denied_reads=(auth_source.parent.resolve(strict=True),),
        denied_writes=(auth_source.parent.resolve(strict=True), auth_copy.parent.resolve(strict=True)),
        allow_macos_trust=True,
    )
    observed = observe_child(
        [
            sandbox_exec,
            "-p",
            profile,
            str(binary),
            "run",
            "--dir",
            str(workspace),
            "--yes",
            "--model",
            model,
            str(task["prompt"]),
        ],
        cwd=workspace,
        env=env,
        timeout_seconds=budgets["run_timeout_seconds"],
        observed_tool_start_stop_threshold=budgets["tool_start_stop_threshold"],
    )
    after = source_state(source, expected_sha)
    if after["head_matches"] is not True or after["clean"] is not True:
        raise RunnerError(f"{variant} source changed during the run")
    final_files = file_digests(workspace)
    changed = changed_paths(initial_files, final_files)
    runtime_paths = [path for path in changed if is_runtime_metadata_path(path)]
    candidate_changes = [path for path in changed if path not in runtime_paths]
    outside_allowlist = sorted(set(candidate_changes) - set(safe_paths))
    paths_status = "PASS" if not outside_allowlist else "FAIL"
    status = "PASS" if observed["exit_code"] == 0 and observed["stop_reason"] is None else "FAIL"
    acceptance_spec = task["acceptance"]
    if status == "PASS" and acceptance_spec["kind"] == "command":
        acceptance = evaluate_command(
            str(acceptance_spec["command"]),
            workspace=workspace,
            shell_wrapper=wrapper,
            env=env,
            timeout_seconds=budgets["acceptance_timeout_seconds"],
        )
    elif status == "PASS" and acceptance_spec["kind"] == "real-browser":
        acceptance = evaluate_browser(
            acceptance_spec,
            candidate=candidate,
            task_id=str(task["id"]),
            repetition=repetition,
            workspace=workspace,
            run_root=run_root,
            env=env,
            stdin=stdin,
            stdout=stdout,
            timeout_seconds=budgets["acceptance_timeout_seconds"],
            sandbox_exec=sandbox_exec,
            denied_reads=(auth_source.parent.resolve(strict=True), auth_copy.parent.resolve(strict=True)),
            denied_writes=(auth_source.parent.resolve(strict=True), auth_copy.parent.resolve(strict=True)),
        )
    else:
        acceptance = {"kind": str(acceptance_spec["kind"]), "status": "UNVERIFIED", "reason": "agent_run_not_successful"}
    rounds = latest_agent_rounds(picogent_home)
    result = {
        "run_id": run_tag,
        "task_id": task["id"],
        "repetition": repetition,
        "candidate": candidate,
        "input_sha256": task_input_digest(
            task,
            model,
            budgets,
            initial_tree_sha256=initial_tree,
        ),
        "source": {
            "verified_before": before["head_matches"] is True and before["clean"] is True,
            "verified_after": after["head_matches"] is True and after["clean"] is True,
        },
        "provider": "codex",
        "requested_model": model,
        "served_model": {"value": None, "status": "UNVERIFIED"},
        "budgets": budgets,
        "sandbox_write_probe": "PASS" if sandbox_ok else "FAIL",
        "shell_sandbox_probe": "PASS" if shell_sandbox_ok else "FAIL",
        "agent": {
            "status": status,
            "exit_code": observed["exit_code"],
            "stop_reason": observed["stop_reason"],
            "error_category": observed["error_category"],
            "latency_ms": observed["latency_ms"],
            "stdout_bytes": observed["stdout_bytes"],
            "stderr_bytes": observed["stderr_bytes"],
            "tool_starts_observed": {
                "value": observed["tool_starts_observed"],
                "status": "OBSERVED",
                "source": "CLI stderr tool-start markers",
            },
            "tool_call_budget_enforcement": {
                "value": "stop-after-observed-start; dispatch is already underway",
                "status": "UNVERIFIED",
            },
            "agent_tool_rounds": rounds,
            "model_calls": {"value": None, "status": "UNVERIFIED"},
            "provider_http_requests": {"value": None, "status": "UNVERIFIED"},
            "token_usage": {"input_tokens": None, "output_tokens": None, "status": "UNVERIFIED"},
        },
        "workspace": {
            "initial_tree_sha256": initial_tree,
            "final_tree_sha256": canonical_digest(final_files),
            "changed_paths": changed,
            "runtime_metadata_paths": runtime_paths,
            "candidate_changed_paths": candidate_changes,
            "allowed_paths": safe_paths,
            "allowed_paths_status": paths_status,
            "out_of_scope_paths": outside_allowlist,
        },
        "acceptance": acceptance,
    }
    validate_run_root(run_root, top_root, marker_value)
    shutil.rmtree(run_root)
    return result


def validate_run_root(root: pathlib.Path, parent: pathlib.Path, marker_value: str) -> None:
    if (
        root.is_symlink()
        or not root.is_dir()
        or not root.name.startswith("run-")
        or root.parent.resolve(strict=True) != parent.resolve(strict=True)
    ):
        raise RunnerError("refusing cleanup: per-run root identity changed")
    marker = root / ".run-marker"
    if marker.is_symlink() or not marker.is_file() or marker.read_text(encoding="ascii") != marker_value:
        raise RunnerError("refusing cleanup: per-run marker mismatch")


def overall_verdict(results: list[dict[str, object]]) -> str:
    for result in results:
        if result["agent"]["status"] == "FAIL":
            return "FAIL"
        if result["workspace"]["allowed_paths_status"] == "FAIL":
            return "FAIL"
        if result["acceptance"]["status"] == "FAIL":
            return "FAIL"
        if result["source"]["verified_before"] is not True or result["source"]["verified_after"] is not True:
            return "FAIL"
    required_unverified = any(
        result["agent"]["provider_http_requests"]["status"] == "UNVERIFIED"
        or result["agent"]["token_usage"]["status"] == "UNVERIFIED"
        or result["agent"]["model_calls"]["status"] == "UNVERIFIED"
        or result["served_model"]["status"] == "UNVERIFIED"
        or result["agent"]["tool_call_budget_enforcement"]["status"] == "UNVERIFIED"
        or result["agent"]["agent_tool_rounds"]["status"] == "UNVERIFIED"
        or result["acceptance"]["status"] == "UNVERIFIED"
        for result in results
    )
    return "UNVERIFIED" if required_unverified else "PASS"


def candidate_source_mapping(order: list[tuple[str, str]]) -> dict[str, dict[str, str]]:
    if len(order) != 2 or dict(order) != {"v3": V3_SHA, "v4": V4_SHA}:
        raise RunnerError("candidate order must contain the exact pinned v3 and v4 sources")
    return {
        candidate: {"variant": variant, "source_sha": sha}
        for candidate, (variant, sha) in zip(("A", "B"), order)
    }


def worktree_remove(
    repo: pathlib.Path,
    worktree: pathlib.Path,
    *,
    parent: pathlib.Path,
    expected_sha: str,
) -> None:
    if worktree.is_symlink() or not worktree.is_dir() or worktree.resolve(strict=True).parent != parent.resolve(strict=True):
        raise RunnerError("refusing to remove a task worktree outside the owned temporary root")
    if worktree.name not in {"source-v3", "source-v4"}:
        raise RunnerError("refusing to remove an unexpected task worktree")
    exact = source_state(worktree, expected_sha)
    if exact["head_matches"] is not True or exact["clean"] is not True:
        raise RunnerError("refusing to remove a worktree whose exact source state changed")
    state = subprocess.run(
        ["git", "-C", str(worktree), "status", "--porcelain=v1", "--untracked-files=all"],
        capture_output=True,
        check=False,
        timeout=10,
    )
    if state.returncode != 0 or state.stdout != b"":
        raise RunnerError("refusing to remove a non-clean task worktree")
    result = subprocess.run(
        ["git", "-C", str(repo), "worktree", "remove", str(worktree)],
        capture_output=True,
        check=False,
        timeout=60,
    )
    if result.returncode != 0:
        raise RunnerError("task worktree cleanup failed; temporary root was preserved")


def write_artifact(path: pathlib.Path, artifact: dict[str, object]) -> None:
    path = path.expanduser()
    if path.exists() or path.is_symlink():
        raise RunnerError("refusing to overwrite an existing evidence file")
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8") as output:
        json.dump(artifact, output, indent=2, sort_keys=True)
        output.write("\n")
    os.chmod(path, 0o600)


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--live", action="store_true", help="allow live calls using the existing Codex login")
    parser.add_argument("--repetitions", type=int, default=2)
    parser.add_argument("--task", action="append", default=[])
    parser.add_argument("--max-tool-rounds", type=int, default=5)
    parser.add_argument(
        "--tool-start-stop-threshold",
        type=int,
        default=12,
        help="best-effort stop after this many observed tool starts; not a pre-dispatch hard cap",
    )
    parser.add_argument("--request-timeout-seconds", type=int, default=45)
    parser.add_argument("--run-timeout-seconds", type=int, default=240)
    parser.add_argument("--acceptance-timeout-seconds", type=int, default=90)
    parser.add_argument("--output", type=pathlib.Path)
    args = parser.parse_args(argv)
    if not args.live:
        parser.error("live comparisons require --live; no provider call is made without it")
    if not 1 <= args.repetitions <= 3:
        parser.error("--repetitions must be between 1 and 3")
    if not 1 <= args.max_tool_rounds <= 8 or not 1 <= args.tool_start_stop_threshold <= 24:
        parser.error("tool budgets exceed the runner's safe bounds")
    if not 10 <= args.request_timeout_seconds <= 120:
        parser.error("request timeout must be between 10 and 120 seconds")
    if not 30 <= args.run_timeout_seconds <= 600 or not 10 <= args.acceptance_timeout_seconds <= 180:
        parser.error("run or acceptance timeout exceeds the runner's safe bounds")
    return args


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    sandbox_exec = "/usr/bin/sandbox-exec"
    if sys.platform != "darwin" or not pathlib.Path(sandbox_exec).is_file():
        raise RunnerError("this runner requires macOS sandbox-exec for write confinement")
    for command in ("git", "go", "python3"):
        if shutil.which(command) is None:
            raise RunnerError(f"required local command is unavailable: {command}")
    fixture = load_fixture_module()
    manifest = fixture.load_manifest()
    if not fixture.verify(manifest):
        raise RunnerError("committed task fixture verification failed")
    all_tasks = {str(task["id"]): task for task in manifest["tasks"]}
    selected = args.task or list(all_tasks)
    if not selected or len(selected) != len(set(selected)) or any(task_id not in all_tasks for task_id in selected):
        raise RunnerError("requested task selection is invalid")
    auth_source = host_codex_auth_path()
    planned_runs = len(selected) * args.repetitions * 2
    minimum_auth_validity_seconds = (
        planned_runs * (args.run_timeout_seconds + args.acceptance_timeout_seconds) + 2 * 240 + 300
    )
    read_codex_credential(auth_source, minimum_validity_seconds=minimum_auth_validity_seconds)
    module_cache = module_cache_path()
    budgets = {
        "max_tool_rounds": args.max_tool_rounds,
        "tool_start_stop_threshold": args.tool_start_stop_threshold,
        "request_timeout_seconds": args.request_timeout_seconds,
        "run_timeout_seconds": args.run_timeout_seconds,
        "acceptance_timeout_seconds": args.acceptance_timeout_seconds,
        "bash_timeout_seconds": 25,
        "build_parallelism": 1,
        "gomaxprocs": 1,
        "gomemlimit_bytes": 1024 * 1024 * 1024,
    }
    parent = pathlib.Path(tempfile.gettempdir()).resolve(strict=True)
    top_root, marker = make_temp_root(parent)
    worktrees: dict[str, pathlib.Path] = {}
    results: list[dict[str, object]] = []
    candidate_source_map: dict[str, dict[str, object]] = {}
    try:
        for label, sha in (("v3", V3_SHA), ("v4", V4_SHA)):
            check = subprocess.run(
                ["git", "-C", str(REPO_ROOT), "cat-file", "-e", f"{sha}^{{commit}}"],
                capture_output=True,
                check=False,
                timeout=10,
            )
            if check.returncode != 0:
                raise RunnerError(f"pinned {label} source commit is unavailable locally")
            path = top_root / f"source-{label}"
            git_worktree_add(REPO_ROOT, path, sha)
            worktrees[label] = path
            state = source_state(path, sha)
            if state["head_matches"] is not True or state["clean"] is not True:
                raise RunnerError(f"pinned {label} source worktree failed preflight")
        binaries = {label: top_root / "bin" / f"picogent-{label}" for label in worktrees}
        for label in ("v3", "v4"):
            build_binary(worktrees[label], binaries[label], top_root, module_cache)
            state = source_state(worktrees[label], V3_SHA if label == "v3" else V4_SHA)
            if state["head_matches"] is not True or state["clean"] is not True:
                raise RunnerError(f"pinned {label} source changed during build")
        variant_pairs = [("v3", V3_SHA), ("v4", V4_SHA)]
        rng = secrets.SystemRandom()
        for task_id in selected:
            task = all_tasks[task_id]
            for repetition in range(1, args.repetitions + 1):
                order = variant_pairs.copy()
                rng.shuffle(order)
                candidate_sources = candidate_source_mapping(order)
                candidate_by_variant = {
                    str(source["variant"]): candidate for candidate, source in candidate_sources.items()
                }
                candidate_source_map[f"{task_id}:r{repetition}"] = {
                    "task_id": task_id,
                    "repetition": repetition,
                    "candidates": candidate_sources,
                }
                for variant, sha in order:
                    remaining_runs = planned_runs - len(results)
                    run_auth_validity = (
                        remaining_runs * (args.run_timeout_seconds + args.acceptance_timeout_seconds) + 300
                    )
                    results.append(
                        run_one(
                            task=task,
                            variant=variant,
                            candidate=candidate_by_variant[variant],
                            repetition=repetition,
                            source=worktrees[variant],
                            binary=binaries[variant],
                            expected_sha=sha,
                            top_root=top_root,
                            auth_source=auth_source,
                            minimum_auth_validity_seconds=run_auth_validity,
                            sandbox_exec=sandbox_exec,
                            module_cache=module_cache,
                            model=DEFAULT_MODEL,
                            budgets=budgets,
                            stdin=sys.stdin,
                            stdout=sys.stdout,
                        )
                    )
        for label in ("v3", "v4"):
            worktree_remove(
                REPO_ROOT,
                worktrees[label],
                parent=top_root,
                expected_sha=V3_SHA if label == "v3" else V4_SHA,
            )
        validate_temp_root(top_root, parent, marker)
        shutil.rmtree(top_root)
    except BaseException:
        # Preserve the exact task root if a source worktree is no longer clean;
        # otherwise remove only worktrees created by this invocation.
        for label, worktree in worktrees.items():
            if worktree.exists():
                try:
                    worktree_remove(
                        REPO_ROOT,
                        worktree,
                        parent=top_root,
                        expected_sha=V3_SHA if label == "v3" else V4_SHA,
                    )
                except RunnerError:
                    raise
        validate_temp_root(top_root, parent, marker)
        shutil.rmtree(top_root)
        raise

    # Results are recorded by candidate label before source identity is revealed.
    verdict = overall_verdict(results)
    artifact = {
        "schema_version": 1,
        "status": verdict,
        "observed_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "provider": "codex",
        "requested_model": DEFAULT_MODEL,
        "auth_mode": "existing Codex login; access-token-only private run copy; refresh token not copied",
        "source_shas": {"v3": V3_SHA, "v4": V4_SHA},
        "budgets": budgets,
        "repetitions": args.repetitions,
        "blind_order": "candidate order randomized per task/repetition; source identity withheld in per-candidate results and disclosed only in this post-run map",
        "candidate_source_map": candidate_source_map,
        "subjective_scores": {
            "status": "UNVERIFIED",
            "reason": "raw model output is not retained, so questions and repair quality are not scored",
        },
        "unverified_dimensions": [
            "actual served model and provider HTTP request count are not exposed by the runtime",
            "token usage aggregate is not persisted by the runtime",
            "tool-start observation is reactive and is not a pre-dispatch hard call cap",
            "subjective questions, repairs, and in-scope edit quality are not scored",
        ],
        "results": results,
    }
    if args.output:
        write_artifact(args.output, artifact)
    print(json.dumps({"status": verdict, "runs": len(results), "output": str(args.output) if args.output else None}, sort_keys=True))
    return {"PASS": 0, "FAIL": 1, "UNVERIFIED": 2}[verdict]


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RunnerError as error:
        print(f"runner error: {error}", file=sys.stderr)
        raise SystemExit(1)
