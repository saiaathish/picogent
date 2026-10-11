"""Regression tests for safe disposable fixture preparation."""

import contextlib
import io
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import fixture


class FixturePreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="picogent-fixture-test-")
        self.addCleanup(self.temp.cleanup)

    def prepare(self, task_id, run_id):
        with patch.object(fixture.tempfile, "gettempdir", return_value=self.temp.name):
            with contextlib.redirect_stdout(io.StringIO()):
                return fixture.prepare(fixture.load_manifest(), task_id, run_id)

    def test_prepare_creates_a_fresh_directory_and_verified_copy(self):
        task = fixture.task_by_id(fixture.load_manifest(), "go-deadline")

        workspace = self.prepare(task["id"], "first-run")

        self.assertTrue(workspace.is_dir())
        self.assertEqual(fixture.tree_digest(workspace), task["initial_tree_sha256"])
        self.assertTrue(workspace.parent.name.startswith("picogent-first-run-"))
        if os.name == "posix":
            self.assertEqual(workspace.parent.stat().st_mode & 0o777, 0o700)

    def test_prepare_allocates_a_fresh_directory_for_each_invocation(self):
        first = self.prepare("go-deadline", "repeat-label")
        second = self.prepare("go-deadline", "repeat-label")

        self.assertNotEqual(first, second)
        self.assertTrue(first.is_dir())
        self.assertTrue(second.is_dir())

    def test_prepare_ignores_a_precreated_predictable_temp_root(self):
        outside = Path(self.temp.name) / "outside"
        outside.mkdir()
        predictable_root = Path(self.temp.name) / "picogent-real-task-fixtures"
        predictable_root.symlink_to(outside, target_is_directory=True)

        workspace = self.prepare("go-deadline", "isolated-run")

        self.assertFalse(workspace.is_relative_to(outside))
        self.assertEqual(list(outside.iterdir()), [])

    def test_fixture_path_rejects_a_symlinked_workspace_root(self):
        target = Path(self.temp.name) / "target"
        target.mkdir()
        symlink = Path(self.temp.name) / "linked-workspace"
        symlink.symlink_to(target, target_is_directory=True)

        with patch.object(fixture, "HERE", Path(self.temp.name)):
            with self.assertRaisesRegex(ValueError, "must not traverse symlinks"):
                fixture.fixture_path({"workspace": "linked-workspace"})

    def test_read_only_verify_does_not_resolve_the_system_temp_directory(self):
        with patch.object(
            fixture.tempfile,
            "gettempdir",
            side_effect=AssertionError("verify must not inspect temp storage"),
        ):
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertTrue(fixture.verify(fixture.load_manifest()))

    def test_tree_digest_rejects_symlinks(self):
        root = Path(self.temp.name) / "tree"
        root.mkdir()
        target = root / "target"
        target.write_text("fixture", encoding="utf-8")
        (root / "link").symlink_to(target)

        with self.assertRaisesRegex(ValueError, "symlinks are not allowed"):
            fixture.tree_digest(root)

    def test_tree_digest_rejects_a_symlinked_root(self):
        target = Path(self.temp.name) / "target"
        target.mkdir()
        (target / "content.txt").write_text("fixture", encoding="utf-8")
        root_link = Path(self.temp.name) / "root-link"
        root_link.symlink_to(target, target_is_directory=True)

        with self.assertRaisesRegex(ValueError, "symlink fixture root"):
            fixture.tree_digest(root_link)

    def test_prepare_rejects_invalid_run_ids(self):
        with self.assertRaisesRegex(ValueError, "run ID"):
            self.prepare("go-deadline", "../escape")


if __name__ == "__main__":
    unittest.main()
