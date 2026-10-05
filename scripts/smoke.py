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


def request(url, token=None, data=None, timeout=240):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, headers=headers, data=json.dumps(data).encode() if data is not None else None)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.read()
    except urllib.error.HTTPError as e:
        # Keep the host's failure reason in CI logs without printing credentials.
        print(f"{req.get_method()} {url}: HTTP {e.code}: {e.read().decode(errors='replace')}", flush=True)
        raise


def wait_control(state, name, process):
    for _ in range(180):
        if process.poll() is not None:
            raise RuntimeError(f"Launcher exited: {process.returncode}")
        try:
            return json.loads((state / name).read_text())
        except (FileNotFoundError, json.JSONDecodeError):
            time.sleep(1)
    raise TimeoutError("No control endpoint")


def wait_healthy(c, state, process, path="/status"):
    start = time.monotonic()
    deadline = start + int(os.environ.get("CARE_SMOKE_BOOT_SECONDS", "600"))
    report = 0
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"Launcher exited: {process.returncode}")
        s = {}
        try:
            s = json.loads(request(c["URL"] + path, c["Token"], timeout=10))
        except (urllib.error.URLError, TimeoutError):
            pass
        if s.get("healthy"):
            return s
        if time.monotonic() >= report or s.get("phase") == "error":
            print(f"Guest wait {int(time.monotonic()-start)}s: phase={s.get('phase')} healthy={s.get('healthy')}", flush=True)
            # Local logs work even when the guest agent never becomes reachable.
            for name in ("console.log",):
                p = state / name
                if p.exists():
                    with p.open("rb") as f:
                        f.seek(max(0, p.stat().st_size - 4096))
                        print(f.read().decode(errors="replace"), flush=True)
            try:
                cc = json.loads((state / "control.json").read_text())
                journal = request(cc["URL"] + "/control/logs", cc["Token"], timeout=15).decode(errors="replace")
                (state / "guest-services.log").write_text(journal, encoding="utf-8")
                print("Guest service journal:\n" + journal, flush=True)
            except (OSError, ValueError) as e:
                print(f"Guest journal unavailable: {e}", flush=True)
            report = time.monotonic() + 60
        if s.get("phase") == "error":
            raise RuntimeError(request(c["URL"] + "/logs", c["Token"], timeout=10).decode())
        time.sleep(2)
    raise TimeoutError("Guest did not become healthy within the elapsed-time boot limit")


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
        s = wait_healthy(c, state, process)
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
        s = wait_healthy(c, state, process)
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
                s = wait_healthy(rc, restored, process, "/control/status")
                assert s["healthy"] and s["configured"]
                request("http://127.0.0.1:8484/api/v1/auth/login/", data=admin)
                print("PASS: encrypted backup -> restore -> same-admin login", flush=True)
            finally:
                request(rc["URL"] + "/control/stop", rc["Token"], {})
                process.wait(timeout=180)
        print("PASS: clean shutdown", flush=True)
        log.close()  # Windows cannot remove an open log file.
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
