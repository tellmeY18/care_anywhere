#!/usr/bin/env python3
"""Check expected HTTP failures remain catchable, without booting an appliance."""
import contextlib
import io
import unittest.mock
import urllib.error

from smoke import request

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
