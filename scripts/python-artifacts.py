"""Resolve Pipfile.lock to hash-checked source URLs for offline Nix builds.

Run with the pinned CARE Pipfile.lock as argv[1]. Uses sdists to support both
architectures. Every selected digest must occur in the upstream lock.
"""
import json
import sys
import urllib.request
from pathlib import Path

lock = json.loads(Path(sys.argv[1]).read_text())
artifacts = []
arch = sys.argv[2]
for name, spec in lock["default"].items():
    version = spec["version"].removeprefix("==")
    if name == "python-magic":
        url = "https://github.com/ohcnetwork/python-magic-bin/releases/expanded_assets/v0.1"
        import re
        html = urllib.request.urlopen(url).read().decode()
        paths = re.findall(r'href="([^"]+\.whl)"', html)
        path = next(p for p in paths if f"{version}" in p and f"manylinux2014_{arch}" in p)
        url = "https://github.com" + path
        import hashlib
        digest = hashlib.sha256(urllib.request.urlopen(url).read()).hexdigest()
        filename = path.rsplit("/", 1)[-1]
    else:
        info = json.load(urllib.request.urlopen(f"https://pypi.org/pypi/{name}/{version}/json"))
        # Universal wheels avoid unnecessary compilation. Native wheels are resolved
        # separately per architecture below; sdists are a fallback.
        choices = info["urls"]
        selected = next((a for a in choices if a["filename"].endswith("py3-none-any.whl")), None)
        if selected is None:
            selected = next((a for a in choices if a["filename"].endswith("py2.py3-none-any.whl")), None)
        if selected is None:
            from packaging.tags import cpython_tags, compatible_tags
            from packaging.utils import parse_wheel_filename
            platforms = [f"manylinux_{v}_{arch}" for v in ["2_28", "2_27", "2_26", "2_24", "2_17"]] + [f"manylinux2014_{arch}"]
            tags = set(cpython_tags((3, 13), platforms=platforms)) | set(compatible_tags((3, 13), platforms=platforms))
            selected = next((a for a in choices if a["filename"].endswith(".whl") and parse_wheel_filename(a["filename"])[3] & tags), None)
        if selected is None:
            selected = next(a for a in choices if a["packagetype"] == "sdist")
        url, filename, digest = selected["url"], selected["filename"], selected["digests"]["sha256"]
    if "sha256:" + digest not in spec["hashes"]:
        raise ValueError(f"{name}: selected artifact absent from lock")
    artifacts.append(dict(requirement=name + "==" + version, url=url, filename=filename, sha256=digest))
Path(sys.argv[3]).write_text(json.dumps(artifacts, indent=2) + "\n")
