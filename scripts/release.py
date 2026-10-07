#!/usr/bin/env python3
"""Version packages and promote one tested Actions run; never rebuild artifacts."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parent.parent
PLATFORMS = {
    "linux-amd64": "AppImage", "linux-arm64": "AppImage",
    "macos-arm64": "dmg", "windows-amd64": "exe",
}
OTA_FILES = ["manifest.json", "kernel", "initrd", "system.img", "runtime.img", "app.img", "data.img"]
REQUIRED_JOBS = {"checks", "appliance-amd64", "appliance-arm64", "macos", "windows"}


def package_version(value):
    if not re.fullmatch(r"\d+\.\d+\.\d+-(?:alpha\.[1-9]\d*|dev)", value):
        raise ValueError(f"Invalid package version: {value!r}")
    return value


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True).strip()


def api(path):
    return json.loads(gh("api", f"repos/{os.environ['GITHUB_REPOSITORY']}/{path}"))


def pages(path, key):
    result = []
    page = 1
    while True:
        data = api(f"{path}?per_page=100&page={page}")
        items = data[key] if key else data
        result.extend(items)
        if len(items) < 100:
            return result
        page += 1


def validate_build(run, jobs):
    if (run["status"] != "completed" or run["conclusion"] != "success"
            or run["head_branch"] != "main" or run["event"] not in {"push", "workflow_dispatch"}
            or run["path"] != ".github/workflows/alpha.yml"):
        raise ValueError("Choose a successful main-branch alpha build, not a PR or diagnostic run")
    passed = {job["name"] for job in jobs if job["conclusion"] == "success"}
    if not REQUIRED_JOBS <= passed:
        raise ValueError(f"Missing acceptance jobs: {sorted(REQUIRED_JOBS - passed)}")


def verify_assets(directory, version):
    names = {f"CARE-Anywhere-{version}-{platform}.{extension}" for platform, extension in PLATFORMS.items()}
    if not version.startswith("0.1."):
        names |= {f"{arch}-{name}" for arch in ("amd64", "arm64") for name in OTA_FILES}
    expected = names | {name + ".sha256" for name in names}
    if {p.name for p in directory.iterdir()} != expected:
        raise ValueError("Release must contain the exact platform packages, OTA layers and checksum sidecars")
    digests = {}
    for name in sorted(expected):
        path = directory / name
        if not path.is_file() or path.is_symlink():
            raise ValueError(f"Invalid release file: {name}")
        with path.open("rb") as f:
            digests[name] = "sha256:" + hashlib.file_digest(f, "sha256").hexdigest()
    for name in names:
        digest, filename = (directory / (name + ".sha256")).read_text().split()
        if filename != name or "sha256:" + digest != digests[name]:
            raise ValueError(f"Release checksum mismatch: {name}")
    if not version.startswith("0.1."):
        for arch in ("amd64", "arm64"):
            manifest = json.loads((directory / f"{arch}-manifest.json").read_text())
            if manifest.get("format") != 2 or manifest.get("arch") != arch or set(manifest["files"]) != set(OTA_FILES) - {"manifest.json"}:
                raise ValueError("Invalid OTA manifest")
            for name, digest in manifest["files"].items():
                if digests[f"{arch}-{name}"] != "sha256:" + digest:
                    raise ValueError("OTA layer does not match its manifest")
    return digests


def missing_assets(existing, digests, draft):
    """Resume identical uploads; never clobber an asset or a published release."""
    seen = set()
    for asset in existing:
        name = asset["name"]
        if name in seen or name not in digests or asset.get("digest") != digests[name] or asset["state"] != "uploaded":
            raise ValueError(f"Existing release asset differs or is incomplete: {name}")
        seen.add(name)
    missing = set(digests) - seen
    if missing and not draft:
        raise ValueError("Published release is incomplete; refusing to modify it")
    return sorted(missing)


def publish(run_id):
    if not re.fullmatch(r"[1-9]\d*", run_id):
        raise ValueError("Build run ID must be a positive integer")
    run = api(f"actions/runs/{run_id}")
    if run["head_repository"]["full_name"] != os.environ["GITHUB_REPOSITORY"]:
        raise ValueError("Build source must belong to this repository")
    validate_build(run, pages(f"actions/runs/{run_id}/jobs", "jobs"))
    sha = run["head_sha"]
    repo = os.environ["GITHUB_REPOSITORY"]
    # Read version and notes from the tested commit, not the publisher's checkout.
    base = gh("api", f"repos/{repo}/contents/VERSION?ref={sha}", "-H", "Accept: application/vnd.github.raw+json").strip()
    version = package_version(f"{base}-alpha.{run['run_number']}")
    tag = "v" + version
    artifacts = pages(f"actions/runs/{run_id}/artifacts", "artifacts")
    artifact_names = list(PLATFORMS)
    if not version.startswith("0.1."):
        artifact_names += ["ota-amd64", "ota-arm64"]
    for platform in artifact_names:
        matches = [a for a in artifacts if a["name"] == platform and not a["expired"]]
        if len(matches) != 1:
            raise ValueError(f"Missing, expired or ambiguous artifact: {platform}")
    with tempfile.TemporaryDirectory() as tmp:
        directory = Path(tmp) / "assets"
        directory.mkdir()
        for platform in artifact_names:
            subprocess.run(["gh", "run", "download", run_id, "--repo", repo, "--name", platform, "--dir", str(directory)], check=True)
        digests = verify_assets(directory, version)
        template = gh("api", f"repos/{repo}/contents/.github/release-notes.md?ref={sha}", "-H", "Accept: application/vnd.github.raw+json")
        notes = Path(tmp) / "notes.md"
        notes.write_text(template.replace("{{VERSION}}", version) + f"\n\n## Build provenance\n\nSource: `{sha}`.\n[Tested build]({run['html_url']}) (run {run['run_number']}).\nAll four packages are from this run; publication does not rebuild them.\n", encoding="utf-8")
        # Refuse moving/reusing a tag for another commit. Creating the ref
        # separately also gives permission failures before creating a draft.
        tags = pages("git/matching-refs/tags/" + tag, None)
        exact = [t for t in tags if t["ref"] == "refs/tags/" + tag]
        if exact:
            if exact[0]["object"]["type"] != "commit" or exact[0]["object"]["sha"] != sha:
                raise ValueError("Release tag does not point to the tested commit")
        else:
            gh("api", f"repos/{repo}/git/refs", "-f", "ref=refs/tags/" + tag, "-f", "sha=" + sha)
        releases = pages("releases", None)
        release = next((r for r in releases if r["tag_name"] == tag), None)
        if release is None:
            gh("release", "create", tag, "--repo", repo, "--verify-tag", "--draft", "--prerelease",
               "--title", "CARE Anywhere " + version, "--notes-file", str(notes))
            # The release list is eventually consistent right after creation.
            for _ in range(10):
                release = next((r for r in pages("releases", None) if r["tag_name"] == tag), None)
                if release:
                    break
                time.sleep(3)
            else:
                raise ValueError("Created draft release did not appear in the release list")
        if not release["prerelease"]:
            raise ValueError("Refusing to modify a non-alpha release")
        existing = pages(f"releases/{release['id']}/assets", None)
        for name in missing_assets(existing, digests, release["draft"]):
            gh("release", "upload", tag, str(directory / name), "--repo", repo)
        # Confirm server-side digests and completeness before making it public.
        missing_assets(pages(f"releases/{release['id']}/assets", None), digests, False)
        if release["draft"] and os.environ.get("PUBLISH") == "true":
            gh("release", "edit", tag, "--repo", repo, "--draft=false", "--prerelease", "--notes-file", str(notes))
        url = gh("release", "view", tag, "--repo", repo, "--json", "url", "--jq", ".url")
        print(url)
        if os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as f:
                f.write(f"Verified four packages and checksums: [{tag}]({url})\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("version").add_argument("--number", required=True)
    commands.add_parser("publish").add_argument("--run-id", required=True)
    args = parser.parse_args()
    if args.command == "version":
        print("version=" + package_version((ROOT / "VERSION").read_text().strip() + "-alpha." + args.number))
    else:
        publish(args.run_id)
