# AGENTS.md

Guidance for AI coding agents working in this repository. Read this fully before
making changes — this project has several non-obvious constraints that are easy
to violate by instinct (e.g. "just fix the formatter output" or "add a container
runtime").

## What this is

CARE Anywhere is an experimental **single-binary native launcher** that boots a
prebuilt, local-first, Linux microVM appliance containing the full
[CARE](https://github.com/ohcnetwork/care) EMR stack (Django API, Celery worker,
Celery beat, PostgreSQL, Redis, MinIO, Caddy, prebuilt React frontend). The goal
is "install CARE like a browser" — no Docker, no Nix, no Python, no Node, no Git
required on the end-user's machine. Those tools are only needed to **build** the
appliance; the shipped binary + bundle requires none of them at runtime.

This is a sibling project to [`care_clinic`](https://github.com/ohcnetwork/care_clinic)
(the Docker Compose + Wails desktop app). We deliberately reuse CARE Clinic's
battle-tested Go internals (atomic file writes, disk-space checks, Windows ACL
handling) rather than reinventing them — see "CARE Clinic reuse" below.

**Read `README.md` first** for the full verified feature list, build commands,
and known gaps before touching anything. This file is about *how to work on the
code*, not what the code does.

## Architecture at a glance

```
main.go            CLI entrypoint: serve | status | stop | logs | backup | restore
vm.go               machine interface (Dial/Done/Close)
vm_darwin.go        Apple Virtualization.framework (Code-Hex/vz) — no QEMU on macOS
vm_qemu.go          shared bundled QEMU lifecycle, authenticated loopback TLS transport
vm_linux.go         KVM checks and parent-death handling
vm_windows.go       WHPX checks and kill-on-close Windows Job Object
guest_linux.go      guest agent: vsock on macOS, mutual TLS on QEMU
guest_other.go      stub for non-Linux GOOS (guest code only builds for Linux target)
storage.go          bundle verification, backup (age-encrypted tar.gz), restore
lock_unix.go/
lock_windows.go     OS-level advisory lock on clinic state dir (flock / LockFileEx)
internal/atomicfile Adapted from CARE Clinic (MIT) — atomic file replacement + Windows ACLs
internal/diskspace  Adapted from CARE Clinic (MIT) — free-space checks
nix/                The ENTIRE appliance build: NixOS guest config, Python env, frontend build
flake.nix           Pins: nixpkgs, microvm.nix, care (backend) repo, care_fe (frontend) repo
```

The binary you build locally (`go build -o dist/care-anywhere .`) is the **host
launcher only**. The actual CARE appliance (kernel + initrd + system.img +
data.img) is built separately via Nix (`nix build .#packages.<system>.bundle`)
and copied next to the launcher. `dist/` is gitignored — never commit built
artifacts, bundles, or the `.state/` runtime directory.

## Hard constraints — do not violate these

1. **No containers, ever.** If you're tempted to add Docker/Podman/OCI anywhere
   in this repo, stop — that's `care_clinic`'s job, not this one. The entire
   point of this project is proving a container-free, VM-based alternative.

2. **Local-first, not offline-only.** Clinic data, compute and storage stay on
   this computer — that part is non-negotiable. The guest gets an
   **outbound-only NAT network device by default on all platforms** (see
   `vm_darwin.go`, disable with `CARE_NO_NETWORK=1`) so features that
   genuinely need live internet (e.g. SNOMED code validation via the
   Snowstorm terminology server at `SNOWSTORM_DEPLOYMENT_URL`) work instead of
   500ing. macOS control/browser traffic uses **vsock**. Linux and Windows use
   QEMU user-mode NAT with one **127.0.0.1-only** forward to the guest agent,
   protected by fresh per-boot mutual TLS credentials delivered through fw_cfg.
   Never expose a raw unauthenticated management endpoint or bind to the LAN.
   CARE_NO_NETWORK=1 restricts QEMU egress while preserving local control.

3. **Dependencies install at Nix build time, never at guest boot time.** The
   whole value proposition collapses if the guest runs `pip install` or
   `npm install` on first boot. Python wheels are resolved and hash-pinned by
   `scripts/python-artifacts.py` into `nix/python-{aarch64,x86_64}.json`; the
   frontend's `package-lock.json` entries are backfilled with registry URLs by
   `scripts/frontend-lock.py` into `nix/frontend-lock.json`. If CARE's
   `Pipfile.lock` or `care_fe`'s `package-lock.json` changes upstream, regenerate
   these files — don't hand-edit them, and don't add runtime install steps as a
   workaround.

4. **The data disk is sacred.** `data.img` holds PostgreSQL, Redis persistence,
   object storage, generated secrets (`/var/lib/care/runtime.env`), and static
   files — it is the only thing that should differ between "fresh install" and
   "clinic with six months of patient records." `system.img` (the application
   code + Python env + frontend) must stay disposable/replaceable. When adding
   anything new that needs to persist, make sure it's rooted under `/var/lib`
   (the data disk's mount point in `nix/guest.nix`), not baked into the Nix
   store closure.

5. **Backup/restore safety invariants are non-negotiable:**
   - `backup()` refuses to run against a *running* clinic (must hold the OS
     lock first) — see `lock_unix.go` / `storage.go`.
   - `backup()` never overwrites an existing backup file or its `.key` sibling
     (`os.O_EXCL`).
   - `restore()` only accepts an **empty destination directory** — it must never
     silently clobber existing clinic data.
   - Corrupted/tampered archives must fail *before* any data is published to
     disk (see the AEAD-drain-before-publish logic in `storage.go`).
   - `storage_test.go` encodes these invariants as regression tests — if you
     touch `storage.go`, run `go test -race ./...` and consider adding a case
     for whatever you changed, not just making existing tests pass.

6. **Cross-platform `GOOS` discipline.** `lock_unix.go`/`lock_windows.go`,
   `vm_darwin.go`/`vm_linux.go`/`vm_windows.go`, and `guest_linux.go`/
   `guest_other.go` all use build tags. When adding a new platform-specific
   capability, follow the existing stub pattern (return a clear "not yet
   supported" error) rather than leaving a platform without a buildable file.

7. **Manifest/bundle integrity.** `verifyBundle()` in `storage.go` checks
   `manifest.json` SHA-256 hashes for every file and rejects path traversal in
   filenames (`filepath.Base(n) != n`). If you add new files to the bundle
   (`nix/bundle.nix`), you must add them to the manifest and to this check —
   never trust bundle contents implicitly.

## Build and verification workflow

This is the loop that was actually used to get this repo to its current
(verified-booting) state. Follow it rather than guessing.

For release work, run builds, compilation-based tests and packaging in GitHub
Actions. When the user says no local builds, that includes local Go tests and
cross-compiles. Formatting and static source inspection are still allowed.

```sh
# 1. Fast local checks (every change, before anything else)
go fmt ./...
go vet ./...
go test -race ./...

# 2. Local macOS launcher build + signing (required to run on Apple Silicon —
#    unsigned binaries cannot use the Virtualization.framework entitlement)
go build -o dist/care-anywhere .
codesign --force --sign - --entitlements entitlements.plist dist/care-anywhere

# 3. Cross-compile sanity for other host platforms (no VM boot test possible
#    locally on macOS for these — just confirms the launcher compiles)
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/care-anywhere-linux .
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/care-anywhere-linux-amd64 .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/care-anywhere.exe .

# 4. Appliance build (Linux-only derivation; needs a Linux builder from macOS).
#    This repo was built against an aarch64-linux remote builder over SSH
#    because `nix build` needs a Linux machine to produce the NixOS guest closure.
nix build --store ssh-ng://<linux-builder-host> --eval-store auto --no-link \
  --print-out-paths .#packages.aarch64-linux.bundle
```

**Do not `nix copy` the full build closure back to macOS** — it drags in the
entire Nix store graph (gigabytes, many thousands of paths) and will likely
exhaust local disk. Instead, export only the four files the launcher actually
needs:

```sh
ssh <linux-builder-host> \
  'tar -C /nix/store/<hash>-care-anywhere-bundle -czf - manifest.json kernel initrd system.img data.img' \
  | tar -xzf - -C dist
```

If the transfer is interrupted, re-run with `rsync -azc --partial` per-file
rather than re-tarring everything — `data.img` in particular is large (8 GiB)
and a half-copied file will fail `verifyBundle()`'s checksum check with a clear
error (seen in practice during this build — don't be alarmed, just re-sync).

### Boot-testing the appliance

```sh
nohup ./dist/care-anywhere serve --bundle ./dist --state ./.state \
  > dist/launcher.log 2>&1 &
sleep 30   # first boot runs Django migrations; can take 30–90s
./dist/care-anywhere status --state ./.state   # {"configured":false,"healthy":true}
./dist/care-anywhere logs   --state ./.state    # journalctl from care-init/api/worker/beat
./dist/care-anywhere stop   --state ./.state
```

`status` reports `healthy` once `/ping/` responds inside the guest, and
`configured` once an administrator has been created via `/control/setup`. Both
being false after >2 minutes means something is actually broken — check
`logs`, not just the launcher's own stdout (`dist/launcher.log` only has host-
side errors; guest-side Django tracebacks only show up via the `logs` command
or `.state/console.log`).

### Known build pitfalls (already hit and fixed once — don't rediscover)

- **Pillow/`libzstd` ImportError at guest boot**: Nix's generic ELF-shrinking
  pass strips RPATHs that auditwheel-bundled native wheels need. Fixed via
  `dontPatchELF = true;` in `nix/python.nix`. If you see
  `ImportError: lib*.so.*: cannot open shared object file` from any Python C
  extension, check this setting before anything else.
- **`python-magic` wheel tagged `cp312-cp312-...` but is actually a pure
  ctypes wrapper**: its WHEEL metadata is wrong upstream. Fixed by a
  `substituteInPlace` in `nix/python.nix` rewriting the tag to `py3-none-...`
  before `pip check`. Don't "fix" this by pinning a different python-magic
  version — the tag rewrite is correct and intentional.
- **npm lockfile missing `resolved`/`integrity` fields**: `care_fe`'s
  `package-lock.json` (as pinned) has gaps for some transitive deps. Fixed via
  `scripts/frontend-lock.py`, which fills in registry metadata **without
  changing any version**. Re-run it if the pin advances; don't hand-add
  `npmDepsHash = lib.fakeHash` and declare victory from the resulting mismatch
  error — that only tells you the *content* hash, not that the lock is valid.
- **Superuser creation**: `care.users.models.CustomUserManager.create_superuser`
  requires `email` as a positional arg even if blank — `nix/guest.nix`'s
  inline admin-creation script passes `email=""` explicitly. CARE's own
  `create_superuser` override is NOT optional-email-friendly; don't assume
  Django's default manager signature applies.
- **`postgresql.service` and `redis.service` systemd units**: `care-init`
  depends on `postgresql.target` (not `postgresql.service` — NixOS splits these)
  and `redis.service`. Double-check target vs. service unit names when adding
  new systemd dependencies in `nix/guest.nix` — NixOS modules don't always use
  the name you'd expect from the plain package.
- **`path:.` as a flake build input on macOS copies untracked files** (observed:
  staged `.state/`'s multi-GB disk images almost filled a build sandbox). Only
  `git add` the files that should be Nix build inputs before running
  `nix build path:.#...`, or use the pushed `github:tellmeY18/care_anywhere` ref
  instead of a local path during iteration.

## CARE Clinic reuse policy

Before writing new Go code for anything filesystem-safety or OS-integration
related, **check if `care_clinic` already solved it**:
https://github.com/ohcnetwork/care_clinic (MIT licensed, same org).

Already ported (see `THIRD_PARTY_NOTICES.md` for exact provenance):

- `internal/atomicfile` — atomic file replacement, fsync-before-rename,
  Windows protected-DACL private files.
- `internal/diskspace` — nearest-existing-ancestor lookup + free/total space.
- `nix/buckets.py` — MinIO bucket bootstrap + public facility-download policy,
  ported from `deployments/minio/entrypoint.sh` to native Python (no shell
  `mc` client needed since there's no container to exec into).

**Not yet ported — do not casually reinvent, check upstream first:**

- Wails desktop UI, setup wizard, requirement checks.
- mDNS/`.local` LAN discovery, TLS trust-on-first-use, client onboarding.
- Scheduled (cron-like) backups with retention pruning — this repo only has
  manual `backup`/`restore` CLI commands today.
- Recovery-code / password-reset UX (`app/app_recovery.go` in care_clinic).
- Staged/journaled restore with rollback phases (`restore_journal.go`) — this
  repo's restore is simpler (all-or-nothing into an empty dir) and does **not**
  yet have care_clinic's `staging → prepared → committed → rolled-back` state
  machine. If you're asked to make restore safer/resumable, read
  `care_clinic/docs/backups-and-restore.md` first.
- Signed update channel / staged image retagging (`compose/update.go`).

When porting something new from care_clinic, preserve its safety invariants
(atomicity, refuse-to-overwrite, fail-closed on ambiguous state) — don't just
copy the happy path.

## Code style

- This repo's Go style is deliberately terse/dense (minimal line breaks, `;`
  statement separators in some spots) to keep the single-file CLI readable at
  a glance — match the existing style in a given file rather than reformatting
  wholesale. `go fmt` is still run and must pass cleanly.
- Nix files use 2-space indentation, matching nixpkgs convention.
- Python helper scripts (`nix/secrets.py`, `nix/buckets.py`, `scripts/*.py`)
  are intentionally small, dependency-light (stdlib + boto3/authlib only where
  CARE itself already depends on them), and run as oneshot systemd services or
  build-time tooling — not long-running services.

## Things to never do

- Never add Docker, Podman, or any container runtime. Bundled QEMU is the
  supported Linux (KVM) and Windows (WHPX) runtime; macOS keeps native
  Virtualization.framework. Users must not install QEMU separately. Explicit
  CARE_QEMU_ACCEL=tcg is available for slow CI/debug boot testing only.
- Never commit `dist/`, `.state/`, `*.img`, or any built bundle — `.gitignore`
  already excludes `/dist/`, `/result*`, `/.state/`, `*.log`. If you add new
  build output directories, extend `.gitignore` rather than relying on `git
  status` vigilance.
- Never weaken `verifyBundle()`'s hash/path-traversal checks "to make testing
  easier" — if a build-time hash mismatch is blocking you, regenerate the
  manifest, don't bypass the check.
- Never make `restore()` able to write into a non-empty directory, even behind
  a flag. If overwrite/force-restore is genuinely needed, it needs its own
  explicit, loudly-labeled code path with a safety backup step first (see how
  care_clinic's update flow takes a `care-pre-update-*.dump.enc` safety backup
  before any destructive retag) — not a silent relaxation of the existing check.
- Never assume the guest has internet access when writing guest-side code
  (`nix/guest.nix`, `guest_linux.go`) — email, SMS, hosted plugins, and any
  outbound HTTP call can fail without connectivity. Handle and document those
  failures; default NAT connectivity is not a promise of internet availability.

## Where to look for more context

- `README.md` — user-facing feature list, verified test results, build
  commands, and the explicit "before an end-user release" TODO list. Keep it
  in sync with reality; update it in the same commit as any behavior change.
- `THIRD_PARTY_NOTICES.md` — exact provenance and license text for every
  ported component.
- `flake.nix` / `flake.lock` — exact pinned commits of `care` and `care_fe`.
  Bumping these pins is how you pick up upstream CARE changes; do it
  deliberately (re-run `scripts/python-artifacts.py` and
  `scripts/frontend-lock.py` afterward) rather than letting Nix silently build
  against a different app version than the hashes expect.
- Sibling repo for cross-reference: `../care` (the main CARE Django backend
  this appliance packages) and `../care_fe` (the frontend) — useful for
  understanding what a given Django management command or frontend env var
  actually does when debugging `nix/guest.nix`.
