# CARE Anywhere — 0.1.0-alpha.1

An offline, single-computer CARE EMR appliance. No Docker, Nix, Python, Node
or Git required to run it — those tools are only needed to build it. Everything
(backend, database, object storage, and the real CARE frontend) runs inside a
small Linux microVM that boots directly from the native launcher.

This is an **early alpha**: use test records only. See "Known limitations" below
before relying on it for anything real.

## Install on macOS (Apple Silicon only)

1. Download `CARE-Anywhere-0.1.0-alpha.1-macos-arm64.dmg` below.
2. Open the DMG and drag **CARE Anywhere** into **Applications**, then eject the
   disk image.
3. Open **CARE Anywhere** from Applications. **This build is ad-hoc signed, not
   notarized** — macOS will refuse to open it the first time. Go to
   **System Settings → Privacy & Security**, scroll to the blocked-app notice,
   and click **Open Anyway**, then open the app again. Do not disable Gatekeeper
   system-wide to work around this.
4. A browser window opens automatically to the control panel. First launch
   copies and verifies an 8 GiB data disk, so the VM can take a minute or two
   to go healthy — the panel shows live status while this happens.
5. Create the administrator account when prompted, then open CARE and sign in
   with it.
6. Use **Clinic & staff setup** in the panel to create your facility,
   departments and additional staff accounts through CARE's own onboarding flow.

Requires macOS 13+ on Apple Silicon, 8 GB RAM, and about 22 GB free disk space.
Closing the browser tab does **not** stop CARE — use **Stop** or
**Quit CARE Anywhere** in the panel before shutting down your computer.
Intel Macs are not built yet.

## Install on Linux (x86_64 and arm64)

1. Download the `.tar.gz` matching your CPU architecture
   (`CARE-Anywhere-0.1.0-alpha.1-linux-amd64.tar.gz` for most PCs,
   `-arm64` for ARM64 machines) below.
2. Extract it: `tar -xzf CARE-Anywhere-0.1.0-alpha.1-linux-*.tar.gz`
3. As your normal desktop user (**not root**), run the installer once:
   `cd care-anywhere && ./install.sh`
4. Launch **CARE Anywhere** from your applications menu. A browser window
   opens automatically.
5. Same as macOS from here: wait for the clinic to go healthy, create the
   administrator, sign in to CARE, then use **Clinic & staff setup**.

Requires hardware virtualization enabled in firmware, read/write access to
`/dev/kvm` (ask your administrator if access is denied), `xdg-open`, a
browser, 8 GB RAM and about 22 GB free disk space. The Firecracker runtime
that boots the VM is bundled and checksum-verified — nothing else is
downloaded at install or first run. Never run the app with `sudo`.

## Verifying your download

Each archive has a matching `.sha256` file:

```sh
shasum -a 256 -c CARE-Anywhere-0.1.0-alpha.1-<file>.sha256
```

## What's inside

- Native Go launcher; no host Docker/Nix/Python/Node/Git at runtime.
- One Linux microVM per clinic: Django/Gunicorn, Celery worker + beat,
  PostgreSQL 17, Redis, MinIO (S3-compatible storage), Caddy, and the real
  prebuilt CARE frontend.
- The guest has **no network interface** — all host↔guest traffic (control API,
  browser proxy) goes over virtio sockets. The appliance is offline by
  construction; email, SMS and other internet-dependent CARE features will not
  work in this alpha.
- The desktop control panel and setup wizard reuse CARE Clinic's actual React
  UI (MIT, `ohcnetwork/care_clinic`), adapted to this VM-backed launcher instead
  of Clinic's Docker Compose engine.
- Facility/department/staff onboarding reuses CARE's own unmodified
  `care_onboarding_fe` plugin (MIT), bundled for fully offline use.
- Encrypted whole-disk backup and restore via the command line (see README).

## Known limitations in this alpha

- **Single computer only.** No LAN access for other devices, no mDNS discovery,
  no TLS/client onboarding — this is unlike CARE Clinic's multi-device model.
- **No scheduled backups.** Backups are manual only (Stop CARE, then "Back up
  now" in the panel, or the `backup`/`restore` CLI commands).
- **No password recovery codes, no in-app update channel.** Losing the admin
  password currently has no self-service recovery path.
- **Windows is not supported yet.**
- **Intel Macs are not built yet** — Apple Silicon only.
- **Linux ARM64 boot is unverified** in this release (builds and passes
  checksum/signature checks; the real KVM boot/setup/restart/backup/restore
  test currently only runs on x86_64 CI runners). Linux x86_64 has a full
  real-boot test passing on every build.
- Restore requires the same CPU architecture as the original backup.
- No telemetry, analytics or crash reporting of any kind.

Full architecture notes, build instructions and the CLI reference are in
[README.md](https://github.com/tellmeY18/care_anywhere/blob/release/alpha/README.md).
