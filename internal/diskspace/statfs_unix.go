//go:build darwin || linux

// Adapted from CARE Clinic. Copyright (c) 2026 Open Healthcare Network Foundation. MIT.
package diskspace

import "golang.org/x/sys/unix"

func statVolume(path string) (Usage, error) {
	var fs unix.Statfs_t
	if e := unix.Statfs(path, &fs); e != nil {
		return Usage{}, e
	}
	return Usage{Free: uint64(fs.Bavail) * uint64(fs.Bsize), Total: uint64(fs.Blocks) * uint64(fs.Bsize)}, nil
}
