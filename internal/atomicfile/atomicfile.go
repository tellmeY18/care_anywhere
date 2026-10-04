// Copyright (c) 2026 Open Healthcare Network Foundation.
// Copied from CARE Clinic under the MIT license. See THIRD_PARTY_NOTICES.md.
package atomicfile

import (
	"os"
	"path/filepath"
)

func Write(path string, data []byte, mode os.FileMode) error { return write(path, data, mode, false) }
func WritePrivate(path string, data []byte) error            { return write(path, data, 0600, true) }
func write(path string, data []byte, mode os.FileMode, private bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if private {
		if err = restrictPrivateFile(f.Name()); err != nil {
			f.Close()
			return err
		}
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(f.Name(), path)
}
