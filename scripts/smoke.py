#!/usr/bin/env python3
"""Real appliance smoke test. Uses an isolated state directory; never existing clinic data."""
import argparse
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time
import urllib.error
import urllib.request


def request(url, token=None, data=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, headers=headers, data=json.dumps(data).encode() if data is not None else None)
    with urllib.request.urlopen(req, timeout=240) as r:
        return r.read()


def wait_control(state, name, process):
    for _ in range(180):
        if process.poll() is not None:
            raise RuntimeError(f"Launcher exited: {process.returncode}")
        try:
            return json.loads((state / name).read_text())
        except (FileNotFoundError, json.JSONDecodeError):
            time.sleep(1)
    raise TimeoutError("No control endpoint")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--binary", type=Path, required=True)
    p.add_argument("--bundle", type=Path, required=True)
    p.add_argument("--state", type=Path, required=True)
    p.add_argument("--restore", action="store_true", help="Also verify backup/restore into a second empty state")
    p.add_argument("--cleanup", action="store_true", help="Remove only the isolated test states created by this invocation after success")
    a = p.parse_args()
    binary, bundle, state = a.binary.resolve(), a.bundle.resolve(), a.state.resolve()
    state.mkdir()  # Refuse any existing state.
    env = dict(os.environ, CARE_NO_BROWSER="1")
    log = (state / "smoke.log").open("w")
    process = subprocess.Popen([str(binary), "desktop", "--state", str(state), "--bundle", str(bundle)], stdout=log, stderr=log, env=env)
    c = None
    try:
        c = wait_control(state, "desktop.json", process)
        panel = request(c["URL"])
        assert b'<div id="root"' in panel
        try:
            request(c["URL"] + "/status")
            raise AssertionError("Unauthenticated status accepted")
        except urllib.error.HTTPError as e:
            assert e.code == 401
        print("PASS: immediate UI and management authentication", flush=True)
        for _ in range(300):
            s = json.loads(request(c["URL"] + "/status", c["Token"]))
            if s["phase"] == "error":
                raise RuntimeError(request(c["URL"] + "/logs", c["Token"]).decode())
            if s["healthy"]:
                break
            time.sleep(2)
        else:
            raise TimeoutError(request(c["URL"] + "/logs", c["Token"]).decode())
        assert not s["configured"]
        admin = {"username": "alphatest", "password": secrets.token_urlsafe(24)}
        request(c["URL"] + "/setup", c["Token"], admin)
        assert json.loads(request(c["URL"] + "/status", c["Token"]))["configured"]
        assert request("http://127.0.0.1:8484/api/v1/auth/login/", data=admin)
        assert b"html" in request("http://127.0.0.1:8484/").lower()
        try:
            request("http://127.0.0.1:8484/control/status", c["Token"])
            raise AssertionError("Management exposed on CARE origin")
        except urllib.error.HTTPError as e:
            assert e.code == 404
        print("PASS: fresh boot, setup, CARE login, frontend, separate origins", flush=True)
        request(c["URL"] + "/stop", c["Token"], {})
        assert json.loads(request(c["URL"] + "/status", c["Token"]))["phase"] == "stopped"
        request(c["URL"] + "/start", c["Token"], {})
        for _ in range(120):
            s = json.loads(request(c["URL"] + "/status", c["Token"]))
            if s["healthy"]:
                break
            time.sleep(2)
        assert s["healthy"] and s["configured"]
        request("http://127.0.0.1:8484/api/v1/auth/login/", data=admin)
        print("PASS: stop/start preserves administrator", flush=True)
        request(c["URL"] + "/quit", c["Token"], {})
        process.wait(timeout=30)
        assert process.returncode == 0
        c = None
        if a.restore:
            archive = state.parent / (state.name + ".age")
            restored = state.parent / (state.name + "-restored")
            subprocess.run([str(binary), "backup", "--state", str(state), "--file", str(archive)], check=True)
            # Synthetic test data only: release disk space before the restore.
            (state / "data.img").unlink()
            subprocess.run([str(binary), "restore", "--state", str(restored), "--file", str(archive), "--key", str(archive)+".key"], check=True)
            process = subprocess.Popen([str(binary), "serve", "--state", str(restored), "--bundle", str(bundle), "--no-open"], stdout=log, stderr=log)
            rc = wait_control(restored, "control.json", process)
            try:
                for _ in range(120):
                    try:
                        s = json.loads(request(rc["URL"] + "/control/status", rc["Token"]))
                        if s["healthy"]:
                            break
                    except urllib.error.URLError:
                        pass
                    time.sleep(2)
                assert s["healthy"] and s["configured"]
                request("http://127.0.0.1:8484/api/v1/auth/login/", data=admin)
                print("PASS: encrypted backup → restore → same-admin login", flush=True)
            finally:
                request(rc["URL"] + "/control/stop", rc["Token"], {})
                process.wait(timeout=180)
        print("PASS: clean shutdown", flush=True)
        if a.cleanup:
            shutil.rmtree(state)
            if a.restore:
                shutil.rmtree(restored)
                archive.unlink()
                Path(str(archive)+".key").unlink()
    finally:
        if c and process.poll() is None:
            try:
                request(c["URL"] + "/quit", c["Token"], {})
                process.wait(timeout=180)
            except Exception:
                print("Test process still active; inspect its logs and stop it gracefully.")
        log.close()


if __name__ == "__main__":
    main()
