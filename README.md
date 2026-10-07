# CARE Anywhere — desktop alpha

A native Go launcher and a prebuilt Linux clinic appliance. On Apple Silicon,
the launcher uses Apple's Virtualization.framework directly: **no host Nix,
Docker, QEMU, Python, Git, or Node is invoked at runtime**. Developers use Nix
and Go to build it. This is an experimental desktop alpha, not a production-ready
replacement for CARE Clinic.

## Install the alpha

Download the platform artifact from [Releases](https://github.com/tellmeY18/care_anywhere/releases).

**macOS (Apple Silicon, macOS 13+):** open the DMG, drag **CARE Anywhere** to
**Applications**, eject the image, then open the app. This alpha is ad-hoc signed,
not notarized: after the first blocked launch, use **System Settings → Privacy &
Security → Open Anyway**. Do not disable Gatekeeper. Intel Mac builds are not yet available.

**Linux (x86_64 or ARM64):** download the matching `.AppImage`, mark it executable
(`chmod +x CARE-Anywhere-*.AppImage`) and open it as your normal desktop user.
QEMU and its libraries are bundled. Enable hardware virtualization and give your
user read/write access to `/dev/kvm`; `xdg-open` and a browser are required.
If FUSE 2 is unavailable, launch with `--appimage-extract-and-run` (requires
additional temporary disk space). Never run the desktop app with `sudo`.

**Windows (x86_64, Windows 10/11):** run the `.exe` installer, then open
**CARE Anywhere** from Start. The installer includes QEMU, the appliance and
the complete frontend; no runtime downloads are required. Enable **Windows
Hypervisor Platform** in Windows Features and CPU virtualization in firmware,
then restart. This alpha is unsigned. WHPX hardware boot needs separate acceptance;
hosted Windows CI exercises the same package using explicit slow QEMU TCG emulation.

Allow **8 GB RAM and 12 GB free disk** for the app and clinic, plus space for
backups. The control panel opens immediately while the appliance is verified and
the initial disk is prepared. Create an administrator, open CARE, sign in, then
create your facility in CARE. Reopening the app returns to its existing panel.
The desktop frontend reuses CARE Clinic's React setup and control-panel components,
with a VM-backed bridge. CARE Clinic's unchanged onboarding plugin is bundled
locally: open **Clinic setup** after signing in to create the facility, departments,
staff and Facility Admin memberships. Automatic post-login redirect is not enabled
in the currently pinned CARE frontend; use the explicit Clinic setup button.

Closing a browser tab keeps CARE running. Use **Stop CARE** or **Quit CARE Anywhere**
before shutting down the computer. The panel can start CARE again, create a cold
encrypted backup while stopped, open the backup folder, and show diagnostics.
Backups are saved in `~/CARE Anywhere Backups`; keep both the `.age` archive and
the `.key` recovery file, storing the key separately. Restore still uses the CLI.

Shutdown is scheduled briefly after its control acknowledgement so the guest
agent can return a response before systemd stops it. The launcher retains its
data lock until the VM has exited.

Clinic data lives outside the app in `~/Library/Application Support/care-anywhere`
on macOS, `${XDG_CONFIG_HOME:-~/.config}/care-anywhere` on Linux, and
`%APPDATA%\care-anywhere` on Windows. Removing the
app keeps that data. Use test records: this alpha has no automatic backups,
password recovery or LAN access. Layered alpha updates require an explicitly
trusted local bundle; automatic signed-channel activation awaits beta.

## What runs

One guest runs native Python 3.13/Gunicorn, Celery worker and beat, PostgreSQL 17,
Redis, MinIO, Caddy, and the prebuilt CARE frontend. Python packages are fetched
with hashes checked against CARE's lock and installed at **build time**. This is
**local-first, not offline-only**: all clinic data, compute and storage stay on
this computer. macOS uses native Virtualization.framework and vsock; Linux and
Windows share bundled QEMU with KVM/WHPX acceleration. QEMU control/browser
traffic uses a loopback-only forward authenticated by fresh per-boot mutual TLS
credentials passed privately through fw_cfg. All platforms have outbound-only
NAT by default for features such as SNOMED lookups. `CARE_NO_NETWORK=1` disables
outbound connectivity while preserving local control. Nothing binds to the LAN.
QEMU disk serials are set on explicit virtio block devices, preserving the guest's
stable disk IDs for the read-only layers and writable data disk.
Email/SMS and other third-party integrations still are not configured.

The base, Python runtime and application images are separate read-only EROFS
disks, overlaid in the initrd. Database, objects, generated settings,
static files and signing keys live on a separate 8 GiB ext4 data disk. The Nix
build runs migrations into a fresh temporary PostgreSQL database and exports an
empty logical seed. First boot restores that seed transactionally only when the
public schema has no tables, then checks migrations and loads reference data.
Existing databases use normal migrations; interruption during seed restore rolls
back instead of leaving a partially imported schema. Setup creates the initial administrator. No patient data
or shared installation secrets are baked into the release.

Gunicorn uses a bounded 300-second worker timeout to accommodate slow emulated
startup and memory-backed heartbeat files under `/run/care-api`. This timeout
also applies to silent request workers; it is not a separate startup-only grace period.

## Run the local preview

The tested build is in `dist/` (ignored by Git). From this repository:

```sh
./dist/care-anywhere serve --bundle ./dist --state ./.state
./dist/care-anywhere status --state ./.state
./dist/care-anywhere logs --state ./.state
./dist/care-anywhere stop --state ./.state
```

`serve` stays running and opens a browser control page. Closing the page does not
stop the clinic; terminating the launcher does. The private access link is also
in `<state>/open.html`. CARE binds to `127.0.0.1:8484`; management uses a separate
random loopback port. The desktop panel has its own private loopback origin.
Use that exact address, not `localhost`: host validation rejects other names.
The management API requires a random per-run bearer token. Do not expose this
preview listener to the LAN. Port overrides currently do not rewrite browser S3
URLs, so keep the default port for attachment workflows.

The first startup copies a 64 MiB formatted seed and sparsely extends its private
copy to 8 GiB; ext4 grows at boot. Existing data disks are never formatted. The desktop panel
appears before this work; the lower-level `serve` control page appears afterward.
Allow at least 9 GiB free beyond the bundle for eventual data growth,
plus room for backups and restore. Avoid cloud-synced data directories.

### Backup and restore

```sh
./dist/care-anywhere stop --state ./.state
./dist/care-anywhere backup --state ./.state --file /external/clinic.age
./dist/care-anywhere restore --state /new/clinic \
  --file /external/clinic.age --key /separate/clinic.age.key
./dist/care-anywhere serve --bundle ./dist --state /new/clinic
```

Backups require the clinic to be stopped and acquire the same OS file lock as the
launcher. This preview encrypts a compressed **whole data disk** and release
metadata with age. It writes a new recovery key beside each backup; move that key
to separate protected storage. It never overwrites an existing backup/key.

Restore validates/decrypts into staging and only accepts an empty destination.
Original data is never replaced. Corruption fails before disk publication.
An interruption between metadata/disk publication may require retrying into
another empty directory; keep the original backup. This is not yet CARE Clinic's
complete interrupted-restore recovery protocol. Restore needs the same CPU
architecture and a compatible release. Cross-architecture migration requires a
future logical PostgreSQL/object export. CARE Clinic `.dump.enc` backups are not
accepted by this preview.

## CARE Clinic reuse

See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Actual reused/adapted code:

- Atomic file replacement, directory sync and Windows private-file ACLs.
- Disk-capacity checks and nearest-existing-path lookup.
- Atomic-write regression coverage.
- Bucket initialization/public facility-download policy, ported to native Python.

CARE Clinic's exclusive-operation, staged-restore and data-preservation contracts
inform the lifecycle. Its Wails wizard, client trust/mDNS, scheduled backups,
recovery-code UX and update journal have **not** been ported. Prefer adapting those
existing components as those features land instead of creating competing versions.

## Build

```sh
go test -race ./...
go build -o dist/care-anywhere .
# macOS development signing (distribution also needs notarization):
codesign --force --sign - --entitlements entitlements.plist dist/care-anywhere

# On ARM64 Linux, or with a configured builder:
nix build .#packages.aarch64-linux.bundle
```

With a user-session SSH agent, a remote store can build without daemon SSH setup:

```sh
nix build --store ssh-ng://root@kenobi --eval-store auto --no-link \
  --print-out-paths .#packages.aarch64-linux.bundle
```

Export only the bundle's `manifest.json`, `kernel`, `initrd`, `system.img`,
`runtime.img`, `app.img` and `data.img` using tar/rsync. Linux/Windows packaging separately stages QEMU and
its dependencies with a checked runtime manifest.
Do not distribute the generated Nix runner or copy
its entire closure. All Nix paths used at runtime belong **inside** the guest.
Use checksum verification after interrupted transfers (`rsync -c` if needed).
The manifest hashes detect corruption, but do not authenticate a publisher yet:
only run locally built or otherwise trusted bundles.

`scripts/python-artifacts.py` resolves pinned Python artifacts for ARM64/x86-64
and checks their hashes against CARE's lock. `scripts/frontend-lock.py` fills in
missing upstream npm artifact URLs/integrity without changing selected versions
(it normalizes a leading `v`). Generated manifests are checked in. The python-magic
wheel's erroneous CPython-3.12 metadata is normalized to its published universal
tag. Native wheels retain their bundled-library RPATHs.

### Package a release

The **Build CARE Anywhere alpha** GitHub Actions workflow builds both Linux
architectures, the reused desktop frontend and local onboarding plugin, the
Apple Silicon DMG, Linux AppImages, and Windows x86_64 offline `.exe` installer.
Pushes to `main` or manual dispatch start a complete build. Pull requests run
the fast Go/UI/release-validator checks. Download `macos-arm64`,
`linux-amd64`, `linux-arm64`, or `windows-amd64` from the run's artifacts. No local Nix
builder or artifact upload from a developer laptop is required. Nix and npm caches
speed subsequent runs; large appliance payloads are compressed once between jobs.

The x86_64 Linux AppImage must pass a real QEMU/KVM boot/setup/restart/backup/restore
test. Windows runs native race tests, installs the actual `.exe`, then exercises
boot/setup/restart/backup/restore under explicit TCG on the hosted runner.
There is no automatic fallback to TCG on user machines. Windows WHPX and ARM64
Linux KVM hardware acceptance remain required outside hosted CI.
GitHub's ARM macOS runners do not support nested virtualization, so macOS CI
checks packaging/signatures; the downloaded build still needs a local boot check.
Build and publication are separate workflows. `VERSION` holds the base version;
each build derives `<base>-alpha.<build run number>` for all four package filenames
and the release tag `v<base>-alpha.<build run number>`. Rerunning failed jobs keeps
the same version and reuses that run's appliance/interface artifacts. The package
version does not change the guest manifest's data-compatibility identifier.

After all build jobs succeed, dispatch **Publish CARE Anywhere alpha** on `main`
with the build run ID. It verifies the source workflow, branch, commit's version,
acceptance jobs, exact four packages, both sets of standalone OTA images/manifests
and all checksum sidecars. OTA hashes must match their architecture's manifest.
All packages must come from one successful run; cross-run diagnostic artifacts
cannot be promoted. Release notes come from the tested commit, with version and
build provenance filled in automatically.

```sh
# Build on GitHub Actions (or push to main):
gh workflow run alpha.yml --ref main
# Retry only failed jobs of that build, without rebuilding successful platforms:
gh run rerun <build-run-id> --failed
# Prepare a complete draft, without rebuilding:
gh workflow run release.yml --ref main -f build_run_id=<build-run-id>
# Publish, or resume a partially uploaded draft:
gh workflow run release.yml --ref main -f build_run_id=<build-run-id> -f publish=true
```

The publisher creates the tag at the tested SHA and uploads into a draft, then
checks GitHub's uploaded-asset digests before publication. Retries skip identical
assets and refuse conflicting assets/tags or modifications to published releases.
Artifacts expire after 14 days: publish within that window or start a new build.
The workflow uses `contents: write` and `actions: read`. If repository policy blocks
tag creation for commits changing workflows, an authorized maintainer can create
the exact lightweight tag at the tested SHA and retry; the workflow never retargets
an existing tag or requests broader credentials. No personal token is stored in CI.
Builds and compilation-based tests for release work run in CI, not on developer laptops.

`npm --prefix frontend run test:ui` checks the reused setup/control screens against
the appliance HTTP contract (install Playwright Chromium first). These focused
tests replace the upstream Wails-only simulated-host suite, which does not test
this adapter. They do not replace the real appliance smoke test.

```sh
# Build-time tools: Python 3.12+, Go, Node; macOS also needs Xcode command-line tools.
npm --prefix frontend ci --ignore-scripts
npm --prefix frontend run build
python3 scripts/build-onboarding.py
python3 scripts/package.py macos --bundle dist
python3 scripts/package.py linux --bundle /path/to/linux-bundle
# On Windows CI (Go, Python, 7-Zip and Inno Setup):
python scripts/package.py windows --bundle dist/bundle
```

Outputs are in `dist/releases/`. Packaging verifies every manifest entry, includes
only an explicit allowlist of appliance files (never a runtime state or backup),
and writes SHA-256 files. Staging directories must be new. macOS uses APFS clones
to avoid duplicating large input disks. Set `MACOS_SIGN_IDENTITY` and
`MACOS_NOTARY_PROFILE` to enable Developer ID signing and notarization when available.

`scripts/smoke.py` tests a fresh isolated clinic, UI authentication, admin setup,
CARE login, frontend access, origin separation, stop/start and optionally a full
backup/restore/login round trip. It refuses an existing test state directory:

```sh
python3 scripts/smoke.py --binary dist/care-anywhere-alpha --bundle dist \
  --state dist/smoke-new --restore --cleanup
```

## Verified in this session

- ARM64 appliance built on `root@kenobi`, exported and booted on Apple Silicon.
- Standalone macOS launcher, no host runtime commands for virtualization.
- API healthy and frontend HTTP 200, with guest networking absent.
- Initial administrator created through authenticated control API; login HTTP 200.
- Cold encrypted backup restored into a separate directory; restored guest booted
  healthy and the same administrator logged in successfully.
- Go race tests: active-clinic backup refusal, backup round trip, overwrite refusal,
  corruption rejection, bundle traversal rejection, and reused private atomic writes.
- Desktop alpha: immediate panel, fresh admin setup and login, separate management
  origin, stop/start preserving the admin, clean quit, encrypted backup/restore
  and same-admin login verified on Apple Silicon.
- Linux x86_64 on chopper: unprivileged Firecracker boot, administrator setup,
  CARE login, frontend, stop/restart, encrypted backup/restore/same-admin login
  and clean shutdown verified. Firecracker uses a clean guest reboot to exit;
  the minimal virtual machine does not implement ACPI power-off.
- Linux ARM64 launcher cross-compiles; ARM64 Linux boot remains unverified.
- Locally hosted CARE Onboarding: standard role/geography import and facility
  creation verified through the actual browser and authenticated CARE API.
- QEMU Linux/Windows support replaces the earlier Firecracker/stub implementation.
  Check the new release's linked Actions run for its acceptance results; historical
  Firecracker and Apple Silicon checks above do not establish QEMU/WHPX coverage.

## Before an end-user release

1. Port CARE Clinic's installer/setup UX and recovery-key verification. Current
   "Create clinic" creates an administrator; it does not configure a facility.
2. Add sustained multi-service
   health, bounded logs, resumable downloads, signed manifests and versioned updates.
3. Add scheduled/application-consistent logical backups and interruption recovery,
   with disk-space budgets and restoration tests on a second machine.
4. Port LAN discovery/TLS/client onboarding and background service integration.
5. Validate Windows WHPX on physical hardware and Linux ARM64 KVM; add sustained
   platform-specific shutdown/crash testing and Windows signing.
6. Test clinical browser workflows (including attachments and PDFs), not just HTTP
   responses. Ship third-party notices/source offers for every appliance component.
7. **Re-evaluate the object store once a release branch has it.** This alpha
   uses `pkgs.silo` (github:pgsty/silo, see nix/guest.nix), an actively
   maintained MinIO fork with the CVE fixes upstream minio/minio lacks. It is
   not yet backported to a nixos-* release branch, so `flake.nix` pins
   `nixpkgs` to a nixos-unstable commit instead — trading release-branch
   stability for the current security fixes. Move back to a release branch
   once it ships silo, or re-pin nixpkgs periodically in the meantime.

## Layered updates (0.2 alpha)

See [docs/UPDATING.md](docs/UPDATING.md) for the step-by-step update guide and [OTA.md](OTA.md) for the contracts, CLI and first-beta gates.
The app's Caddy fragment is parsed with the complete server configuration during
the Nix build. Guest diagnostics include Caddy and object-store startup failures.
Python wheels retain one build-time bytecode variant, and initialization runs
migrations, reference-data sync and static collection in one Django process to
avoid repeated compilation/import work on slow emulated hosts.
`update-fetch` downloads HTTPS images without activating them. `update-stage
--trust-local` stages a verified local bundle while stopped; next start creates a
cold snapshot and switches to state-owned layers. `update-status` reports pending
or interrupted activation; `update-recover --destination` copies pre-update data
into a new directory without overwriting post-update records. Native APFS clones
are used on macOS; other hosts need space for a full snapshot copy.

The new format checks base ABI, host protocol, runtime/base hashes and data schema.
**0.1 preview data cannot be booted with the 0.2 layered base.** Preserve its old
bundle and encrypted backup. A separate tested migration is required. Signing is
deferred to beta, so downloaded hashes alone do not authenticate a publisher.
