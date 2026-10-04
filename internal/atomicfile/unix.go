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
	d, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
