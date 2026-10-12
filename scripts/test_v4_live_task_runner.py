import base64
import json
import os
import pathlib
import sys
import tempfile
import time
import unittest


sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import v4_live_task_runner as runner


def fake_jwt(expires_at: int) -> str:
    header = base64.urlsafe_b64encode(b'{"alg":"none"}').decode("ascii").rstrip("=")
    payload = base64.urlsafe_b64encode(json.dumps({"exp": expires_at}).encode("ascii")).decode("ascii").rstrip("=")
    return f"{header}.{payload}.signature"


class CredentialTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="picogent-runner-test-")
        self.root = pathlib.Path(self.temp.name)
        self.source = self.root / ".codex" / "auth.json"
        self.source.parent.mkdir(mode=0o700)
        self.access_token = fake_jwt(int(time.time()) + 3600)
        with self.source.open("x", encoding="utf-8") as output:
            json.dump(
                {
                    "tokens": {
                        "access_token": self.access_token,
                        "refresh_token": "refresh-must-not-copy",
                        "account_id": "account-test",
                        "id_token": "id-token-must-not-copy",
                    }
                },
                output,
            )
        os.chmod(self.source, 0o600)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_preflight_extracts_only_access_credential(self) -> None:
        credential = runner.read_codex_credential(self.source, minimum_validity_seconds=30)
        self.assertEqual(credential["access_token"], self.access_token)
        self.assertEqual(credential["account_id"], "account-test")
        self.assertNotIn("refresh_token", credential)
        self.assertNotIn("id_token", credential)

    def test_preflight_rejects_token_that_expires_too_soon(self) -> None:
        with self.assertRaises(runner.RunnerError):
            runner.read_codex_credential(self.source, minimum_validity_seconds=3601)

    def test_preflight_rejects_non_private_source_file(self) -> None:
        os.chmod(self.source, 0o644)
        with self.assertRaises(runner.RunnerError):
            runner.read_codex_credential(self.source, minimum_validity_seconds=1)

    def test_host_auth_path_rejects_symlinked_directory_and_file(self) -> None:
        original_dir = self.root / ".codex-original"
        self.source.parent.rename(original_dir)
        symlinked_dir = self.root / ".codex"
        symlinked_dir.symlink_to(original_dir, target_is_directory=True)
        with self.assertRaises(runner.RunnerError):
            runner.host_codex_auth_path(self.root)

        symlinked_dir.unlink()
        original_dir.rename(symlinked_dir)
        auth_target = self.root / "auth-target.json"
        auth_target.write_text("{}", encoding="utf-8")
        self.source.unlink()
        self.source.symlink_to(auth_target)
        with self.assertRaises(runner.RunnerError):
            runner.host_codex_auth_path(self.root)

    def test_private_copy_contains_no_refresh_or_id_token(self) -> None:
        credential = runner.read_codex_credential(self.source, minimum_validity_seconds=30)
        destination = self.root / "run" / "codex-home" / "auth.json"
        runner.write_access_only_auth_file(credential, destination)
        stored = json.loads(destination.read_text(encoding="utf-8"))
        self.assertEqual(stored, {"tokens": {"access_token": self.access_token, "account_id": "account-test"}})
        self.assertEqual(destination.stat().st_mode & 0o777, 0o600)


class FixtureLoadingTests(unittest.TestCase):
    def test_loads_fixture_helper_from_script_relative_path(self) -> None:
        fixture = runner.load_fixture_module()
        manifest = fixture.load_manifest()
        self.assertEqual(len(manifest["tasks"]), 3)
        self.assertEqual(fixture.MANIFEST_PATH, runner.FIXTURE_DIR / "manifest.json")


