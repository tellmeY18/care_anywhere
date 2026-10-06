package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func snapshotDisk(src, dst string) error {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EXDEV) {
			return copyExclusive(src, dst)
		}
		return err
	}
	f, err := os.OpenFile(dst, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
