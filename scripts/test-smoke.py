#!/usr/bin/env python3
"""Check expected HTTP failures remain catchable, without booting an appliance."""
import contextlib
import io
import unittest.mock
import urllib.error
import os
from pathlib import Path
import tempfile

from smoke import request, remove_test_state

for data, method in ((None, "GET"), ({}, "POST")):
    error = urllib.error.HTTPError("http://127.0.0.1/status", 401, "Unauthorized", {}, io.BytesIO(b"unauthorized"))
    output = io.StringIO()
    with unittest.mock.patch("urllib.request.urlopen", side_effect=error), contextlib.redirect_stdout(output):
        try:
            request(error.url, data=data)
        except urllib.error.HTTPError as caught:
            assert caught is error
        else:
            raise AssertionError("HTTP failure was swallowed")
    assert f"{method} {error.url}: HTTP 401: unauthorized" in output.getvalue()

with tempfile.TemporaryDirectory() as tmp:
    state = Path(tmp) / "synthetic-clinic"
    layers = state / "layers" / "test-release"
    layers.mkdir(parents=True)
    image = layers / "app.img"
    image.write_bytes(b"synthetic read-only layer")
    image.chmod(0o400)
    remove_test_state(state)
    assert not state.exists()

with tempfile.TemporaryDirectory() as tmp:
    state = Path(tmp) / "synthetic-clinic"
    state.mkdir()
    denied = PermissionError("unrelated cleanup failure")
    with unittest.mock.patch("shutil.rmtree", side_effect=lambda path, onexc: onexc(os.rmdir, str(path), denied)):
        try:
            remove_test_state(state)
        except PermissionError as caught:
            assert caught is denied
        else:
            raise AssertionError("Unrelated cleanup failure was swallowed")
