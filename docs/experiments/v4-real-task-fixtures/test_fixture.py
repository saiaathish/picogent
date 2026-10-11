"""Regression tests for safe disposable fixture preparation."""

import contextlib
import io
import tempfile
import unittest
from pathlib import Path

import fixture


class FixturePreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="picogent-fixture-test-")
        self.addCleanup(self.temp.cleanup)
        self.original_runs_root = fixture.RUNS_ROOT
        self.addCleanup(setattr, fixture, "RUNS_ROOT", self.original_runs_root)

    def test_prepare_creates_a_new_private_parent_and_verified_copy(self):
        fixture.RUNS_ROOT = Path(self.temp.name) / "new-runs-root"
        task = fixture.task_by_id(fixture.load_manifest(), "go-deadline")

        with contextlib.redirect_stdout(io.StringIO()):
            workspace = fixture.prepare(fixture.load_manifest(), task["id"], "first-run")

        self.assertTrue(workspace.is_dir())
        self.assertEqual(fixture.tree_digest(workspace), task["initial_tree_sha256"])
        if hasattr(Path(fixture.RUNS_ROOT), "stat"):
            self.assertEqual(Path(fixture.RUNS_ROOT).stat().st_mode & 0o777, 0o700)

    def test_prepare_refuses_to_reuse_a_run_id(self):
        fixture.RUNS_ROOT = Path(self.temp.name) / "runs"
        fixture.RUNS_ROOT.mkdir(mode=0o700)
        (fixture.RUNS_ROOT / "same-run").mkdir()

        with self.assertRaises(FileExistsError):
            fixture.prepare(fixture.load_manifest(), "go-deadline", "same-run")

    def test_prepare_rejects_a_symlinked_runs_root_without_writing_through_it(self):
        outside = Path(self.temp.name) / "outside"
        outside.mkdir()
        fixture.RUNS_ROOT = Path(self.temp.name) / "runs-link"
        fixture.RUNS_ROOT.symlink_to(outside, target_is_directory=True)

        with self.assertRaisesRegex(ValueError, "must not be a symlink"):
            fixture.prepare(fixture.load_manifest(), "go-deadline", "blocked-run")

        self.assertEqual(list(outside.iterdir()), [])

    def test_tree_digest_rejects_symlinks(self):
        root = Path(self.temp.name) / "tree"
        root.mkdir()
        target = root / "target"
        target.write_text("fixture", encoding="utf-8")
        (root / "link").symlink_to(target)

        with self.assertRaisesRegex(ValueError, "symlinks are not allowed"):
            fixture.tree_digest(root)

    def test_prepare_rejects_invalid_run_ids(self):
        fixture.RUNS_ROOT = Path(self.temp.name) / "runs"

        with self.assertRaisesRegex(ValueError, "run ID"):
            fixture.prepare(fixture.load_manifest(), "go-deadline", "../escape")


if __name__ == "__main__":
    unittest.main()
