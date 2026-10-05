# CARE Anywhere — multiplatform alpha

Local-first CARE EMR: clinic data, database and compute stay on your computer.
Complete offline packages include the native launcher, Linux appliance, desktop
control panel and onboarding plugin. No Docker, Nix, Python, Node, Git or separately
installed QEMU required. **Early alpha: use test records.**

## Downloads and installation

| Platform | Download | Start |
| --- | --- | --- |
| macOS 13+ Apple Silicon | `CARE-Anywhere-0.1.0-alpha.2-macos-arm64.dmg` | Drag to Applications, eject, open CARE Anywhere |
| Linux x86_64 | `CARE-Anywhere-0.1.0-alpha.2-linux-amd64.AppImage` | Mark executable, then open |
| Linux ARM64 | `CARE-Anywhere-0.1.0-alpha.2-linux-arm64.AppImage` | Mark executable, then open |
| Windows 10/11 x86_64 | `CARE-Anywhere-0.1.0-alpha.2-windows-amd64.exe` | Run installer, open CARE Anywhere from Start |

Allow **8 GB RAM and 22 GB free disk**, plus temporary installation/extraction and
backup space. First preparation copies an 8 GiB data disk and runs migrations;
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
shasum -a 256 -c CARE-Anywhere-0.1.0-alpha.2-linux-amd64.AppImage.sha256
```

No supported in-place updates, LAN access, scheduled backups, recovery codes or
cross-architecture restore. Preserve both backup `.age` and `.key` files, with
the key stored separately. Stop CARE and back up before testing a new alpha.
Uninstalling preserves clinic data (`%APPDATA%\care-anywhere` on Windows).

[Architecture, CLI and build documentation](https://github.com/tellmeY18/care_anywhere/blob/release/alpha/README.md).
