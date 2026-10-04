# CARE Anywhere — appliance preview

A native Go launcher and a prebuilt Linux clinic appliance. On Apple Silicon,
the launcher uses Apple's Virtualization.framework directly: **no host Nix,
Docker, QEMU, Python, Git, or Node is invoked at runtime**. Developers use Nix
and Go to build it. This repository is an experimental first milestone, not a
signed end-user installer or a production-ready replacement for CARE Clinic.

## What runs

One guest runs native Python 3.13/Gunicorn, Celery worker and beat, PostgreSQL 17,
Redis, MinIO, Caddy, and the prebuilt CARE frontend. Python packages are fetched
with hashes checked against CARE's lock and installed at **build time**. The guest
has no NIC: control and browser traffic use virtio sockets. External email/SMS,
hosted plugins, and other internet integrations consequently do not work yet.

The OS/application image is read-only. Database, objects, generated settings,
static files and signing keys live on a separate 8 GiB ext4 data disk. First boot
migrates the database; setup creates the initial administrator. No patient data
or shared installation secrets are baked into the release.

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
in `<state>/open.html`. Clinic and management bind only to `127.0.0.1:8484`.
Use that exact address, not `localhost`: host validation rejects other names.
The management API requires a random per-run bearer token. Do not expose this
preview listener to the LAN. Port overrides currently do not rewrite browser S3
URLs, so keep the default port for attachment workflows.

The first startup copies an 8 GiB disk and verifies the bundle, so it can take
time before the control page appears. Allow at least 12 GiB free beyond the bundle,
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

Export only the bundle's `manifest.json`, `kernel`, `initrd`, `system.img`, and
`data.img`, using tar/rsync. Do not distribute the generated Nix runner or copy
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

## Verified in this session

- ARM64 appliance built on `root@kenobi`, exported and booted on Apple Silicon.
- Standalone macOS launcher, no host runtime commands for virtualization.
- API healthy and frontend HTTP 200, with guest networking absent.
- Initial administrator created through authenticated control API; login HTTP 200.
- Cold encrypted backup restored into a separate directory; restored guest booted
  healthy and the same administrator logged in successfully.
- Go race tests: active-clinic backup refusal, backup round trip, overwrite refusal,
  corruption rejection, bundle traversal rejection, and reused private atomic writes.
- Linux ARM64 launcher cross-compiles. Linux Firecracker boot is **not tested**.
- Windows returns an explicit unsupported error. No WHPX implementation is shipped.

## Before an end-user release

1. Port CARE Clinic's installer/setup UX and recovery-key verification. Current
   "Create clinic" creates an administrator; it does not configure a facility.
2. Separate management and clinical browser origins; add sustained multi-service
   health, bounded logs, resumable downloads, signed manifests and versioned updates.
3. Add scheduled/application-consistent logical backups and interruption recovery,
   with disk-space budgets and restoration tests on a second machine.
4. Port LAN discovery/TLS/client onboarding and background service integration.
5. Test Firecracker/jailer on Linux and implement/test Windows WHPX. The Linux
   preview expects an external `bundle/runtime/firecracker`; it isn't bundled yet.
6. Test clinical browser workflows (including attachments and PDFs), not just HTTP
   responses. Ship third-party notices/source offers for every appliance component.

Current `0.1.0-preview` manifests do not encode a migration compatibility policy.
Do not swap arbitrary preview images against important data: take a backup and
restore into a separate directory for testing. Signed updates and rollback are
intentionally not exposed as working commands.
