#!/usr/bin/env python3
"""Build offline alpha packages from a verified, prebuilt appliance (build tool only)."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent.parent
VERSION = "0.1.0-alpha.1"


def run(*args, **kwargs):
    subprocess.run(args, check=True, **kwargs)


def copy(source, target):
    # APFS copy-on-write avoids another 10 GiB allocation on the build Mac.
    if os.uname().sysname == "Darwin":
        run("cp", "-c", str(source), str(target))
    else:
        run("cp", "--reflink=auto", "--sparse=always", str(source), str(target))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("platform", choices=["macos", "linux"])
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "dist/releases")
    args = parser.parse_args()
    bundle = args.bundle.resolve()
    manifest = json.loads((bundle / "manifest.json").read_text())
    for name in [manifest["kernel"], manifest["initrd"], "system.img", "data.img"]:
        if name not in manifest["files"]:
            raise ValueError(f"Missing checksum: {name}")
    for name, digest in manifest["files"].items():
        if Path(name).name != name:
            raise ValueError("Invalid bundle filename")
        with (bundle / name).open("rb") as f:
            if hashlib.file_digest(f, "sha256").hexdigest() != digest:
                raise ValueError(f"Bundle checksum mismatch: {name}")
    arch = manifest["arch"]
    out = args.output.resolve()
    out.mkdir(parents=True, exist_ok=True)
    stage = out / f"stage-{args.platform}-{arch}"
    stage.mkdir()  # Refuse to overwrite an earlier build.
    if args.platform == "macos":
        if os.uname().sysname != "Darwin" or arch != "arm64":
            raise ValueError("The first macOS alpha requires an Apple Silicon build host and bundle")
        app = stage / "CARE Anywhere.app"
        contents = app / "Contents"
        exe = contents / "MacOS/care-anywhere"
        resources = contents / "Resources"
        exe.parent.mkdir(parents=True)
        resources.mkdir()
        dest = resources / "bundle"
        run("go", "build", "-trimpath", "-o", str(exe), ".", cwd=ROOT)
        (contents / "Info.plist").write_bytes(plistlib.dumps({
            "CFBundleName": "CARE Anywhere", "CFBundleDisplayName": "CARE Anywhere",
            "CFBundleIdentifier": "network.ohc.care-anywhere", "CFBundleVersion": "1",
            "CFBundleShortVersionString": "0.1.0", "CFBundleExecutable": "care-anywhere",
            "CFBundlePackageType": "APPL", "LSMinimumSystemVersion": "13.0",
            "LSUIElement": True, "NSHighResolutionCapable": True,
            "CFBundleIconFile": "AppIcon", "LSApplicationCategoryType": "public.app-category.medical",
        }))
        run("swift", str(ROOT / "scripts/icon.swift"), str(resources / "AppIcon.iconset"))
        run("iconutil", "-c", "icns", str(resources / "AppIcon.iconset"), "-o", str(resources / "AppIcon.icns"))
        shutil.rmtree(resources / "AppIcon.iconset")
        (stage / "Applications").symlink_to("/Applications")
        (stage / "Start here.txt").write_text(
            "CARE Anywhere — Apple Silicon alpha\n\n"
            "1. Drag CARE Anywhere to Applications.\n2. Eject this disk image.\n"
            "3. Open CARE Anywhere from Applications. Your browser opens automatically.\n\n"
            "This alpha is not notarized. After the first blocked launch, use System Settings > "
            "Privacy & Security > Open Anyway, then open the app again. Do not disable Gatekeeper.\n\n"
            "Requires macOS 13+, Apple Silicon, 8 GB RAM and 22 GB free for app + clinic.\n"
            "Initial preparation can take several minutes. Use test records for this alpha.\n"
            "Closing the browser keeps CARE running. Stop or quit from its control panel.\n"
            "Removing the app preserves clinic data in ~/Library/Application Support/care-anywhere.\n"
        )
    else:
        if "firecracker" not in manifest["files"]:
            raise ValueError("Linux bundle must include checksum-verified Firecracker")
        app = stage / "care-anywhere"
        app.mkdir()
        resources = app
        dest = app / "bundle"
        env = dict(os.environ, GOOS="linux", GOARCH=arch, CGO_ENABLED="0")
        run("go", "build", "-trimpath", "-o", str(app / "care-anywhere"), ".", cwd=ROOT, env=env)
        shutil.copy2(ROOT / "scripts/install-linux.sh", app / "install.sh")
        (app / "install.sh").chmod(0o755)
        shutil.copy2(ROOT / "scripts/care-anywhere.svg", app / "care-anywhere.svg")
        (app / "README.txt").write_text(
            "CARE Anywhere Linux alpha\n\nExtract this archive, then run ./install.sh once.\n"
            "Launch CARE Anywhere from your applications menu. Everything runs locally.\n"
            "Requires 8 GB RAM, KVM access, xdg-open, a browser, and 22 GB disk space.\n"
            "If /dev/kvm access is denied, ask your administrator to grant your user KVM access.\n"
            "Never run the desktop app with sudo.\n"
            "Data is in ${XDG_CONFIG_HOME:-~/.config}/care-anywhere and survives uninstall.\n"
            "Stop CARE before removing ~/.local/opt/care-anywhere and its applications-menu entry.\n"
        )
    dest.mkdir()
    shutil.copytree(ROOT / "frontend/dist", resources / "web")
    shutil.copytree(ROOT / "dist/onboarding", resources / "onboarding")
    for name in ["manifest.json", *manifest["files"]]:
        copy(bundle / name, dest / name)
    for name in ["LICENSE", "THIRD_PARTY_NOTICES.md", "README.md", "flake.lock"]:
        shutil.copy2(ROOT / name, resources / name)
    if args.platform == "macos":
        identity = os.environ.get("MACOS_SIGN_IDENTITY", "-")
        run("codesign", "--force", "--sign", identity, "--options", "runtime", "--entitlements", str(ROOT / "entitlements.plist"), str(app))
        run("codesign", "--verify", "--deep", "--strict", str(app))
        artifact = out / f"CARE-Anywhere-{VERSION}-macos-{arch}.dmg"
        run("hdiutil", "create", "-volname", "CARE Anywhere", "-srcfolder", str(stage), "-format", "UDZO", "-ov", str(artifact))
        run("hdiutil", "verify", str(artifact))
        if identity != "-" and os.environ.get("MACOS_NOTARY_PROFILE"):
            run("xcrun", "notarytool", "submit", str(artifact), "--keychain-profile", os.environ["MACOS_NOTARY_PROFILE"], "--wait")
            run("xcrun", "stapler", "staple", str(artifact))
    else:
        artifact = out / f"CARE-Anywhere-{VERSION}-linux-{arch}.tar.gz"
        with tarfile.open(artifact, "w:gz") as tf:
            tf.add(app, arcname="care-anywhere")
    with artifact.open("rb") as f:
        digest = hashlib.file_digest(f, "sha256").hexdigest()
    artifact.with_suffix(artifact.suffix + ".sha256").write_text(f"{digest}  {artifact.name}\n")
    print(artifact)


if __name__ == "__main__":
    main()
