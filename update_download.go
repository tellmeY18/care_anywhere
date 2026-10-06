package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/tellmeY18/care_anywhere/internal/diskspace"
)

// Download is separate from trust/activation. A hash supplied by the same
// unsigned server detects corruption, not publisher identity.
func fetchUpdate(state, address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("update manifest must use HTTPS")
	}
	client := &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || r.URL.Scheme != "https" {
			return errors.New("unsafe update redirect")
		}
		return nil
	}}
	resp, err := client.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("manifest download: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("update manifest too large")
	}
	var m manifest
	if err = json.Unmarshal(b, &m); err != nil {
		return err
	}
	if err = validateLayers(m); err != nil {
		return err
	}
	if m.Format != 2 || len(m.Files) != 6 || m.Kernel != "kernel" || m.Initrd != "initrd" {
		return errors.New("invalid update file set")
	}
	root := filepath.Join(state, "downloads")
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	unlock, err := lockState(root)
	if err != nil {
		return err
	}
	defer unlock()
	dest := filepath.Join(root, bundleID(m))
	if err = os.MkdirAll(dest, 0700); err != nil {
		return err
	}
	for _, name := range []string{"kernel", "initrd", "system.img", "runtime.img", "app.img", "data.img"} {
		digest := m.Files[name]
		if len(digest) != 64 {
			return errors.New("missing update checksum")
		}
		path := filepath.Join(dest, name)
		// Completed files survive an interrupted download; partial files are
		// discarded. Chunk/range resumption is unnecessary for small app layers.
		if fileHashMatches(path, digest) {
			continue
		}
		if _, e := os.Lstat(path); e == nil {
			return errors.New("existing downloaded file has wrong checksum; remove it before retrying")
		} else if !os.IsNotExist(e) {
			return e
		}
		active, e := readPointer(state, "current")
		if e == nil && fileHashMatches(filepath.Join(active, name), digest) {
			if e = copyExclusive(filepath.Join(active, name), path); e == nil {
				continue
			}
		}
		part := path + ".part"
		f, e := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(e) {
			return errors.New("interrupted partial download exists; remove only the .part file and retry")
		}
		if e != nil {
			return e
		}
		asset := u.ResolveReference(&url.URL{Path: m.Arch + "-" + name})
		r, e := client.Get(asset.String())
		if e == nil && r.StatusCode != 200 {
			e = fmt.Errorf("layer download: %s", r.Status)
		}
		if e == nil {
			space, spaceErr := diskspace.Of(dest)
			if spaceErr != nil {
				e = spaceErr
			} else if r.ContentLength < 0 || r.ContentLength > 8<<30 || uint64(r.ContentLength)+(256<<20) > space.Free {
				e = errors.New("unknown/oversized layer or insufficient disk space")
			}
		}
		if e == nil {
			var n int64
			n, e = io.Copy(f, io.LimitReader(r.Body, (8<<30)+1))
			if n > 8<<30 {
				e = errors.New("layer exceeds size limit")
			}
		}
		if r != nil {
			r.Body.Close()
		}
		if e == nil {
			e = f.Sync()
		}
		e = errors.Join(e, f.Close())
		if e != nil {
			os.Remove(part)
			return e
		}
		if !fileHashMatches(part, digest) {
			os.Remove(part)
			return errors.New("download checksum mismatch")
		}
		if e = os.Rename(part, path); e != nil {
			return e
		}
	}
	if err = saveJSON(filepath.Join(dest, "manifest.json"), m); err != nil {
		return err
	}
	if _, err = verifyBundle(dest); err != nil {
		return err
	}
	fmt.Printf("Downloaded (NOT authenticated or activated): %s\nReview provenance before update-stage --trust-local --bundle <path>.\n", dest)
	return nil
}
