package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tellmeY18/care_anywhere/internal/diskspace"
)

func openURL(url string) error {
	if os.Getenv("CARE_NO_BROWSER") == "1" {
		return nil
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Run()
	}
	return exec.Command("xdg-open", url).Run()
}

// friendlyStartError turns the last lines of the launcher log into plain-language
// guidance. Falls back to a message that still points at the diagnostics panel,
// which the UI now always shows a button for (never a dead end for the reader).
func friendlyStartError(state string) string {
	tail := ""
	if b, e := os.ReadFile(filepath.Join(state, "launcher.log")); e == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) > 20 {
			lines = lines[len(lines)-20:]
		}
		tail = strings.ToLower(strings.Join(lines, "\n"))
	}
	switch {
	case strings.Contains(tail, "insufficient disk space"):
		return "Not enough free disk space. CARE needs about 9 GB free to prepare your clinic. Free up space on this computer, then open Diagnostics below and press Try again."
	case strings.Contains(tail, "checksum mismatch") || strings.Contains(tail, "invalid manifest") || strings.Contains(tail, "invalid bundle"):
		return "Some app files look incomplete or damaged. Try reinstalling CARE Anywhere from a fresh download."
	case strings.Contains(tail, "address already in use") || strings.Contains(tail, "bind: "):
		return "Another program on this computer is using the port CARE needs. Close other clinic windows, restart this computer if needed, then open Diagnostics below and press Try again."
	case strings.Contains(tail, "kvm") || strings.Contains(tail, "/dev/kvm"):
		return "This computer needs hardware virtualization enabled, and your user needs access to it. Ask your administrator, then open Diagnostics below and press Try again."
	default:
		return "CARE could not start. Open Diagnostics below to see what happened, then press Try again."
	}
}

func defaultBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" && filepath.Base(filepath.Dir(exe)) == "MacOS" {
		return filepath.Join(filepath.Dir(exe), "../Resources/bundle"), nil
	}
	return filepath.Join(filepath.Dir(exe), "bundle"), nil
}

