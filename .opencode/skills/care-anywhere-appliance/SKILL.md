---
name: care-anywhere-appliance
description: >
  Use when building, boot-testing, or debugging the CARE Anywhere appliance in
  this repo — the Nix-built NixOS microVM guest (kernel/initrd/system.img/
  data.img bundle) and its native Go launcher (vz on macOS, Firecracker on
  Linux). Covers the cross-machine build loop (remote Linux Nix builder +
  macOS launcher), bundle export without copying the whole Nix closure,
  codesigning for Virtualization.framework, boot verification via the control
  API, and backup/restore testing. Activate for any task touching flake.nix,
  nix/*.nix, main.go, vm_*.go, guest_*.go, or storage.go.
---

# CARE Anywhere: appliance build and boot-test loop

This encodes the exact working sequence used to get this repo's appliance
building, booting, and passing backup/restore verification. Follow it instead
of reinventing — several steps here fix real problems hit during development
(see `AGENTS.md` → "Known build pitfalls" for the *why*; this skill is the
*how*, step by step).

## Prerequisites

- A Linux Nix builder reachable over SSH (the appliance is a NixOS/Linux
  derivation; it cannot build directly on macOS). Confirm reachability first:
  `ssh -o BatchMode=yes -o ConnectTimeout=10 <builder-host> uname -m`
- Go toolchain locally, for the launcher.
- On macOS: the binary must be **codesigned with the virtualization
  entitlement** to use `Code-Hex/vz` — an unsigned/unentitled binary fails
  silently or with a cryptic VM-start error.

## 1. Fast checks (always, before anything else)

```sh
go fmt ./...
go vet ./...
go test -race ./...
```

If `storage_test.go` fails, do not loosen the assertion — it encodes the
backup/restore safety invariants (refuse-overwrite, refuse-active-clinic,
reject-corruption-before-publish, reject-path-traversal). Fix the code, not
the test, unless you are deliberately and explicitly changing a safety
contract (and if so, update `AGENTS.md` too).

## 2. Build + sign the launcher (macOS)

```sh
go build -o dist/care-anywhere .
codesign --force --sign - --entitlements entitlements.plist dist/care-anywhere
```

Re-run `codesign` after **every** `go build` — a fresh binary has no signature.
If `codesign` reports "internal error in Code Signing subsystem", it's almost
always a stale/locked file from a prior run; `chmod u+w dist dist/care-anywhere`
then retry.

Cross-compile sanity for other platforms (compiles only — cannot boot-test a
Linux/Windows guest from macOS without a real Linux/Windows host):

```sh
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/care-anywhere-linux .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/care-anywhere.exe .
```

## 3. Build the appliance bundle on the remote Linux builder

```sh
nix build --store ssh-ng://<builder-host> --eval-store auto --no-link \
  --print-out-paths .#packages.aarch64-linux.bundle
```

Swap `aarch64-linux` for `x86_64-linux` to match the builder's architecture —
the launcher refuses to boot a bundle whose `manifest.json` arch doesn't match
`runtime.GOARCH`.

**Before running this against a local working copy**, make sure nothing huge
and untracked is sitting in the repo root — `nix build path:.#...` on macOS
will include untracked files as part of the flake's source input, and a stray
multi-GB `.state/data.img` left over from a previous boot test can blow past
available sandbox space on the *build host* mid-build. Either:
- `git add` only the files that should be inputs before building from `path:.`, or
- push to the repo and build against `github:tellmeY18/care_anywhere` instead
  of a local path while iterating.

If the build fails with a Python `ImportError: lib*.so.*: cannot open shared
object file` — this is the Pillow/zstd RPATH-stripping issue, already fixed by
`dontPatchELF = true;` in `nix/python.nix`. Don't re-debug it; check that
setting is still present.

If it fails with `hash mismatch in fixed-output derivation`, update the
corresponding hash (`npmDepsHash` in `nix/frontend.nix` or `vendorHash` in
`nix/guest.nix`'s `buildGoModule` call) to the "got:" value Nix reports — this
is expected when source or lockfile pins change, not a bug.

## 4. Export only the four files the launcher needs — never the full closure

```sh
ssh <builder-host> \
  'tar -C /nix/store/<hash>-care-anywhere-bundle -czf - manifest.json kernel initrd system.img data.img' \
  | tar -xzf - -C dist
```

Do **not** `nix copy` the bundle store path back to macOS — it pulls in the
entire build closure (every Python/Node/Nix dependency, many GB) and is both
slow and liable to exhaust local disk. The tar-over-ssh approach above pulls
exactly the ~1.5 GB payload the launcher actually reads.

If the transfer gets interrupted (common with `data.img` at 8 GiB), don't
re-tar everything — resync just the broken file with checksum verification:

```sh
rsync -azc --partial <builder-host>:/nix/store/<hash>-care-anywhere-bundle/data.img dist/data.img
```

The launcher will tell you exactly which file failed
(`checksum mismatch: data.img`) if you skip this and boot anyway — that error
means "re-sync this one file," not "something is fundamentally broken."

## 5. Boot and verify

```sh
nohup ./dist/care-anywhere serve --bundle ./dist --state ./.state \
  > dist/launcher.log 2>&1 &
sleep 30   # first boot runs Django migrations; allow up to ~90s on slower hardware
./dist/care-anywhere status --state ./.state
```

Expect `{"configured":false,"healthy":true}` on a fresh clinic. `healthy`
means the guest's `/ping/` responds; `configured` means an administrator has
been created via setup. If `healthy` stays false past 2 minutes:

```sh
./dist/care-anywhere logs --state ./.state
```

This streams `journalctl` for `care-init`/`care-api`/`care-worker`/`care-beat`
from inside the guest over vsock — it is the primary debugging tool. Don't
rely solely on `dist/launcher.log`, which only has host-side errors.

To exercise the full admin-setup path (not just health):

```sh
python3 - <<'PY'
import json, secrets, urllib.request, pathlib
s = pathlib.Path(".state")
c = json.loads((s / "control.json").read_text())
password = secrets.token_urlsafe(24)
req = urllib.request.Request(
    c["URL"] + "/control/setup",
    data=json.dumps({"username": "admin", "password": password}).encode(),
    headers={"Authorization": "Bearer " + c["Token"], "Content-Type": "application/json"},
)
print(urllib.request.urlopen(req, timeout=60).read().decode())
(s / "test-admin.json").write_text(json.dumps({"username": "admin", "password": password}))
(s / "test-admin.json").chmod(0o600)
PY
./dist/care-anywhere status --state ./.state   # now {"configured":true,...}
```

Verify actual CARE login (not just the control API) and the frontend:

```sh
python3 - <<'PY'
import json, pathlib, urllib.request
d = json.loads(pathlib.Path(".state/test-admin.json").read_text())
req = urllib.request.Request(
    "http://127.0.0.1:8484/api/v1/auth/login/",
    data=json.dumps(d).encode(),
    headers={"Content-Type": "application/json"},
)
print("Login:", urllib.request.urlopen(req).status)
print("Frontend:", urllib.request.urlopen("http://127.0.0.1:8484/").status)
PY
```

## 6. Stop cleanly before any backup

```sh
./dist/care-anywhere stop --state ./.state
```

`backup()` takes the same OS lock the launcher holds while running and will
refuse to run against a live clinic — this is intentional, not a bug to route
around.

## 7. Backup/restore round-trip test

Any change touching `storage.go`, `lock_unix.go`, or `nix/guest.nix`'s data
layout should be verified with a full round trip, not just unit tests:

```sh
./dist/care-anywhere backup  --state ./.state --file ./dist/test.age
./dist/care-anywhere restore --state ./dist/restored \
  --file ./dist/test.age --key ./dist/test.age.key

nohup ./dist/care-anywhere serve --bundle ./dist --state ./dist/restored \
  > dist/restore-boot.log 2>&1 &
sleep 40
./dist/care-anywhere status --state ./dist/restored   # must show healthy:true

# Confirm the restored clinic has the SAME administrator (not just that it boots):
python3 - <<'PY'
import json, pathlib, urllib.request
d = json.loads(pathlib.Path(".state/test-admin.json").read_text())
req = urllib.request.Request(
    "http://127.0.0.1:8484/api/v1/auth/login/",
    data=json.dumps(d).encode(),
    headers={"Content-Type": "application/json"},
)
print("Restored admin login:", urllib.request.urlopen(req).status)
PY

./dist/care-anywhere stop --state ./dist/restored
```

A restore that boots but can't log in with the original credentials is a
**failed** test, not a partial pass — it means the data disk round-trip is
silently losing state.

## Checklist before calling any appliance change "done"

- [ ] `go test -race ./...` passes
- [ ] Launcher builds and is codesigned
- [ ] Appliance bundle builds on the remote Linux builder with no hash
      mismatches left unresolved
- [ ] Fresh boot reaches `healthy:true` within ~90s
- [ ] Admin setup succeeds via `/control/setup`
- [ ] CARE login (`/api/v1/auth/login/`) returns 200, not just `/ping/`
- [ ] Frontend root returns 200
- [ ] Clean `stop` succeeds
- [ ] Backup → restore → boot → same-admin-login round trip succeeds
- [ ] `README.md`'s "Verified in this session" section updated if behavior
      changed (don't let it silently rot into a false claim)
