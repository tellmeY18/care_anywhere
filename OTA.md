# Layered releases and OTA

## Alpha 0.2 implementation

The launcher stays installed. The guest is three read-only EROFS disks, merged
by an initrd read-only OverlayFS at `/nix/store`. Virtio IDs `care-base`,
`care-runtime`, `care-app`, and `care-data` are identical on VZ and QEMU.
The base calls `/run/care/app/bin/{init,api,worker,beat,secrets,buckets}`;
the app provides that entrypoint and the Caddy include. No runtime installation
of Python packages, frontend packages, Nix, or containers is involved.

| Layer | Contents | Update trigger |
| --- | --- | --- |
| Facilitator | Native launcher, guest transport, desktop web panel, onboarding plugin, bundled host QEMU | Host/protocol/UI change; manual install during alpha |
| Base | Kernel/initrd, NixOS, PostgreSQL 17 without JIT, Redis, Silo/MinIO API, Caddy, agent, fonts | Security patches and tested NixOS train upgrades |
| Runtime | Python 3.13, pinned wheels and PDF/native dependencies | Lockfile or nixpkgs security change |
| App | CARE code, frontend, seed, startup scripts and Caddy fragment | CARE releases |

The runtime derivation no longer takes the CARE source tree as a build input.
The image builder subtracts base paths from runtime and base/runtime paths from
app. Paths are Nix input-addressed (not necessarily content-addressed); complete
store paths are immutable. Exact `requires_base` and `requires_runtime` hashes
prevent mixing a subtracted closure with the wrong lower layer. `base_abi` alone
is insufficient for this kind of closure subtraction.

Manifest format 2 includes `base_abi`, `requires_base_abi`, `protocol`,
`min_host_protocol`, `data_schema`, `migrates`, and all six file hashes.
The launcher rejects unsupported formats/protocols, mismatched layers, nonregular
files and unsafe paths before boot. Version text is not the schema gate.
Legacy format-0 preview data is **not** accepted by the layered base. Restore an
old backup with its old bundle; a tested logical migration is required to bring
that clinic into 0.2. Do not replace its `release.json` to bypass this guard.

## Available update commands

```sh
# Fetch into state/downloads; this never activates or establishes publisher trust.
care-anywhere update-fetch --state /clinic --url https://publisher/path/manifest.json

# Stop CARE, independently verify the source, then opt in to this unsigned alpha.
care-anywhere update-stage --state /clinic --bundle /trusted/download --trust-local
care-anywhere update-status --state /clinic
# Start normally: the installed app uses the state-owned layer set automatically.
```

The manifest and its named images must be sibling HTTPS objects. Redirects must
stay HTTPS. Downloads have bounded sizes, known lengths and free-space checks.
Completed files are hash-checked and reused after interruption; partial file
range resumption is not implemented. A crash-retained `.part` file must be
removed explicitly before retry. The active state-owned layer set can supply
unchanged files without network traffic. CI appliance artifacts contain all
images; alpha GitHub Releases publish desktop installers, not a signed channel.

Staging takes the clinic lock and requires it to be stopped in this alpha.
Verified files are copied into `layers/<manifest-hash>/`; existing verified
state-owned files can be hard-linked. The installer is never modified.
`pending.json` selects the next start; `current.json` selects subsequent starts.

Before activation the launcher snapshots the **stopped** data disk and its old
release metadata under `snapshots/before-*`. macOS uses native `clonefile` on
APFS; unsupported filesystems and other hosts use a checked exclusive copy.
Copies are synced before an `update.json` cutover journal is published. Every
update gets a snapshot, regardless of `migrates`: code can alter persisted data
even without Django migrations. Snapshots remain until deliberately removed;
automatic retention pruning is deferred until recovery/retention UI is tested.
Allow a full disk's spare capacity on copy-based hosts and room for APFS divergence.

A healthy status response confirms API, frontend, and the required services,
then clears the cutover journal. A failed or interrupted activation refuses
another start rather than guessing what migrations completed. Inspect
`update-status`, then recover with:

```sh
care-anywhere update-recover --state /clinic --destination /new-empty-clinic
care-anywhere serve --state /new-empty-clinic --bundle /previous-matching-bundle
```

The old snapshot is copied into a **new** destination. Original and post-update
data remain untouched. Records entered after the snapshot will not be in the
recovered clinic. There is no silent rollback. After successful health confirmation,
snapshots still contain `data.img` and `release.json`; retain the matching previous
bundle alongside disaster-recovery materials. Snapshots are not encrypted external
backups. Keep using the age backup/restore commands.

## First beta gate: signing and unattended delivery

Signing-key provisioning is deliberately deferred by the maintainer. The next
steps extend the implemented storage/activation path rather than replace it:

1. Provision an offline Ed25519 root, pin its public key in the facilitator, and
   authorize a CI release-signing key through a root-signed key list. Record key IDs,
   expiry and rotation/revocation policy; never ship private keys in artifacts.
2. Publish immutable per-architecture images plus a signed `channels/beta.json`
   containing exact hashes, byte counts, base/runtime requirements, data-schema
   transition, migration flag, expiry and monotonic release sequence. Sign exact
   bytes (or a specified canonical representation), not incidental JSON formatting.
3. Verify signatures, expiry, supported protocol, architecture and anti-rollback
   high-water mark before offering/staging an update. Persist the highest accepted
   sequence atomically. A recovery to a previous layer must not lower that mark.
   Offline operation keeps the last verified install; expired metadata prevents
   accepting new updates, not starting the existing clinic.
4. Add periodic background discovery/download and authenticated panel actions:
   “Update on next start”, “Restart to update”, migration/data-loss warning,
   progress, diagnostics and recovery. Move staging to its own lock so downloads
   can proceed during care; cutover retains the exclusive clinic lock.
5. CI derives migration fingerprints from Django's complete migration graph,
   including dependency packages; it checks forward transitions against the
   previous release. Until then alpha manifests conservatively say `migrates:true`.
   Schema-changing updates remain blocked. Test old-clinic upgrades and failed
   migrations before admitting a new schema transition.
6. Exercise power-loss boundaries, download interruption, wrong signatures,
   expired metadata, replay/downgrade, incompatible runtime/base, disk exhaustion,
   failed migrations and recovery on APFS, Linux and Windows. Require same-admin
   login plus attachment/PDF workflows after both update and recovery. A successful
   same-revision alpha cutover smoke test is not a migration acceptance test.
7. Developer ID/notarization + Sparkle for facilitator updates on macOS; until
   provisioned, offer the manual installer. Data-layer delivery does not modify
   the signed app. Keep the host/UI update frequency low; independently signed
   panel assets can follow once the panel protocol is versioned.

## Cadence and size gates

- Review nixpkgs monthly and urgently for relevant CVEs. The current unstable pin
  deliberately provides patched Silo; move to a stable branch when it contains
  that package. Soak NixOS train jumps for 4–8 weeks. Keep PostgreSQL 17 explicit;
  a major upgrade requires verified backup plus `pg_upgrade`/logical migration.
- Record per-image and packaged sizes in CI. Compare app-only releases to verify
  that base/runtime hashes are unchanged; native-wheel and nixpkgs bumps are
  expected to replace runtime and potentially base. No unsupported size promise.
- Alpha removes PostgreSQL LLVM/JIT, unnecessary wheel bytecode and unused Windows
  QEMU target executables, strips host Go binaries, and uses LZMA DMGs. It ships a
  64 MiB formatted seed rather than 8 GiB; only its private copy is extended to an
  8 GiB logical data disk. ext4 grows on boot and QEMU supports discard/unmap.
- Do not cut fontconfig/Pango/fonts (PDF rendering), bound Redis with an eviction
  policy (Celery messages), or change object-store formats without workload tests.
  Keep the 4 GiB VM allocation until service memory measurements justify lowering it.
- Custom kernels, smaller host QEMU builds, chunked deltas and logical scheduled
  backups follow measured bottlenecks. Full-image hashes and manifest verification
  remain mandatory even if transport later uses deltas.