func openDesktop() error {
	home, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	state := filepath.Join(home, "care-anywhere")
	if err = os.MkdirAll(state, 0700); err != nil {
		return err
	}
	if c, err := readDesktopControl(state); err == nil {
		if _, err = controlCall(c, "GET", "/status", nil, 2*time.Second); err == nil {
			return openURL(c.URL + "/#" + c.Token)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(filepath.Join(state, "desktop.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	cmd := exec.Command(exe, "desktop", "--state", state)
	cmd.Stdout, cmd.Stderr = output, output
	if err = cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func readDesktopControl(state string) (control, error) {
	var c control
	b, err := os.ReadFile(filepath.Join(state, "desktop.json"))
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	return c, err
}

func controlCall(c control, method, path string, body []byte, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequest(method, c.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(b)))
	}
	return b, nil
}

func desktop(state, bundle string) error {
	if runtime.GOOS == "windows" {
		return errors.New("Windows desktop is not supported yet")
	}
	var err error
	if bundle == "" {
		bundle, err = defaultBundle()
		if err != nil {
			return err
		}
	}
	if err = os.MkdirAll(filepath.Join(state, "desktop"), 0700); err != nil {
		return err
	}
	unlock, err := lockState(filepath.Join(state, "desktop"))
	if err != nil {
		if c, e := readDesktopControl(state); e == nil {
			return openURL(c.URL + "/#" + c.Token)
		}
		return err
	}
	defer unlock()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	c := control{URL: "http://" + listener.Addr().String(), Token: randomToken()}
	if err = saveJSON(filepath.Join(state, "desktop.json"), c); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(state, "desktop.json"))
	var mu sync.Mutex
	var child *exec.Cmd
	var childDone chan struct{}
	phase, detail := "stopped", "Your clinic is stopped. Your records are saved on this computer."
	var operation sync.Mutex
	start := func() error {
		mu.Lock()
		defer mu.Unlock()
		if child != nil {
			return errors.New("CARE is already starting or running")
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		output, e := os.OpenFile(filepath.Join(state, "launcher.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if e != nil {
			return e
		}
		cmd := exec.Command(exe, "serve", "--state", state, "--bundle", bundle, "--no-open")
		cmd.Stdout, cmd.Stderr = output, output
		if e = cmd.Start(); e != nil {
			output.Close()
			return e
		}
		child, childDone = cmd, make(chan struct{})
		done := childDone
		phase, detail = "starting", "Checking the appliance and preparing your clinic. First launch can take a few minutes."
		go func() {
			e := cmd.Wait()
			output.Close()
			mu.Lock()
			defer mu.Unlock()
			child = nil
			if e != nil {
				phase, detail = "error", friendlyStartError(state)
			} else {
				phase, detail = "stopped", "Your clinic is stopped. Your records are saved on this computer."
			}
			close(done)
		}()
		return nil
	}
	stop := func() error {
		mu.Lock()
		running, done := child != nil, childDone
		mu.Unlock()
		if !running {
			return nil
		}
		cc, e := readControl(state)
		if e != nil {
			return errors.New("CARE is still preparing. Wait for startup to finish before stopping")
		}
		if _, e = controlCall(cc, "POST", "/control/stop", nil, 160*time.Second); e != nil {
			return e
		}
		select {
		case <-done:
			return nil
		case <-time.After(180 * time.Second):
			return errors.New("CARE is still shutting down; please wait")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(home, "CARE Anywhere Backups")
	web := filepath.Join(filepath.Dir(bundle), "web")
	if override := os.Getenv("CARE_DESKTOP_WEB"); override != "" {
		web = override
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		return fmt.Errorf("desktop UI is missing; reinstall the complete CARE Anywhere app: %w", err)
	}
	webHandler := http.FileServer(http.Dir(web))
	listBackups := func() ([]map[string]any, error) {
		items := []map[string]any{}
		entries, err := os.ReadDir(backupDir)
		if os.IsNotExist(err) {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".age") {
				continue
			}
			info, e := entry.Info()
			if e != nil {
				return nil, e
			}
			items = append(items, map[string]any{"db_dump": entry.Name(), "files_archive": "", "label": strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "care-"), ".age"), "manual": true, "encrypted": true, "size_bytes": info.Size()})
		}
		return items, nil
	}
	quit := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Host != listener.Addr().String() {
			http.Error(w, "Invalid host", 403)
			return
		}
		if (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/")) && r.Method == "GET" {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
			webHandler.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+c.Token {
			http.Error(w, "Reopen CARE Anywhere from Applications to reconnect.", 401)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != c.URL {
			http.Error(w, "Invalid origin", 403)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/status" {
			mu.Lock()
			p, d, running := phase, detail, child != nil
			mu.Unlock()
			_, setupErr := os.Stat(filepath.Join(state, "desktop-configured"))
			s := map[string]any{"phase": p, "detail": d, "healthy": false, "configured": setupErr == nil, "backupDir": backupDir, "stateDir": state, "platform": runtime.GOOS}
			if running {
				if cc, e := readControl(state); e == nil {
					if b, e := controlCall(cc, "GET", "/control/status", nil, 4*time.Second); e == nil {
						var guest map[string]bool
						if json.Unmarshal(b, &guest) == nil {
							s["healthy"], s["configured"] = guest["healthy"], guest["configured"]
							if guest["healthy"] {
								s["phase"] = "ready"
								s["detail"] = "Your clinic is ready on this computer."
							}
						}
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(s)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/backups" {
			items, err := listBackups()
			if err != nil {
				http.Error(w, "Cannot read backup folder: "+err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(items)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/storage" {
			space, e := diskspace.Of(state)
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			level := "ok"
			if space.Free < 2<<30 {
				level = "low"
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"checked_at": time.Now().Unix(), "level": level, "headline": "Clinic storage", "drives": []any{map[string]any{"id": "vm", "label": "Clinic data", "path": state, "free": space.Free, "total": space.Total, "level": level, "message": "8 GiB clinic data disk", "cleanable": false}}, "backup": map[string]any{"dir": backupDir, "free": space.Free, "total": space.Total, "need": 8 << 30, "set_bytes": 8 << 30, "days_left": 0, "shares_docker_drive": true, "level": level, "message": "Whole-disk encrypted backups"}, "last_run": map[string]any{"state": "", "at": 0, "reason": "", "message": ""}, "newest_backup_at": 0, "stale": false})
			return
		}
		if r.Method == "GET" && r.URL.Path == "/logs" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			for _, name := range []string{"launcher.log", "console.log"} {
				fmt.Fprintln(w, "\n--- "+name+" ---")
				if f, e := os.Open(filepath.Join(state, name)); e == nil {
					if info, e := f.Stat(); e == nil && info.Size() > 32768 {
						f.Seek(-32768, io.SeekEnd)
					}
					io.Copy(w, io.LimitReader(f, 32768))
					f.Close()
				}
			}
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if !operation.TryLock() {
			http.Error(w, "Another operation is in progress. Please wait.", 409)
			return
		}
		defer operation.Unlock()
		var e error
		switch r.URL.Path {
		case "/start":
			e = start()
		case "/stop":
			e = stop()
		case "/quit":
			e = stop()
			if e == nil {
				select {
				case quit <- struct{}{}:
				default:
				}
			}
		case "/setup":
			var b []byte
			b, e = io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
			if e == nil {
				var cc control
				cc, e = readControl(state)
				if e == nil {
					var statusBytes []byte
					statusBytes, e = controlCall(cc, "GET", "/control/status", nil, 5*time.Second)
					var guest struct {
						Configured bool `json:"configured"`
					}
					if e == nil {
						e = json.Unmarshal(statusBytes, &guest)
					}
					if e == nil && !guest.Configured {
						_, e = controlCall(cc, "POST", "/control/setup", b, 10*time.Minute)
					}
					if e == nil {
						var credentials map[string]string
						if e = json.Unmarshal(b, &credentials); e == nil {
							e = registerOnboarding(credentials)
						}
					}
					if e == nil {
						e = atomicWrite(filepath.Join(state, "desktop-configured"), []byte("1\n"), 0600)
					}
				}
			}
		case "/reset-password":
			var b []byte
			b, e = io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
			if e == nil {
				var cc control
				cc, e = readControl(state)
				if e == nil {
					_, e = controlCall(cc, "POST", "/control/reset-password", b, 2*time.Minute)
				}
			}
		case "/backup":
			mu.Lock()
			running := child != nil
			mu.Unlock()
			if running {
				e = errors.New("Stop CARE before making a backup")
			} else if e = os.MkdirAll(backupDir, 0700); e == nil {
				e = backup(state, filepath.Join(backupDir, "care-"+time.Now().Format("20060102-150405")+".age"))
			}
		case "/backups-folder":
			if e = os.MkdirAll(backupDir, 0700); e == nil {
				e = openURL(backupDir)
			}
		default:
			http.NotFound(w, r)
			return
		}
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		io.WriteString(w, "Done")
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	defer server.Close()
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()
	if err = openURL(c.URL + "/#" + c.Token); err != nil {
		fmt.Fprintln(os.Stderr, "Open CARE using the private desktop.json access link:", err)
	}
	if err = start(); err != nil {
		mu.Lock()
		phase, detail = "error", err.Error()
		mu.Unlock()
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-quit:
	case <-signals:
		operation.Lock()
		defer operation.Unlock()
		if err = stop(); err != nil {
			return err
		}
	case err = <-errs:
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func registerOnboarding(credentials map[string]string) error {
	// Use CARE's own authentication and plugin API, as CARE Clinic does.
	client := &http.Client{Timeout: 30 * time.Second}
	post := func(path string, body any, token string) ([]byte, error) {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		req, e := http.NewRequest("POST", "http://127.0.0.1:8484"+path, bytes.NewReader(b))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		result, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if e != nil {
			return nil, e
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("CARE onboarding registration failed (%s)", resp.Status)
		}
		return result, nil
	}
	b, e := post("/api/v1/auth/login/", credentials, "")
	if e != nil {
		return e
	}
	var tokens struct {
		Access string `json:"access"`
	}
	if e = json.Unmarshal(b, &tokens); e != nil {
		return e
	}
	if tokens.Access == "" {
		return errors.New("CARE login did not return an access token")
	}
	_, e = post("/api/v1/plug_config/", map[string]any{"slug": "care_onboarding_fe", "meta": map[string]any{"url": "http://127.0.0.1:8484/onboarding/assets/remoteEntry.js", "config": map[string]any{"redirect_after_login": true}}}, tokens.Access)
	return e
}
