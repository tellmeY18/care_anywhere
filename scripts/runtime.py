#!/usr/bin/env python3
"""Stage QEMU and its native dependencies on CI, never on the user's computer."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import urllib.request


def download(url, dest, digest, algorithm="sha256"):
    urllib.request.urlretrieve(url, dest)
    with dest.open("rb") as f:
        if hashlib.file_digest(f, algorithm).hexdigest() != digest:
            raise ValueError(f"Checksum mismatch: {url}")


def stage_runtime(dest, arch):
    dest.mkdir()
    if os.name == "nt":
        installer = dest.parent / "qemu-setup.exe"
        download("https://qemu.weilnetz.de/w64/2026/qemu-w64-setup-20260811.exe", installer,
                 "f37d9ee8f498879b7ca92a9959d30ce7971f9894f584de574f2ac6f5ba925a7265e9d8556d0f56bc75e4e384fc7cac6baed78a89c2e42cdbef109724f01ec16c", "sha512")
        subprocess.run(["7z", "x", str(installer), "-o" + str(dest), "-y"], check=True)
        installer.unlink()
        # QEMU's Windows distribution keeps firmware at its top level.
        firmware = dest / "share/qemu"
        firmware.mkdir(parents=True, exist_ok=True)
        for pattern in ("*.bin", "*.rom", "*.dtb", "*.fd"):
            for p in dest.glob(pattern):
                shutil.copy2(p, firmware / p.name)
    else:
        name = "qemu-system-" + ("aarch64" if arch == "arm64" else "x86_64")
        binary = Path(shutil.which(name))
        shutil.copy2(binary, dest / "qemu")
        libs = dest / "lib"
        libs.mkdir()
        output = subprocess.check_output(["ldd", str(binary)], text=True)
        for path in re.findall(r"(/[^\s()]+)", output):
            shutil.copy2(path, libs / Path(path).name)
        loader = next(libs.glob("ld-linux*"))
        (dest / name).write_text(
            '#!/bin/sh\nHERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\n'
            f'exec "$HERE/lib/{loader.name}" --library-path "$HERE/lib" "$HERE/qemu" "$@"\n')
        (dest / name).chmod(0o755)
        shutil.copytree("/usr/share/qemu", dest / "share/qemu", symlinks=False)
        if Path("/usr/share/seabios").exists():
            shutil.copytree("/usr/share/seabios", dest / "share/qemu", dirs_exist_ok=True)
        shutil.copytree("/usr/share/doc/qemu-system-common", dest / "licenses", symlinks=False)
        for path in re.findall(r"(/[^\s()]+)", output):
            result = subprocess.run(["dpkg-query", "-S", path], capture_output=True, text=True)
            if result.returncode == 0:
                package = result.stdout.split(":", 1)[0]
                notice = Path("/usr/share/doc") / package / "copyright"
                if notice.exists():
                    shutil.copy2(notice, dest / "licenses" / (package + ".copyright"))
    files = {}
    for p in sorted(dest.rglob("*")):
        if p.is_file():
            with p.open("rb") as f:
                files[p.relative_to(dest).as_posix()] = hashlib.file_digest(f, "sha256").hexdigest()
    (dest / "runtime.json").write_text(json.dumps(files, indent=2))
