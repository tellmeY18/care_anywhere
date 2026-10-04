// Adapted from CARE Clinic app/internal/sys/diskspace. MIT.
// Copyright (c) 2026 Open Healthcare Network Foundation.
package diskspace

import (
	"errors"
	"os"
	"path/filepath"
)

type Usage struct {
	Path        string
	Free, Total uint64
}

func Of(path string) (Usage, error) {
	if path == "" {
		return Usage{}, errors.New("no folder given")
	}
	p, e := filepath.Abs(path)
	if e != nil {
		return Usage{}, e
	}
	for {
		if _, e := os.Stat(p); e == nil {
			u, e := statVolume(p)
			u.Path = p
			return u, e
		} else if !os.IsNotExist(e) {
			return Usage{}, e
		}
		parent := filepath.Dir(p)
		if parent == p {
			return Usage{}, errors.New("no part of path exists")
		}
		p = parent
	}
}
