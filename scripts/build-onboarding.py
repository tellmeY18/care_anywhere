#!/usr/bin/env python3
"""Build CARE Clinic's pinned onboarding plugin unchanged, for offline hosting."""
import io
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import os

COMMIT = "d78c2f177d281f8b27370e8c9f04571a70fcba08"
root = Path(__file__).resolve().parent.parent
output = root / "dist/onboarding"
if output.exists():
    raise SystemExit(f"Output already exists: {output}")
with tempfile.TemporaryDirectory() as directory:
    source = Path(directory)
    url = f"https://api.github.com/repos/ohcnetwork/care_onboarding_fe/tarball/{COMMIT}"
    with urllib.request.urlopen(url) as response:
        archive = response.read()
    with tarfile.open(fileobj=io.BytesIO(archive)) as tf:
        tf.extractall(source, filter="data")
    checkout = next(source.iterdir())
    subprocess.run(["npm", "ci", "--ignore-scripts"], cwd=checkout, check=True)
    subprocess.run(["npm", "test"], cwd=checkout, check=True)
    subprocess.run(["npm", "run", "build"], cwd=checkout, check=True,
                   env=dict(os.environ, ONBOARDING_BASE_PATH="/onboarding/"))
    shutil.copytree(checkout / "dist", output)
    shutil.copy2(checkout / "LICENSE", output / "LICENSE")
    (output / "SOURCE.txt").write_text(f"https://github.com/ohcnetwork/care_onboarding_fe/tree/{COMMIT}\n")
