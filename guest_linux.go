package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/mdlayher/vsock"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func guestMain() error {
	var listener net.Listener
	var e error
	credentials, err := os.ReadFile("/sys/firmware/qemu_fw_cfg/by_name/opt/care/tls/raw")
	if err == nil {
		config, err := guestTLS(credentials, true)
		if err != nil {
			return err
		}
		listener, e = tls.Listen("tcp", ":8080", config)
	} else if os.IsNotExist(err) {
		listener, e = vsock.Listen(8080, nil)
	} else {
		return err
	}
	if e != nil {
		return e
	}
	defer listener.Close()
	var mutation sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method", 405)
			return
		}
		cmd := exec.CommandContext(r.Context(), "journalctl", "--no-pager", "-n", "100", "-u", "care-init", "-u", "care-api", "-u", "care-worker", "-u", "care-beat")
		cmd.Stdout = w
		cmd.Stderr = w
		_ = cmd.Run()
	})
	target, _ := url.Parse("http://127.0.0.1:8081")
	mux.Handle("/", httputil.NewSingleHostReverseProxy(target))
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method", 405)
			return
		}
		_, e := os.Stat("/var/lib/care/configured")
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:9000/ping/")
		healthy := err == nil && resp.StatusCode == 200
		if resp != nil {
			resp.Body.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"configured": e == nil, "healthy": healthy})
	})
	mux.HandleFunc("/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		if !mutation.TryLock() {
			http.Error(w, "busy", 409)
			return
		}
		defer mutation.Unlock()
		if _, e := os.Stat("/var/lib/care/configured"); e == nil {
			http.Error(w, "already configured", 409)
			return
		}
		var input struct{ Username, Password string }
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); e != nil || !singleLine(input.Username) || !singleLine(input.Password) || len(input.Password) < 12 || input.Username == "" {
			http.Error(w, "username and password (12+ characters) required", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		// Credentials use stdin, never command-line arguments or journal output.
		cmd := exec.CommandContext(ctx, "/run/current-system/sw/bin/care-admin")
		cmd.Stdin = bytes.NewReader(mustJSON(input))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if e := cmd.Run(); e != nil {
			http.Error(w, "Setup failed; inspect guest journal", 500)
			return
		}
		if e := atomicWrite("/var/lib/care/configured", []byte("1\n"), 0600); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		io.WriteString(w, "Clinic created")
	})
	mux.HandleFunc("/reset-password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		if !mutation.TryLock() {
			http.Error(w, "busy", 409)
			return
		}
		defer mutation.Unlock()
		if _, e := os.Stat("/var/lib/care/configured"); e != nil {
			http.Error(w, "not configured yet", 409)
			return
		}
		var input struct{ Username, Password string }
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); e != nil || !singleLine(input.Username) || !singleLine(input.Password) || len(input.Password) < 12 || input.Username == "" {
			http.Error(w, "username and new password (12+ characters) required", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		// Credentials use stdin, never command-line arguments or journal output.
		cmd := exec.CommandContext(ctx, "/run/current-system/sw/bin/care-admin-reset")
		cmd.Stdin = bytes.NewReader(mustJSON(input))
		if e := cmd.Run(); e != nil {
			http.Error(w, "No administrator with that username was found", 404)
			return
		}
		io.WriteString(w, "Password reset")
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		if !mutation.TryLock() {
			http.Error(w, "busy", 409)
			return
		}
		defer mutation.Unlock()
		// Firecracker exits on guest reboot; its minimal machine has no ACPI poweroff.
		action := "poweroff"
		if cmdline, e := os.ReadFile("/proc/cmdline"); e == nil {
			for _, arg := range strings.Fields(string(cmdline)) {
				if arg == "care.shutdown=reboot" {
					action = "reboot"
				}
			}
		}
		// Queue shutdown with systemd rather than stopping our own service before
		// this HTTP response reaches the host. The host still waits for VM exit
		// before releasing the clinic lock; this only acknowledges scheduling.
		if e := exec.Command("systemd-run", "--unit=care-shutdown", "--on-active=2s", "--timer-property=AccuracySec=100ms", "systemctl", action, "--no-block").Run(); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		io.WriteString(w, "Shutting down")
	})
	return (&http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}).Serve(listener)
}
func mustJSON(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
