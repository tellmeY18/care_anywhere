package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tellmeY18/care_anywhere/internal/atomicfile"
)

// Unsigned alpha transport is deliberately local and opt-in. These on-disk
// contracts also serve the signed channel downloader planned for beta.
func validateLayers(m manifest) error {
	if m.Format == 0 {
		return nil
	}
	if m.Format != 2 || m.BaseABI != 1 || m.RequiresBaseABI != m.BaseABI ||
		m.Protocol != 1 || m.MinHostProtocol < 1 || m.MinHostProtocol > 1 ||
		m.DataSchema < 1 || m.DataBytes < 1<<30 || m.DataBytes > 64<<30 ||
		len(m.RequiresBase) != 64 || len(m.RequiresRuntime) != 64 ||
		m.RequiresBase != m.Files["system.img"] || m.RequiresRuntime != m.Files["runtime.img"] {
		return errors.New("incompatible layered bundle: format, base ABI, protocol, runtime or data geometry")
	}
	return nil
}

func compatibleData(old, next manifest) bool {
	if old.Arch != next.Arch {
		return false
	}
	if old.Format == 0 || next.Format == 0 {
		return old.Format == next.Format && old.Version == next.Version
	}
	// Schema transitions need a dedicated migration release; never infer safety
	// from a version string or a publisher-supplied migrates=false alone.
	return old.Format == 2 && next.Format == 2 && old.BaseABI == next.BaseABI && old.DataSchema == next.DataSchema
}

func readManifest(path string) (manifest, error) {
	var m manifest
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	return m, err
}

func bundleID(m manifest) string {
	b, _ := json.Marshal(m)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fileHashMatches(path, want string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return err == nil && hex.EncodeToString(h.Sum(nil)) == want
}

// Recovery never rolls a live clinic backwards or overwrites patient records.
// It publishes the pre-update snapshot into a new, empty state directory.
func recoverUpdate(state, destination string) error {
	if destination == "" {
		return errors.New("--destination must name a new empty state directory")
	}
	unlock, err := lockState(state)
	if err != nil {
		return err
	}
	defer unlock()
	b, err := os.ReadFile(filepath.Join(state, "update.json"))
	if err != nil {
		return err
	}
	var journal map[string]string
	if err = json.Unmarshal(b, &journal); err != nil {
		return err
	}
	snapshot := journal["snapshot"]
	if filepath.Dir(snapshot) != filepath.Join(state, "snapshots") || filepath.Base(snapshot) == "." {
		return errors.New("invalid recovery snapshot path")
	}
	info, err := os.Lstat(snapshot)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("snapshot must be a real directory")
	}
	for _, name := range []string{"release.json", "data.img"} {
		info, err := os.Lstat(filepath.Join(snapshot, name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("invalid snapshot member")
		}
	}
	// Mkdir, not MkdirAll: a pre-existing destination is never accepted.
	if err = os.Mkdir(destination, 0700); err != nil {
		return err
	}
	unlockDestination, err := lockState(destination)
	if err != nil {
		return err
	}
	defer unlockDestination()
	stage, err := os.MkdirTemp(destination, "recover-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = snapshotDisk(filepath.Join(snapshot, "data.img"), filepath.Join(stage, "data.img")); err != nil {
		return err
	}
	m, err := readManifest(filepath.Join(snapshot, "release.json"))
	if err != nil {
		return err
	}
	if err = saveJSON(filepath.Join(destination, "release.json"), m); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(stage, "data.img"), filepath.Join(destination, "data.img")); err != nil {
		return err
	}
	if err = atomicfile.SyncDir(destination); err != nil {
		return err
	}
	fmt.Println("Pre-update data recovered into", destination, "— start it with the previous matching bundle. The failed clinic and all post-update data remain untouched.")
	return nil
}

func readPointer(state, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(state, name+".json"))
	if err != nil {
		return "", err
	}
	var id string
	if err = json.Unmarshal(b, &id); err != nil {
		return "", err
	}
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != id {
		return "", errors.New("invalid layer pointer")
	}
	return filepath.Join(state, "layers", id), nil
}

