# Updating CARE Anywhere (alpha OTA guide)

Practical steps for updating the clinic **appliance** (base / runtime / app
layers) without reinstalling the launcher. Design and contracts: [OTA.md](../OTA.md).

> **Alpha limits.** Releases are not signed yet (planned for beta). A download
> hash only detects corruption, not who published it, so activation is always an
> explicit `--trust-local` step. Updates are applied **while CARE is stopped**.
> Clinics created with 0.1 preview builds cannot be updated to 0.2: keep the old
> bundle and a backup, and start a new test clinic.

## 0. Before you start

1. **Stop CARE** (panel: *Stop CARE*; or `care-anywhere stop`). Staging refuses
   to run while the clinic is running.
2. **Back up** (`care-anywhere backup --state <dir> --file <path>.age`) and store
   the `.key` file separately. Update snapshots are *not* a substitute.
3. Make sure you have free disk: roughly one extra disk's worth on Linux/Windows
   (the snapshot is a full copy); macOS/APFS uses a near-free clone.

## 1. Find the launcher binary and state directory

| OS | Binary | State directory |
| --- | --- | --- |
| macOS | `/Applications/CARE Anywhere.app/Contents/MacOS/care-anywhere` | `~/Library/Application Support/care-anywhere` |
| Linux | AppImage: run `./CARE-Anywhere-*.AppImage --appimage-extract`, then use `squashfs-root/care-anywhere` (the AppImage itself always opens the desktop panel) | `${XDG_CONFIG_HOME:-~/.config}/care-anywhere` |
| Windows | `care-anywhere.exe` in the install folder (GUI build prints nothing; run from a terminal and check `launcher.log`/exit code) | `%APPDATA%\care-anywhere` |

Examples below use `care-anywhere` and `$STATE`.

## 2. Download the new layers

Each release publishes per-architecture files (`arm64-*` or `amd64-*`:
`manifest.json`, `kernel`, `initrd`, `system.img`, `runtime.img`, `app.img`,
`data.img`, plus `.sha256` sidecars).

```sh
care-anywhere update-fetch --state "$STATE" \
  --url https://github.com/tellmeY18/care_anywhere/releases/download/v0.2.0-alpha.39/arm64-manifest.json
```

Use `amd64-manifest.json` on x86_64. The command downloads over HTTPS only,
checks every file's SHA-256 and size, reuses layers you already have, and prints
the folder it wrote (`$STATE/downloads/<id>`). It **never activates** anything.
If a `.part` file remains after an interruption, delete it and rerun.

Prefer manual download? Put all files for one architecture in a folder, rename
away the `arm64-`/`amd64-` prefix, keep `manifest.json`, and use that folder in
step 3. Verify with the `.sha256` sidecars first.

## 3. Stage the update

```sh
care-anywhere update-stage --state "$STATE" \
  --bundle "$STATE/downloads/<id>" --trust-local
care-anywhere update-status --state "$STATE"
```

`--trust-local` means *you* vouch for where the files came from. Staging checks
the base ABI, host protocol, runtime/base hashes and data schema, and refuses an
incompatible or schema-changing update. It copies layers into
`$STATE/layers/<id>` and writes `pending.json`. `update-status` should list
`pending`.

## 4. Restart to apply

Start CARE normally (panel *Start CARE*). On that start the launcher:

1. creates a **cold snapshot** of the data disk (`$STATE/snapshots/before-*`),
2. writes an `update.json` journal and switches to the new layers,
3. clears the journal only after the guest reports healthy (API, frontend and all
   services up).

Check `update-status`: `update` should be gone and `current` should show the new id.
Then sign in and try a normal workflow before resuming real use.

## 5. If something goes wrong

If activation fails or is interrupted, CARE refuses to start (it will not guess
what state the data is in), and `update-status` still shows `update`. Recover the
pre-update data into a **new, empty** directory; live data is never overwritten:

```sh
care-anywhere update-recover --state "$STATE" --destination /path/to/new-clinic
care-anywhere serve --state /path/to/new-clinic --bundle <previous-matching-bundle>
```

Records entered after the snapshot are not in the recovered copy; they remain in
the original state directory. Keep the previous bundle (the app's installed
`bundle` folder, or your old download) until you are satisfied.

## Cleaning up

Snapshots and old `layers/*` are kept until you remove them. After a confirmed
healthy update plus a fresh backup you can delete old `snapshots/before-*`
folders. Do not delete `current.json`, `pending.json`, `update.json`, or
`release.json`.

## Not available until beta

Signed channel manifests, automatic background download, an in-panel
"Update on restart" button, rollback by swapping layers, and facilitator
(launcher) self-update. See the beta gates in [OTA.md](../OTA.md).
