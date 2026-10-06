package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDownloadIsVerifiedButNeverActivated(t *testing.T) {
	bundle, m := testLayerBundle(t)
	state := t.TempDir()
	requests := map[string]int{}
	corrupt := true
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/"+m.Arch+"-")
		requests[name]++
		if name == "app.img" && corrupt {
			w.Write([]byte("tampered"))
			return
		}
		http.ServeFile(w, r, filepath.Join(bundle, name))
	}))
	defer server.Close()
	// No production TLS bypass: only this test's HTTP transport trusts its CA.
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = oldTransport }()
	url := server.URL + "/" + m.Arch + "-manifest.json"
	if fetchUpdate(state, url) == nil {
		t.Fatal("accepted corrupt download")
	}
	mu.Lock()
	corrupt = false
	mu.Unlock()
	if err := fetchUpdate(state, url); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := requests["system.img"]
	mu.Unlock()
	if count != 1 {
		t.Fatal("redownloaded completed layer")
	}
	for _, name := range []string{"current.json", "pending.json", "release.json", "data.img"} {
		if _, err := os.Lstat(filepath.Join(state, name)); !os.IsNotExist(err) {
			t.Fatal("download activated clinic state", name)
		}
	}
	if _, err := verifyBundle(filepath.Join(state, "downloads", bundleID(m))); err != nil {
		t.Fatal(err)
	}
	if fetchUpdate(state, "http://example.com/manifest.json") == nil {
		t.Fatal("accepted insecure transport")
	}
}