func stageUpdate(state, source string) error {
	if source == "" {
		return errors.New("--bundle must name a trusted local layered bundle")
	}
	m, err := verifyBundle(source)
	if err != nil {
		return err
	}
	if m.Format != 2 || m.Arch != runtime.GOARCH {
		return errors.New("update requires a layered bundle for this architecture")
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		return err
	}
	unlock, err := lockState(state)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err = os.Lstat(filepath.Join(state, "update.json")); !os.IsNotExist(err) {
		return errors.New("resolve the previous update before staging another")
	}
	old, err := readManifest(filepath.Join(state, "release.json"))
	if err != nil {
		return err
	}
	if !compatibleData(old, m) {
		return errors.New("incompatible data schema/base ABI; preview data requires a separate migration, not an in-place update")
	}
	root := filepath.Join(state, "layers")
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	id := bundleID(m)
	dest := filepath.Join(root, id)
	if _, err = os.Lstat(dest); os.IsNotExist(err) {
		stage, err := os.MkdirTemp(root, "stage-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		// Reuse verified immutable files from the active layer set. An app-only
		// update does not need a second copy of the OS or Python runtime.
		active, _ := readPointer(state, "current")
		var current manifest
		if active != "" {
			verified, e := verifyBundle(active)
			if e == nil {
				current = verified
			}
		}
		for name, digest := range m.Files {
			out := filepath.Join(stage, name)
			if current.Files[name] == digest && os.Link(filepath.Join(active, name), out) == nil {
				continue
			}
			if err = copyExclusive(filepath.Join(source, name), out); err != nil {
				return err
			}
			if err = os.Chmod(out, 0400); err != nil {
				return err
			}
		}
		if err = saveJSON(filepath.Join(stage, "manifest.json"), m); err != nil {
			return err
		}
		if _, err = verifyBundle(stage); err != nil {
			return err
		}
		if err = os.Rename(stage, dest); err != nil {
			return err
		}
		if err = atomicfile.SyncDir(root); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err = verifyBundle(dest); err != nil {
		return err
	}
	if err = saveJSON(filepath.Join(state, "pending.json"), id); err != nil {
		return err
	}
	fmt.Println("Update staged for next start. A cold recovery snapshot will be created before activation. Signing is not enabled; this bundle was explicitly trusted locally.")
	return nil
}

// selectBundle runs only with the clinic's OS lock held. Every update gets a
// cold snapshot, even a non-migrating one: app code can change persistent state.
// A crash during cutover fails closed; we never guess which schema is on disk.
func selectBundle(state, fallback string) (string, error) {
	if _, err := os.Lstat(filepath.Join(state, "update.json")); !os.IsNotExist(err) {
		return "", errors.New("an update was interrupted or has not passed health checks; run update-status and recover from the retained snapshot into an empty state directory")
	}
	next, err := readPointer(state, "pending")
	if os.IsNotExist(err) {
		current, e := readPointer(state, "current")
		if os.IsNotExist(e) {
			return fallback, nil
		}
		return current, e
	}
	if err != nil {
		return "", err
	}
	m, err := verifyBundle(next)
	if err != nil {
		return "", err
	}
	old, err := readManifest(filepath.Join(state, "release.json"))
	if err != nil {
		return "", err
	}
	if !compatibleData(old, m) {
		return "", errors.New("pending update no longer matches clinic data")
	}
	snapshots := filepath.Join(state, "snapshots")
	if err = os.MkdirAll(snapshots, 0700); err != nil {
		return "", err
	}
	snapshot, err := os.MkdirTemp(snapshots, "before-")
	if err != nil {
		return "", err
	}
	if err = snapshotDisk(filepath.Join(state, "data.img"), filepath.Join(snapshot, "data.img")); err != nil {
		return "", err
	}
	if err = saveJSON(filepath.Join(snapshot, "release.json"), old); err != nil {
		return "", err
	}
	if err = atomicfile.SyncDir(snapshot); err != nil {
		return "", err
	}
	if err = atomicfile.SyncDir(snapshots); err != nil {
		return "", err
	}
	if err = saveJSON(filepath.Join(state, "update.json"), map[string]string{"snapshot": snapshot, "bundle": next, "phase": "activating"}); err != nil {
		return "", err
	}
	if err = saveJSON(filepath.Join(state, "current.json"), bundleID(m)); err != nil {
		return "", err
	}
	if err = saveJSON(filepath.Join(state, "release.json"), m); err != nil {
		return "", err
	}
	if err = os.Remove(filepath.Join(state, "pending.json")); err != nil {
		return "", err
	}
	return next, atomicfile.SyncDir(state)
}

func finishUpdate(state string) error {
	err := os.Remove(filepath.Join(state, "update.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return atomicfile.SyncDir(state)
}

func updateStatus(state string) error {
	for _, name := range []string{"current", "pending", "update"} {
		b, err := os.ReadFile(filepath.Join(state, name+".json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", name, b)
	}
	return nil
}
