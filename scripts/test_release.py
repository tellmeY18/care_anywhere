"""Release trust boundaries and interrupted-upload retry checks (stdlib only)."""
import hashlib
from pathlib import Path
import tempfile
import unittest

from release import PLATFORMS, REQUIRED_JOBS, missing_assets, package_version, validate_build, verify_assets


class ReleaseTests(unittest.TestCase):
    def test_build_must_be_complete_main_build(self):
        run = dict(status="completed", conclusion="success", head_branch="main", event="push", path=".github/workflows/alpha.yml")
        jobs = [dict(name=n, conclusion="success") for n in REQUIRED_JOBS]
        validate_build(run, jobs)
        for key, value in (("conclusion", "failure"), ("head_branch", "debug/windows"), ("event", "pull_request"), ("path", "other.yml")):
            with self.assertRaises(ValueError):
                validate_build(dict(run, **{key: value}), jobs)
        with self.assertRaises(ValueError):
            validate_build(run, jobs[:-1])

    def test_retries_never_replace_assets(self):
        digests = {"a.exe": "sha256:abc", "a.exe.sha256": "sha256:def"}
        existing = [dict(name="a.exe", digest="sha256:abc", state="uploaded")]
        self.assertEqual(missing_assets(existing, digests, True), ["a.exe.sha256"])
        with self.assertRaises(ValueError):
            missing_assets(existing, digests, False)
        with self.assertRaises(ValueError):
            missing_assets([dict(existing[0], digest="sha256:wrong")], digests, True)
        existing.append(dict(name="a.exe.sha256", digest="sha256:def", state="uploaded"))
        self.assertEqual(missing_assets(existing, digests, False), [])

    def test_exact_assets_and_tampering(self):
        version = "0.1.0-alpha.123"
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for platform, extension in PLATFORMS.items():
                name = f"CARE-Anywhere-{version}-{platform}.{extension}"
                (root / name).write_bytes(b"test package")
                (root / (name + ".sha256")).write_text(hashlib.sha256(b"test package").hexdigest() + "  " + name + "\n")
            self.assertEqual(len(verify_assets(root, version)), 8)
            (root / "unexpected.log").write_text("diagnostics")
            with self.assertRaises(ValueError):
                verify_assets(root, version)
            (root / "unexpected.log").unlink()
            (root / name).write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                verify_assets(root, version)

    def test_versions_are_safe_filenames(self):
        self.assertEqual(package_version("0.2.0-alpha.123"), "0.2.0-alpha.123")
        for value in ("../escape", "0.1.0-alpha.0", "0.1.0-alpha.1\n", "0.1.0;exit"):
            with self.assertRaises(ValueError):
                package_version(value)
