#!/usr/bin/env python3
"""Build CARE Clinic's pinned onboarding plugin, with one upstream bug patched,
for offline hosting. See PATCHES below for exactly what and why."""
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

# Each patch: (relative file, exact text to find, exact text to replace it with).
# Keep this list small, exact-match (fails loudly if upstream changes the line
# rather than silently no-op'ing), and each entry must explain why.
PATCHES = [
    (
        "src/care/content.ts",
        'const path = `/questionnaire/${encodeURIComponent(questionnaire.id)}`;',
        'const path = `/questionnaire/${encodeURIComponent(questionnaire.slug)}`;',
        # CARE's QuestionnaireViewSet uses `slug` as its detail-route lookup_field,
        # not `id`. Building this URL with the UUID makes every single
        # get_organizations/set_organizations call 404 with "No Questionnaire
        # matches the given query" — 100% reproducible, not a race or a stale
        # cache. Verified directly against a running instance: the questionnaire
        # exists and its sharing is already correct at creation time (sharing
        # happens via the `organizations` field passed to the create call), so
        # this broken follow-up "verify/repair" step was failing on fully correct
        # data. Reported upstream; remove this patch once a release of
        # care_onboarding_fe past this commit lands and is repinned.
    ),
    (
        "src/care/content.test.ts",
        'assert.match(String(url), /questionnaire-id\\/set_organizations\\/$/);',
        'assert.match(String(url), /standard-form\\/set_organizations\\/$/);',
        # care_onboarding_fe's own unit test hard-coded this same bug into its
        # mock assertion, which is exactly why it never caught the mismatch
        # against a real backend. Keep this test change paired with the fix
        # above so `npm test` still passes and still means something.
    ),
]

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
    for relative_path, find, replace, *_ in PATCHES:
        path = checkout / relative_path
        text = path.read_text()
        if find not in text:
            raise SystemExit(
                f"Patch target not found in {relative_path} (upstream changed?): {find!r}"
            )
        path.write_text(text.replace(find, replace, 1))
    subprocess.run(["npm", "ci", "--ignore-scripts"], cwd=checkout, check=True)
    subprocess.run(["npm", "test"], cwd=checkout, check=True)
    subprocess.run(["npm", "run", "build"], cwd=checkout, check=True,
                   env=dict(os.environ, ONBOARDING_BASE_PATH="/onboarding/"))
    shutil.copytree(checkout / "dist", output)
    shutil.copy2(checkout / "LICENSE", output / "LICENSE")
    (output / "SOURCE.txt").write_text(
        f"https://github.com/ohcnetwork/care_onboarding_fe/tree/{COMMIT}\n"
        f"Patched before build — see PATCHES in this script for exact diffs and why.\n"
    )
