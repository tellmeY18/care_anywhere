# CARE Anywhere — multiplatform alpha

Local-first CARE EMR: clinic data, database and compute stay on your computer.
Complete offline packages include the native launcher, Linux appliance, desktop
control panel and onboarding plugin. No Docker, Nix, Python, Node, Git or separately
installed QEMU required. **Early alpha: use test records.**

## Downloads and installation

| Platform | Download | Start |
| --- | --- | --- |
| macOS 13+ Apple Silicon | `CARE-Anywhere-{{VERSION}}-macos-arm64.dmg` | Drag to Applications, eject, open CARE Anywhere |
| Linux x86_64 | `CARE-Anywhere-{{VERSION}}-linux-amd64.AppImage` | Mark executable, then open |
| Linux ARM64 | `CARE-Anywhere-{{VERSION}}-linux-arm64.AppImage` | Mark executable, then open |
| Windows 10/11 x86_64 | `CARE-Anywhere-{{VERSION}}-windows-amd64.exe` | Run installer, open CARE Anywhere from Start |

Allow **8 GB RAM and 12 GB free disk**, plus temporary installation/extraction,
update snapshots and backup space. First preparation extends a 64 MiB formatted
seed to an 8 GiB logical clinic disk and grows ext4 at boot;
the browser control panel appears while it works. Create the administrator, open
CARE, sign in, then use **Clinic & staff setup** for facility/staff onboarding.
Closing the browser keeps CARE running. Use **Stop** or **Quit CARE Anywhere**.

- **macOS:** native Virtualization.framework; ad-hoc signed, not notarized. After
  the first blocked launch, use System Settings → Privacy & Security → Open Anyway.
- **Linux:** bundled QEMU/KVM, hardware virtualization and read/write `/dev/kvm`
  access required. Use `chmod +x CARE-Anywhere-*.AppImage`. Requires a browser and
  `xdg-open`; without FUSE 2 use `--appimage-extract-and-run` and allow extra
  temporary disk space. Never run as root.
- **Windows:** bundled QEMU/WHPX. Enable **Windows Hypervisor Platform** in Windows
  Features and CPU virtualization in firmware, then restart. Per-user installer;
  no separate QEMU installation. This alpha installer is unsigned.

## Changes

- Separate base, Python-runtime and app EROFS images with stable disk identifiers
  and initrd store overlay. App releases can replace app data without reinstalling
  the launcher. Explicit base ABI, protocol and exact runtime/base hash contracts.
- Download-only HTTPS update fetching, explicit trusted-local staging, activation
  on restart, cold pre-update snapshots, health confirmation and recovery into a
  new state directory. Signing/automatic activation remains gated for beta.
- `arm64-*` and `amd64-*` assets are standalone update layers, not installers.
  Their manifests support `update-fetch`; see [OTA instructions](https://github.com/tellmeY18/care_anywhere/blob/main/OTA.md).
- Excluded PostgreSQL JIT/LLVM; retained one precompiled Python bytecode variant
  and consolidated Django initialization to avoid repeated startup compilation/imports; stripped launchers,
  LZMA DMGs and only the required Windows QEMU target executable.
- **Format boundary:** this layered alpha refuses 0.1 preview data. Keep the old
  bundle and backup; start a new test clinic. Do not edit release metadata to force
  an upgrade. Schema-changing updates require a separately tested migration.

- Faster first boot: CI builds an empty migrated database seed. New clinics
  restore it transactionally instead of replaying historical schema changes;
  installation secrets and administrator accounts are still created locally.
- Shared QEMU lifecycle on Linux and Windows, retaining native macOS virtualization.
- Outbound-only NAT on all platforms. `CARE_NO_NETWORK=1` restricts outbound
  connectivity while retaining local control. Third-party features still require
  internet connectivity and their own service configuration.
- QEMU management uses a **127.0.0.1-only** forward with fresh per-boot mutual TLS
  credentials; macOS retains vsock. No management listener binds to the LAN.
- Windows state locking and kill-on-close VM job; Linux parent-death cleanup.
- AppImages and complete offline Windows installer built and published by CI.

## Verification and limits

Publication requires Go/UI checks, every platform package, Linux x86_64 QEMU/KVM
boot/setup/login/restart/encrypted-backup/restore acceptance, and the installed
Windows package's equivalent real-guest test using explicit **TCG** software
emulation. Hosted Windows CI has no WHPX; **physical Windows WHPX boot remains
unverified**. Hosted macOS cannot run nested Virtualization.framework guests;
macOS packaging/signatures are checked. Linux ARM64 hardware boot remains unverified.
TCG is an explicit CI/debug option, never an automatic user fallback.

Each download has a `.sha256` sidecar. For example:

```sh
shasum -a 256 -c CARE-Anywhere-{{VERSION}}-linux-amd64.AppImage.sha256
```

No signed automatic updates, LAN access, scheduled backups, recovery codes or
cross-architecture restore. Preserve both backup `.age` and `.key` files, with
the key stored separately. Stop CARE and back up before testing a new alpha.
Uninstalling preserves clinic data (`%APPDATA%\care-anywhere` on Windows).

[Architecture, CLI and build documentation](https://github.com/tellmeY18/care_anywhere/blob/main/README.md).
