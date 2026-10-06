//go:build darwin || linux

// Copyright (c) 2026 Open Healthcare Network Foundation. MIT.
package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
)

func restrictPrivateFile(string) error { return nil }
func replace(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return SyncDir(filepath.Dir(to))
}

func SyncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
