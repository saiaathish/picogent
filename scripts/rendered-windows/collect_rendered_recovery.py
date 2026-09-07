#!/usr/bin/env python3
"""Collect Windows owned-browser rendered-recovery evidence.

Drives a real Chromium (Playwright) through allow → undo → fresh-process
reload against the rendered_fixture binary. Emits digest-only platform and
observation JSON outside the checkout. Fail closed: never invents PASS.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import re
import shutil
import stat
import subprocess
import sys
import time


def sha256_file(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def is_inside(parent: pathlib.Path, candidate: pathlib.Path) -> bool:
    parent_abs = os.path.normcase(os.path.abspath(os.fspath(parent)))
    candidate_abs = os.path.normcase(os.path.abspath(os.fspath(candidate)))
    try:
        return os.path.commonpath((parent_abs, candidate_abs)) == parent_abs
    except ValueError:
        return False


def assert_no_symlink_components(path: pathlib.Path, label: str) -> None:
    current = pathlib.Path(os.path.abspath(os.fspath(path)))
    while True:
        try:
            if stat.S_ISLNK(current.lstat().st_mode):
                raise SystemExit(f"{label} must not contain symbolic-link components")
        except FileNotFoundError:
            parent = current.parent
            if parent == current:
                return
            current = parent
            continue
        parent = current.parent
        if parent == current:
            return
        current = parent


def prepare_output_directory(raw_path: pathlib.Path, workspace: pathlib.Path) -> pathlib.Path:
    if not raw_path.is_absolute():
        raise SystemExit("--out must be an absolute directory outside the checkout")
    output = pathlib.Path(os.path.abspath(os.fspath(raw_path)))
    assert_no_symlink_components(output, "--out")
    if is_inside(workspace, output):
        raise SystemExit("--out must be outside the source checkout")
    output.mkdir(parents=True, exist_ok=True)
    assert_no_symlink_components(output, "--out")
    if is_inside(workspace, pathlib.Path(os.path.realpath(output))):
        raise SystemExit("--out resolves inside the source checkout")
    return output


def prepare_fixture_home(raw_path: pathlib.Path | None, temp_root: pathlib.Path) -> pathlib.Path:
    home_root = raw_path or (temp_root / "picogent-rendered-windows-507")
    if not home_root.is_absolute():
        raise SystemExit("--home-root must be an absolute directory below the temp directory")
    home_root = pathlib.Path(os.path.abspath(os.fspath(home_root)))
    assert_no_symlink_components(home_root, "--home-root")
    if home_root == temp_root or not is_inside(temp_root, home_root):
        raise SystemExit("--home-root must be a child of the temp directory")
    if home_root.exists():
        if not home_root.is_dir():
            raise SystemExit(f"--home-root is not a directory: {home_root}")
        shutil.rmtree(home_root)
    home_root.mkdir(parents=True)
    return home_root


def wait_manifest(path: pathlib.Path, timeout: float = 45.0) -> tuple[dict, bytes]:
    deadline = time.time() + timeout
    while time.time() < deadline:
        if path.exists() and path.stat().st_size > 0:
            data = path.read_bytes()
            return json.loads(data.decode("utf-8")), data
        time.sleep(0.1)
    raise TimeoutError(f"timed out waiting for {path}")


def stop_fixture(proc: subprocess.Popen | None, phase: str) -> dict[str, object]:
    if proc is None:
        return {"exited": False, "returncode": None}
    if proc.poll() is None:
        try:
            proc.terminate()
        except ProcessLookupError:
            pass
        try:
            proc.wait(timeout=15)
        except subprocess.TimeoutExpired:
            try:
                proc.kill()
            except ProcessLookupError:
                pass
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired as error:
                raise RuntimeError(f"fixture process did not exit after termination (phase={phase})") from error
    if proc.poll() is None:
        raise RuntimeError(f"fixture process did not exit after termination (phase={phase})")
    return {"exited": True, "returncode": proc.returncode}


def write_exclusive(path: pathlib.Path, data: bytes) -> None:
    descriptor = os.open(
        os.fspath(path), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600
    )
    try:
        with os.fdopen(descriptor, "wb") as handle:
            descriptor = -1
            handle.write(data)
            handle.flush()
            os.fsync(handle.fileno())
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def start_fixture(
    *,
    fixture_bin: pathlib.Path,
    phase: str,
    home: pathlib.Path,
    workspace: pathlib.Path,
    manifest: pathlib.Path,
    sha: str,
) -> tuple[subprocess.Popen, dict, bytes]:
    if manifest.exists():
        manifest.unlink()
    env = os.environ.copy()
    env.update(
        {
            "PICOGENT_RENDERED_FIXTURE_SOURCE_SHA": sha,
            "PICOGENT_RENDERED_FIXTURE_PHASE": phase,
            "PICOGENT_RENDERED_FIXTURE_HOME": str(home),
            "PICOGENT_RENDERED_FIXTURE_WORKSPACE": str(workspace),
            "PICOGENT_RENDERED_FIXTURE_MANIFEST": str(manifest),
            "PICOGENT_RENDERED_FIXTURE_ADDR": "127.0.0.1:0",
            "PICOGENT_NO_BROWSER": "1",
        }
    )
    command = [
        str(fixture_bin),
        "-phase",
        phase,
        "-home",
        str(home),
        "-workspace",
        str(workspace),
        "-manifest",
        str(manifest),
        "-addr",
        "127.0.0.1:0",
    ]
    proc = subprocess.Popen(
        command,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    try:
        manifest_data, raw_manifest = wait_manifest(manifest)
        return proc, manifest_data, raw_manifest
    except Exception:
        stop_fixture(proc, f"{phase} startup")
        output, _ = proc.communicate(timeout=10)
        raise RuntimeError(f"fixture {phase} failed:\n{output}") from None


def normalize_arch(raw: str) -> str:
    value = (raw or "").strip().lower()
    mapping = {
        "amd64": "amd64",
        "x86_64": "amd64",
        "x64": "amd64",
        "arm64": "arm64",
        "aarch64": "arm64",
    }
    if value not in mapping:
        raise SystemExit(f"unsupported architecture identifier: {raw!r}")
    return mapping[value]


def sanitize_browser_id(raw: str) -> str:
    cleaned = []
    for ch in raw.lower():
        if ("a" <= ch <= "z") or ("0" <= ch <= "9") or ch in "-_.":
            cleaned.append(ch)
    out = "".join(cleaned).strip(".-_")
    if not out or len(out) > 64:
        raise SystemExit(f"browser identity invalid after sanitize: {raw!r} -> {out!r}")
    return out


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--sha", required=True, help="exact clean behavior/candidate SHA")
    parser.add_argument("--fixture-bin", required=True, type=pathlib.Path)
    parser.add_argument("--out", required=True, type=pathlib.Path)
    parser.add_argument(
        "--home-root",
        type=pathlib.Path,
        default=None,
        help="directory below os temp used for disposable fixture home",
    )
    args = parser.parse_args()

    sha = args.sha.strip().lower()
    if len(sha) != 40 or any(c not in "0123456789abcdef" for c in sha):
        raise SystemExit("sha must be a full lowercase commit id")

    fixture_bin = args.fixture_bin.resolve()
    if not fixture_bin.is_file():
        raise SystemExit(f"fixture binary missing: {fixture_bin}")

    workspace_root = pathlib.Path(os.path.realpath(os.getcwd()))
    out = prepare_output_directory(args.out, workspace_root)

    temp_root = pathlib.Path(
        os.path.realpath(os.path.abspath(os.environ.get("TEMP") or os.environ.get("TMP") or "/tmp"))
    )
    home_root_candidate = args.home_root or (temp_root / "picogent-rendered-windows-507")
    if home_root_candidate.is_absolute():
        home_root_candidate = pathlib.Path(os.path.abspath(os.fspath(home_root_candidate)))
        if is_inside(home_root_candidate, out) or is_inside(out, home_root_candidate):
            raise SystemExit("--out and --home-root must be separate directories")
    home_root = prepare_fixture_home(args.home_root, temp_root)
    home = home_root / "home"
    workspace = home / "workspace"
    probe = workspace / "rendered-recovery-probe.txt"
    seed_manifest = home / "seed-manifest.json"
    reload_manifest = home / "reload-manifest.json"
    browser_profile = home_root / "browser-profile"
    screenshot_root = home_root / "screenshots"
    home.mkdir(parents=True)
    screenshot_root.mkdir()

    from playwright.sync_api import sync_playwright

    seed_proc: subprocess.Popen | None = None
    reload_proc: subprocess.Popen | None = None
    seed = None
    reload_data = None
    seed_shutdown = None
    reload_shutdown = None
    screenshots: dict[str, pathlib.Path] = {}
    observed: dict = {}

    with sync_playwright() as p:
        context = p.chromium.launch_persistent_context(
            user_data_dir=str(browser_profile),
            headless=True,
            args=["--disable-dev-shm-usage"],
            viewport={"width": 1440, "height": 1100},
        )
        page = context.new_page()
        try:
            seed_proc, seed, seed_manifest_data = start_fixture(
                fixture_bin=fixture_bin,
                phase="seed",
                home=home,
                workspace=workspace,
                manifest=seed_manifest,
                sha=sha,
            )
            page.goto(seed["url"], wait_until="domcontentloaded")
            page.wait_for_selector("#prompt:not([disabled])", timeout=30000)
            undo = page.locator("#undo-turn")
            observed["initial_undo_visible"] = undo.is_enabled()
            screenshots["initial"] = screenshot_root / "windows-initial.png"
            page.screenshot(path=str(screenshots["initial"]), full_page=True)

            page.fill("#prompt", "Run the rendered recovery fixture.")
            page.click("#send")
            page.wait_for_selector("#perm.is-on", timeout=30000)
            buttons = page.locator("#perm button")
            observed["permission_controls_visible"] = buttons.count() >= 4 and all(
                buttons.nth(i).is_visible() for i in range(min(4, buttons.count()))
            )
            observed["probe_absent_before_allow"] = not probe.exists()
            screenshots["permission"] = screenshot_root / "windows-permission.png"
            page.screenshot(path=str(screenshots["permission"]), full_page=True)

            page.click("#perm button[data-allow='1']")
            deadline = time.time() + 30
            while time.time() < deadline and not probe.exists():
                time.sleep(0.1)
            if not probe.exists():
                raise TimeoutError("probe file did not appear after Allow")
            page.wait_for_function(
                "() => { const el = document.getElementById('undo-turn'); return el && !el.disabled; }"
            )
            page.wait_for_function("() => document.body.innerText.includes('Edited 1 file')")
            body_after_allow = page.inner_text("body")
            observed["edited_one_file_visible"] = "Edited 1 file" in body_after_allow
            observed["changed_files_one_visible_after_allow"] = "Changed files (1)" in body_after_allow
            observed["undo_visible_after_allow"] = page.locator("#undo-turn").is_enabled()
            observed["probe_sha256_after_allow"] = sha256_file(probe)
            screenshots["allowed"] = screenshot_root / "windows-allowed.png"
            page.screenshot(path=str(screenshots["allowed"]), full_page=True)

            page.click("#undo-turn")
            deadline = time.time() + 30
            while time.time() < deadline and probe.exists():
                time.sleep(0.1)
            page.wait_for_function(
                "() => { const el = document.getElementById('undo-turn'); return el && el.disabled; }"
            )
            body_after_undo = page.inner_text("body")
            observed["probe_absent_after_undo"] = not probe.exists()
            observed["undo_visible_after_undo"] = page.locator("#undo-turn").is_enabled()
            observed["changed_files_one_visible_after_undo"] = "Changed files (1)" in body_after_undo
            screenshots["undone"] = screenshot_root / "windows-undone.png"
            page.screenshot(path=str(screenshots["undone"]), full_page=True)

            seed_shutdown = stop_fixture(seed_proc, "seed")
            seed_proc = None

            reload_proc, reload_data, reload_manifest_data = start_fixture(
                fixture_bin=fixture_bin,
                phase="reload",
                home=home,
                workspace=workspace,
                manifest=reload_manifest,
                sha=sha,
            )
            try:
                page.goto(reload_data["url"], wait_until="domcontentloaded")
                page.wait_for_selector("#prompt:not([disabled])", timeout=30000)
                page.wait_for_function("() => document.body.innerText.includes('Changed files (1)')")
                body_reload = page.inner_text("body")
                observed["changed_files_one_visible_after_reload"] = "Changed files (1)" in body_reload
                observed["undo_visible_after_reload"] = page.locator("#undo-turn").is_enabled()
                observed["probe_absent_after_reload"] = not probe.exists()
                screenshots["reload"] = screenshot_root / "windows-reload.png"
                page.screenshot(path=str(screenshots["reload"]), full_page=True)
            finally:
                reload_shutdown = stop_fixture(reload_proc, "reload")
                reload_proc = None

            required = [
                not observed["initial_undo_visible"],
                observed["permission_controls_visible"],
                observed["probe_absent_before_allow"],
                observed["edited_one_file_visible"],
                observed["changed_files_one_visible_after_allow"],
                observed["undo_visible_after_allow"],
                observed["probe_absent_after_undo"],
                not observed["undo_visible_after_undo"],
                observed["changed_files_one_visible_after_undo"],
                observed["changed_files_one_visible_after_reload"],
                not observed["undo_visible_after_reload"],
                observed["probe_absent_after_reload"],
                bool(seed_shutdown and seed_shutdown["exited"]),
                bool(reload_shutdown and reload_shutdown["exited"]),
                bool(seed["source_sha_verified"]),
                not bool(seed["source_tree_modified"]),
                bool(reload_data["source_sha_verified"]),
                not bool(reload_data["source_tree_modified"]),
            ]
            verdict = "PASS" if all(required) else "FAIL"

            retained_screenshots: dict[str, pathlib.Path] = {}
            for name, staging_path in sorted(screenshots.items()):
                retained_path = out / staging_path.name
                write_exclusive(retained_path, staging_path.read_bytes())
                retained_screenshots[name] = retained_path
            screenshot_digests = {
                name: sha256_file(path) for name, path in sorted(retained_screenshots.items())
            }
            screenshot_set_sha = hashlib.sha256(
                json.dumps(screenshot_digests, sort_keys=True, separators=(",", ":")).encode()
            ).hexdigest()

            browser_version = "unrecorded"
            if context.browser is not None and getattr(context.browser, "version", None):
                browser_version = str(context.browser.version)
            else:
                ua = page.evaluate("() => navigator.userAgent")
                match = re.search(r"(?:Chromium|Chrome)/([0-9.]+)", ua or "")
                if match:
                    browser_version = match.group(1)
            browser_id = sanitize_browser_id(f"playwright-chromium-headless-{browser_version}")
            if hasattr(os, "uname"):
                arch_raw = os.uname().machine
            else:
                arch_raw = os.environ.get("PROCESSOR_ARCHITECTURE", "amd64")
            architecture = normalize_arch(arch_raw)

            observation = {
                "schema": "picogent.v4.rendered-recovery-observation.v1",
                "candidate_sha": sha,
                "platform": "windows",
                "architecture": architecture,
                "browser": browser_id,
                "fixture": "rendered-recovery",
                "environment": "task-owned-disposable",
                "seed_started_at": seed["started_at_utc"],
                "reload_started_at": reload_data["started_at_utc"],
                "source_sha_verified": bool(seed["source_sha_verified"])
                and bool(reload_data["source_sha_verified"]),
                "source_tree_modified": bool(seed["source_tree_modified"])
                or bool(reload_data["source_tree_modified"]),
                **observed,
                "provenance": {
                    "seed_manifest_sha256": sha256_bytes(seed_manifest_data),
                    "reload_manifest_sha256": sha256_bytes(reload_manifest_data),
                    "seed_fixture_exit_verified": bool(seed_shutdown and seed_shutdown["exited"]),
                    "reload_fixture_exit_verified": bool(reload_shutdown and reload_shutdown["exited"]),
                },
                "screenshot_sha256": screenshot_digests,
                "screenshot_set_sha256": screenshot_set_sha,
                "verdict": verdict,
            }
            observation_path = out / "windows-rendered-recovery-observation.json"
            observation_data = (json.dumps(observation, indent=2, sort_keys=True) + "\n").encode(
                "utf-8"
            )
            write_exclusive(observation_path, observation_data)
            write_exclusive(out / "windows-rendered-seed-manifest.json", seed_manifest_data)
            write_exclusive(out / "windows-rendered-reload-manifest.json", reload_manifest_data)

            platform = {
                "schema": "picogent.v4.rendered-platform-evidence.v1",
                "candidate_sha": sha,
                "platform": "windows",
                "architecture": architecture,
                "environment": "task-owned-disposable",
                "browser": browser_id,
                "fixture": "rendered-recovery",
                "observation_sha256": sha256_file(observation_path),
                "screenshot_sha256": screenshot_set_sha,
                "observed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "verdict": verdict,
                "source_tree_modified": observation["source_tree_modified"],
            }
            platform_path = out / "windows-rendered-platform-evidence.json"
            write_exclusive(
                platform_path,
                (json.dumps(platform, indent=2, sort_keys=True) + "\n").encode("utf-8"),
            )

            summary = {
                "verdict": verdict,
                "browser": platform["browser"],
                "architecture": architecture,
                "candidate_sha": sha,
                "observation_sha256": platform["observation_sha256"],
                "platform_sha256": sha256_file(platform_path),
                "seed_manifest_sha256": observation["provenance"]["seed_manifest_sha256"],
                "reload_manifest_sha256": observation["provenance"]["reload_manifest_sha256"],
                "screenshot_set_sha256": screenshot_set_sha,
                "source_sha_verified": observation["source_sha_verified"],
                "source_tree_modified": observation["source_tree_modified"],
                "fixture_exit_verified": all(
                    (
                        observation["provenance"]["seed_fixture_exit_verified"],
                        observation["provenance"]["reload_fixture_exit_verified"],
                    )
                ),
                "required_checks": required,
            }
            write_exclusive(
                out / "windows-collection-summary.json",
                (json.dumps(summary, indent=2, sort_keys=True) + "\n").encode("utf-8"),
            )
            print(json.dumps(summary, sort_keys=True))
            return 0 if verdict == "PASS" else 1
        finally:
            cleanup_errors = []
            for phase, proc in (("seed", seed_proc), ("reload", reload_proc)):
                if proc is None:
                    continue
                try:
                    stop_fixture(proc, phase)
                except Exception as error:
                    cleanup_errors.append(f"{phase}: {error}")
            try:
                context.close()
            except Exception as error:
                cleanup_errors.append(f"browser: {error}")
            if cleanup_errors:
                raise RuntimeError(f"fixture cleanup failed: {'; '.join(cleanup_errors)}")


if __name__ == "__main__":
    sys.exit(main())
