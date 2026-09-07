#!/usr/bin/env python3
"""Focused fail-closed tests for the Windows rendered evidence collector."""

from __future__ import annotations

import importlib.util
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest


SCRIPT = pathlib.Path(__file__).with_name("collect_owned_browser.py")
SPEC = importlib.util.spec_from_file_location("collect_owned_browser", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
COLLECTOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(COLLECTOR)


class CollectorBoundaryTests(unittest.TestCase):
    def test_platform_is_explicit_and_bounded(self) -> None:
        self.assertEqual(COLLECTOR.normalize_platform("linux"), "linux")
        self.assertEqual(COLLECTOR.normalize_platform(" DARWIN "), "darwin")
        with self.assertRaisesRegex(SystemExit, "platform must be one of"):
            COLLECTOR.normalize_platform("android")

    def test_default_home_root_uses_platform(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            temp_root = pathlib.Path(os.path.realpath(temporary))
            home_root = COLLECTOR.prepare_fixture_home(None, temp_root, "linux")
            self.assertEqual(home_root, temp_root / "picogent-rendered-linux-507")

    def test_output_directory_must_be_outside_checkout_without_symlinks(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(os.path.realpath(temporary))
            checkout = root / "checkout"
            checkout.mkdir()

            output = COLLECTOR.prepare_output_directory(root / "artifacts", checkout)
            self.assertTrue(output.is_dir())

            with self.assertRaisesRegex(SystemExit, "outside the source checkout"):
                COLLECTOR.prepare_output_directory(checkout / "artifacts", checkout)

            link = root / "output-link"
            try:
                link.symlink_to(root / "redirected", target_is_directory=True)
            except OSError as error:
                self.skipTest(f"symlink creation unavailable: {error}")
            with self.assertRaisesRegex(SystemExit, "symbolic-link components"):
                COLLECTOR.prepare_output_directory(link / "artifacts", checkout)

    def test_fixture_home_is_reset_only_below_temp(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            temp_root = pathlib.Path(os.path.realpath(temporary))
            home = temp_root / "fixture-home"
            home.mkdir()
            (home / "stale").write_text("stale", encoding="utf-8")

            prepared = COLLECTOR.prepare_fixture_home(home, temp_root)
            self.assertEqual(prepared, home)
            self.assertTrue(prepared.is_dir())
            self.assertFalse((prepared / "stale").exists())

            with self.assertRaisesRegex(SystemExit, "child of the temp directory"):
                COLLECTOR.prepare_fixture_home(temp_root.parent / "outside", temp_root)

    def test_exclusive_write_rejects_a_symlink_target(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(os.path.realpath(temporary))
            target = root / "target"
            target.write_bytes(b"original")
            link = root / "link"
            try:
                link.symlink_to(target)
            except OSError as error:
                self.skipTest(f"symlink creation unavailable: {error}")

            with self.assertRaises(FileExistsError):
                COLLECTOR.write_exclusive(link, b"replacement")
            self.assertEqual(target.read_bytes(), b"original")

    def test_stop_fixture_waits_for_exit(self) -> None:
        proc = subprocess.Popen(
            [sys.executable, "-c", "import time; time.sleep(30)"],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        result = COLLECTOR.stop_fixture(proc, "test")
        self.assertTrue(result["exited"])
        self.assertIsNotNone(proc.poll())


if __name__ == "__main__":
    unittest.main()
