// Adapted from CARE Clinic. Copyright (c) 2026 Open Healthcare Network Foundation. MIT.
package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "private.json")
	for _, data := range []string{"first value", "replacement"} {
		if e := WritePrivate(path, []byte(data)); e != nil {
			t.Fatal(e)
		}
		got, e := os.ReadFile(path)
		if e != nil || string(got) != data {
			t.Fatal("private atomic replacement did not persist")
		}
		info, e := os.Stat(path)
		if e != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
			t.Fatal("private file is not owner restricted")
		}
	}
	if e := WritePrivate(dir, []byte("cannot replace directory")); e == nil {
		t.Fatal("replaced a directory")
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 1 {
		t.Fatal("left staging files")
	}
}
