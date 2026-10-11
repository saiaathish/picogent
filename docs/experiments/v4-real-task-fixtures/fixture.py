#!/usr/bin/env python3
"""Verify and safely prepare the synthetic v3/v4 comparison fixtures."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
import sys
import tempfile
from pathlib import Path


HERE = Path(__file__).resolve().parent
MANIFEST_PATH = HERE / "manifest.json"
RUNS_ROOT = Path(tempfile.gettempdir()) / "picogent-real-task-fixtures"


def tree_digest(root: Path) -> str:
    root = root.resolve(strict=True)
    digest = hashlib.sha256()
    files = sorted(root.rglob("*"), key=lambda path: path.relative_to(root).as_posix())
    regular_files = []
    for path in files:
        if path.is_symlink():
            raise ValueError(f"symlinks are not allowed in fixture trees: {path}")
        if path.is_file():
            regular_files.append(path)
        elif not path.is_dir():
            raise ValueError(f"unsupported filesystem entry in fixture tree: {path}")
    if not regular_files:
        raise ValueError(f"fixture tree is empty: {root}")
    for path in regular_files:
        relative = path.relative_to(root).as_posix().encode("utf-8")
        content = path.read_bytes()
        digest.update(len(relative).to_bytes(8, "big"))
        digest.update(relative)
        digest.update(len(content).to_bytes(8, "big"))
        digest.update(content)
    return digest.hexdigest()


def load_manifest() -> dict:
    with MANIFEST_PATH.open(encoding="utf-8") as source:
        manifest = json.load(source)
    ids = [task["id"] for task in manifest["tasks"]]
    if len(ids) != len(set(ids)):
        raise ValueError("task IDs must be unique")
    return manifest


def task_by_id(manifest: dict, task_id: str) -> dict:
    for task in manifest["tasks"]:
        if task["id"] == task_id:
            return task
    raise ValueError(f"unknown task ID: {task_id}")


def fixture_path(task: dict) -> Path:
    path = (HERE / task["workspace"]).resolve(strict=True)
    if not path.is_relative_to(HERE):
        raise ValueError(f"workspace escapes fixture root: {task['workspace']}")
    return path


def verify(manifest: dict) -> bool:
    ok = True
    for task in manifest["tasks"]:
        actual = tree_digest(fixture_path(task))
        expected = task["initial_tree_sha256"]
        matches = re.fullmatch(r"[0-9a-f]{64}", expected) is not None and actual == expected
        status = "PASS" if matches else "FAIL"
        print(f"{status} {task['id']} expected={expected} actual={actual}")
        ok = ok and matches
    return ok


def prepare(manifest: dict, task_id: str, run_id: str) -> Path:
    if re.fullmatch(r"[a-z0-9][a-z0-9-]{0,47}", run_id) is None:
        raise ValueError("run ID must be 1-48 lowercase letters, digits, or hyphens")
    task = task_by_id(manifest, task_id)
    source = fixture_path(task)
    expected = task["initial_tree_sha256"]
    actual = tree_digest(source)
    if actual != expected:
        raise ValueError(f"pristine fixture digest mismatch for {task_id}: {actual}")

    temp_root = Path(tempfile.gettempdir()).resolve(strict=True)
    if RUNS_ROOT.is_symlink():
        raise ValueError(f"temporary fixture root must not be a symlink: {RUNS_ROOT}")
    resolved_runs_root = RUNS_ROOT.resolve(strict=False)
    if not resolved_runs_root.is_relative_to(temp_root) or resolved_runs_root == temp_root:
        raise ValueError(f"temporary fixture root escapes the system temp directory: {RUNS_ROOT}")
    run_root = RUNS_ROOT / run_id
    if run_root.exists() or run_root.is_symlink():
        raise FileExistsError(f"refusing to reuse existing run directory: {run_root}")
    run_root.mkdir(parents=True, exist_ok=False)
    destination = run_root / "workspace"
    shutil.copytree(source, destination, symlinks=False)
    copied_digest = tree_digest(destination)
    if copied_digest != expected:
        raise ValueError(f"prepared copy digest mismatch: {copied_digest}")
    print(destination)
    return destination


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("verify", help="verify all committed pristine fixture digests")
    prepare_parser = subparsers.add_parser("prepare", help="copy one pristine fixture to a new temp run")
    prepare_parser.add_argument("task_id")
    prepare_parser.add_argument("--run-id", required=True)
    args = parser.parse_args()

    try:
        manifest = load_manifest()
        if args.command == "verify":
            return 0 if verify(manifest) else 1
        prepare(manifest, args.task_id, args.run_id)
        return 0
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f"fixture error: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