class CandidateSourceMappingTests(unittest.TestCase):
    def test_randomized_candidate_order_maps_to_exact_variant_shas(self) -> None:
        v3_sha = runner.V3_SHA
        v4_sha = runner.V4_SHA
        forward = runner.candidate_source_mapping([("v3", v3_sha), ("v4", v4_sha)])
        reverse = runner.candidate_source_mapping([("v4", v4_sha), ("v3", v3_sha)])

        self.assertEqual(forward["A"], {"variant": "v3", "source_sha": v3_sha})
        self.assertEqual(forward["B"], {"variant": "v4", "source_sha": v4_sha})
        self.assertEqual(reverse["A"], {"variant": "v4", "source_sha": v4_sha})
        self.assertEqual(reverse["B"], {"variant": "v3", "source_sha": v3_sha})

    def test_candidate_source_mapping_rejects_incomplete_or_duplicate_variants(self) -> None:
        with self.assertRaises(runner.RunnerError):
            runner.candidate_source_mapping([("v3", runner.V3_SHA), ("v3", runner.V3_SHA)])
        with self.assertRaises(runner.RunnerError):
            runner.candidate_source_mapping([("v3", "a" * 40), ("v4", runner.V4_SHA)])


class ChildObservationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="picogent-runner-child-")
        self.root = pathlib.Path(self.temp.name)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_final_tool_start_marker_is_counted_once(self) -> None:
        child = self.root / "emit-marker.sh"
        child.write_text("#!/bin/sh\nprintf '→ fake-tool' >&2\n", encoding="utf-8")
        child.chmod(0o700)
        observed = runner.observe_child(
            [str(child)],
            cwd=self.root,
            env=os.environ.copy(),
            timeout_seconds=5,
            observed_tool_start_stop_threshold=1,
        )
        self.assertEqual(observed["tool_starts_observed"], 1)
        self.assertIsNone(observed["stop_reason"])

    def test_provider_error_artifact_uses_only_a_category(self) -> None:
        self.assertEqual(runner.classify_agent_error(b"codex http 401: unauthorized"), "provider_unauthorized")
        self.assertEqual(runner.classify_agent_error(b"HTTP 429 rate limit"), "provider_rate_limited")
        self.assertEqual(runner.classify_agent_error(b"connection refused"), "provider_transport_failure")
        self.assertEqual(runner.classify_agent_error(b""), "no_diagnostic")


class RuntimeMetadataTests(unittest.TestCase):
    def test_only_known_picogent_runtime_paths_are_excluded_from_candidate_changes(self) -> None:
        known = (
            ".picogent/project-run/.run.lock",
            ".picogent/workspace-instance",
            ".picogent/workspace-instance.lock",
            ".picogent/undo/headless-6245dbc1642a796d.json",
        )
        for path in known:
            with self.subTest(path=path):
                self.assertTrue(runner.is_runtime_metadata_path(path))
        for path in (".picogent/notes.txt", ".picogent/undo/user.json", "due/due.go"):
            with self.subTest(path=path):
                self.assertFalse(runner.is_runtime_metadata_path(path))


class SandboxProfileTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="picogent-runner-sandbox-")
        self.root = pathlib.Path(self.temp.name)
        self.run_root = self.root / "run-root"
        self.run_root.mkdir(mode=0o700)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_sensitive_directory_deny_precedes_broad_read_allow(self) -> None:
        denied = self.root / ".codex"
        denied.mkdir()
        profile = runner.sandbox_profile(
            self.run_root,
            denied_reads=(denied,),
            denied_writes=(denied,),
            allow_network=False,
        )
        self.assertLess(profile.index("(deny file-read*"), profile.index("(allow file-read*)"))
        allowed_run_root = f'(allow file-write* (subpath "{self.run_root.resolve()}"))'
        self.assertLess(profile.index(allowed_run_root), profile.index("(deny file-write*"))
        self.assertIn(f'(deny file-read* (subpath "{denied.resolve()}"))', profile)
        self.assertIn(f'(deny file-write* (subpath "{denied.resolve()}"))', profile)
        self.assertNotIn("(allow network*)", profile)
        self.assertNotIn("mach-lookup*", profile)

    def test_system_trust_lookup_is_narrow_and_opt_in(self) -> None:
        default_profile = runner.sandbox_profile(self.run_root, allow_network=True)
        trusted_profile = runner.sandbox_profile(self.run_root, allow_network=True, allow_macos_trust=True)
        self.assertNotIn("mach-lookup", default_profile)
        self.assertIn('(allow mach-lookup (global-name "com.apple.trustd.agent"))', trusted_profile)
        self.assertNotIn("mach-lookup*", trusted_profile)
        with self.assertRaises(runner.RunnerError):
            runner.sandbox_profile(self.run_root, allow_network=False, allow_macos_trust=True)

    def test_shell_environment_drops_auth_and_user_provider_settings(self) -> None:
        env = {
            "HOME": str(self.run_root / "home"),
            "TMPDIR": str(self.run_root / "tmp"),
            "GOMODCACHE": str(self.root / "module-cache"),
            "PICOGENT_CODEX_HOME": str(self.root / "codex-home"),
            "OPENAI_API_KEY": "not-forwarded",
            "PICOGENT_PROVIDER": "codex",
        }
        child = runner.shell_tool_env(env)
        self.assertEqual(child["HOME"], env["HOME"])
        self.assertEqual(child["GOMODCACHE"], env["GOMODCACHE"])
        self.assertNotIn("PICOGENT_CODEX_HOME", child)
        self.assertNotIn("OPENAI_API_KEY", child)
        self.assertNotIn("PICOGENT_PROVIDER", child)

    def test_input_digest_binds_prompt_model_budget_and_fixture(self) -> None:
        task = {"id": "sample", "prompt": "do work", "allowed_paths": ["main.go"]}
        budgets = {"max_tool_rounds": 3}
        base = runner.task_input_digest(
            task,
            "model",
            budgets,
            initial_tree_sha256="tree-a",
        )
        self.assertEqual(
            base,
            runner.task_input_digest(
                task,
                "model",
                budgets,
                initial_tree_sha256="tree-a",
            ),
        )
        self.assertNotEqual(
            base,
            runner.task_input_digest(
                task,
                "different-model",
                budgets,
                initial_tree_sha256="tree-a",
            ),
        )
        self.assertNotEqual(
            base,
            runner.task_input_digest(
                task,
                "model",
                budgets,
                initial_tree_sha256="tree-b",
            ),
        )

    @unittest.skipUnless(sys.platform == "darwin" and pathlib.Path("/usr/bin/sandbox-exec").is_file(), "macOS sandbox required")
    def test_nested_shell_canary_denies_auth_network_and_outside_writes(self) -> None:
        source_auth = self.root / "host" / ".codex" / "auth.json"
        source_auth.parent.mkdir(parents=True)
        copied_auth = self.run_root / "codex-home" / "auth.json"
        copied_auth.parent.mkdir()
        for path in (source_auth, copied_auth):
            path.write_text('{"tokens":{"access_token":"probe"}}', encoding="utf-8")
            os.chmod(path, 0o600)

        home = self.run_root / "home"
        tmp = self.run_root / "tmp"
        home.mkdir()
        tmp.mkdir()
        env = {"HOME": str(home), "TMPDIR": str(tmp), "GOMODCACHE": str(self.root / "module-cache")}
        profile = self.root / "profiles" / "shell.sbpl"
        profile.parent.mkdir()
        profile.write_text(
            runner.sandbox_profile(
                self.run_root,
                denied_reads=(source_auth.parent, copied_auth.parent),
                denied_writes=(source_auth.parent, copied_auth.parent),
                allow_network=False,
            ),
            encoding="utf-8",
        )
        wrapper = self.root / "tool-bin" / "bash"
        runner.write_shell_wrapper(
            wrapper,
            sandbox_exec="/usr/bin/sandbox-exec",
            profile_path=profile,
            env=env,
        )
        self.assertTrue(
            runner.probe_shell_sandbox(
                self.run_root,
                wrapper=wrapper,
                env=env,
                auth_paths=(source_auth, copied_auth),
            )
        )


if __name__ == "__main__":
    unittest.main()
