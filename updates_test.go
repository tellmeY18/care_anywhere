package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testLayerBundle(t *testing.T) (string, manifest) {
	t.Helper()
	dir := t.TempDir()
	m := manifest{Version: "test", Arch: runtime.GOARCH, Format: 2, BaseABI: 1, RequiresBaseABI: 1, Protocol: 1, MinHostProtocol: 1, DataSchema: 1, DataBytes: 8 << 30, Kernel: "kernel", Initrd: "initrd", System: "/nix/store/test", Files: map[string]string{}, Migrates: true}
	for _, name := range []string{"kernel", "initrd", "system.img", "runtime.img", "app.img", "data.img"} {
		b := []byte(name)
		h := sha256.Sum256(b)
		m.Files[name] = hex.EncodeToString(h[:])
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m.RequiresBase, m.RequiresRuntime = m.Files["system.img"], m.Files["runtime.img"]
	if err := saveJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	return dir, m
}

func TestLayerContracts(t *testing.T) {
	dir, m := testLayerBundle(t)
	if _, err := verifyBundle(dir); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*manifest){
		func(m *manifest) { m.Format++ }, func(m *manifest) { m.RequiresBaseABI++ },
		func(m *manifest) { m.MinHostProtocol++ }, func(m *manifest) { m.RequiresRuntime = "bad" },
		func(m *manifest) { m.DataBytes = -1 }, func(m *manifest) { m.Protocol++ },
	} {
		next := m
		change(&next)
		if validateLayers(next) == nil {
			t.Fatal("accepted incompatible layer")
		}
	}
	next := m
	next.Version = "different-package-version"
	if !compatibleData(m, next) {
		t.Fatal("package version became a schema gate")
	}
	next.DataSchema++
	if compatibleData(m, next) {
		t.Fatal("accepted unplanned schema transition")
	}
	next = m
	next.Format = 0
	if compatibleData(next, m) {
		t.Fatal("accepted legacy preview data")
	}
	if err := os.Remove(filepath.Join(dir, "runtime.img")); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(dir); err == nil {
		t.Fatal("accepted missing runtime")
	}
}

func TestUpdateCutoverAndRecovery(t *testing.T) {
	source, m := testLayerBundle(t)
	state := t.TempDir()
	old := m
	old.Version = "previous"
	if err := saveJSON(filepath.Join(state, "release.json"), old); err != nil {
		t.Fatal(err)
	}
	data := []byte("clinic before migration")
	if err := os.WriteFile(filepath.Join(state, "data.img"), data, 0600); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockState(state)
	if err != nil {
		t.Fatal(err)
	}
	if stageUpdate(state, source) == nil {
		t.Fatal("staged while clinic running")
	}
	unlock()
	if err = stageUpdate(state, source); err != nil {
		t.Fatal(err)
	}
	unlock, err = lockState(state)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectBundle(state, source)
	if err != nil {
		t.Fatal(err)
	}
	if selected == source {
		t.Fatal("did not activate state-owned bundle")
	}
	if _, err = selectBundle(state, source); err == nil {
		t.Fatal("reboot accepted an unconfirmed cutover")
	}
	if err = os.WriteFile(filepath.Join(state, "data.img"), []byte("new records"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock()
	destination := filepath.Join(t.TempDir(), "recovered")
	if err = recoverUpdate(state, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "data.img"))
	if err != nil || string(got) != string(data) {
		t.Fatal("snapshot changed with live data", err)
	}
	if recoverUpdate(state, destination) == nil {
		t.Fatal("recovery overwrote a destination")
	}
	got, _ = os.ReadFile(filepath.Join(state, "data.img"))
	if string(got) != "new records" {
		t.Fatal("recovery destroyed post-update records")
	}
	if err = finishUpdate(state); err != nil {
		t.Fatal(err)
	}
	if current, err := selectBundle(state, source); err != nil || current != selected {
		t.Fatal(current, err)
	}
	if err = saveJSON(filepath.Join(state, "current.json"), "../escape"); err != nil {
		t.Fatal(err)
	}
	if _, err = selectBundle(state, source); err == nil {
		t.Fatal("accepted pointer traversal")
	}
}

func TestSmallSeedCreatesFullDiskWithoutOverwriting(t *testing.T) {
	bundle, m := testLayerBundle(t)
	state := t.TempDir()
	m.DataBytes = 1 << 20 // Exercise extension without allocating GiB in tests.
	if err := ensureData(state, bundle, m); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(state, "data.img"))
	if err != nil || info.Size() != m.DataBytes {
		t.Fatal(info, err)
	}
	if err = os.WriteFile(filepath.Join(state, "data.img"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = ensureData(state, bundle, m); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(state, "data.img"))
	if string(b) != "existing" {
		t.Fatal("reinitialized an existing disk")
	}
}
