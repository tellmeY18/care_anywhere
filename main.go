package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type manifest struct {
	Version string            `json:"version"`
	Arch    string            `json:"arch"`
	Kernel  string            `json:"kernel"`
	Initrd  string            `json:"initrd"`
	System  string            `json:"system"`
	Files   map[string]string `json:"files"`
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "guest" {
		return guestMain()
	}
	if len(os.Args) < 2 {
		return errors.New("usage: care-anywhere serve|status|stop|backup|restore|doctor [options]")
	}
	home, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	state := fs.String("state", filepath.Join(home, "care-anywhere"), "clinic state directory")
	bundle := fs.String("bundle", "", "unpacked appliance bundle")
	port := fs.Int("port", 8484, "local control and clinic port")
	file := fs.String("file", "", "backup file")
	key := fs.String("key", "", "recovery identity file")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	*state, err = filepath.Abs(*state)
	if err != nil {
		return err
	}
	switch os.Args[1] {
	case "doctor":
		return doctor()
	case "serve":
		return serve(*state, *bundle, *port)
	case "backup":
		return backup(*state, *file)
	case "restore":
		return restore(*state, *file, *key)
	case "status", "stop", "logs":
		c, err := readControl(*state)
		if err != nil {
			return err
		}
		method := "GET"
		path := "/control/status"
		if os.Args[1] == "logs" {
			path = "/control/logs"
		}
		if os.Args[1] == "stop" {
			method = "POST"
			path = "/control/stop"
		}
		req, _ := http.NewRequest(method, c.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+c.Token)
		client := http.Client{Timeout: 150 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("%s", resp.Status)
		}
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	default:
		return errors.New("unknown command")
	}
}

type control struct{ URL, Token string }

func readControl(state string) (control, error) {
	var c control
	b, e := os.ReadFile(filepath.Join(state, "control.json"))
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	return c, e
}
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func saveJSON(path string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return atomicWrite(path, b, 0600)
}

func serve(state, bundle string, port int) error {
	if bundle == "" {
		return errors.New("--bundle is required")
	}
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return err
	}
	m, err := verifyBundle(bundle)
	if err != nil {
		return err
	}
	if m.Arch != runtime.GOARCH {
		return fmt.Errorf("bundle architecture %s does not match %s", m.Arch, runtime.GOARCH)
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		return err
	}
	unlock, err := lockState(state)
	if err != nil {
		return err
	}
	defer unlock()
	if err = ensureData(state, bundle, m); err != nil {
		return err
	}
	var release manifest
	b, err := os.ReadFile(filepath.Join(state, "release.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &release); err != nil {
		return err
	}
	if release.Version != m.Version {
		return errors.New("this data belongs to a different release; in-place upgrades are not implemented")
	}
	machine, err := bootVM(bundle, state, m)
	if err != nil {
		return err
	}
	defer machine.Close()
	transport := &http.Transport{DialContext: machine.Dial, ResponseHeaderTimeout: 15 * time.Minute}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Minute}
	target, _ := url.Parse("http://guest")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	// Guest agent management endpoints must never be forwarded without host auth.
	token := randomToken()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	c := control{URL: "http://" + listener.Addr().String(), Token: token}
	if err = saveJSON(filepath.Join(state, "control.json"), c); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(state, "control.json"))
	defer os.Remove(filepath.Join(state, "open.html"))
	done := make(chan struct{})
	var stop sync.Once
	var mutation sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/control/", func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "invalid host", 403)
			return
		}
		if r.URL.Path == "/control/" && r.Method == "GET" {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-ancestors 'none'")
			io.WriteString(w, controlHTML)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", 401)
			return
		}
		switch r.URL.Path {

		case "/control/logs":
			if r.Method != "GET" {
				http.Error(w, "method", 405)
				return
			}
			resp, e := client.Get("http://guest/logs")
			if e != nil {
				http.Error(w, e.Error(), 502)
				return
			}
			defer resp.Body.Close()
			io.Copy(w, resp.Body)
		case "/control/status":
			if r.Method != "GET" {
				http.Error(w, "method", 405)
				return
			}
			resp, e := client.Get("http://guest/status")
			if e != nil {
				http.Error(w, "Guest starting or unavailable", 503)
				return
			}
			defer resp.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		case "/control/setup":
			if r.Method != "POST" {
				http.Error(w, "method", 405)
				return
			}
			if !mutation.TryLock() {
				http.Error(w, "busy", 409)
				return
			}
			defer mutation.Unlock()
			req, _ := http.NewRequest("POST", "http://guest/setup", http.MaxBytesReader(w, r.Body, 8192))
			resp, e := client.Do(req)
			if e != nil {
				http.Error(w, e.Error(), 502)
				return
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		case "/control/stop":
			if r.Method != "POST" {
				http.Error(w, "method", 405)
				return
			}
			if !mutation.TryLock() {
				http.Error(w, "busy", 409)
				return
			}
			defer mutation.Unlock()
			req, _ := http.NewRequest("POST", "http://guest/stop", nil)
			resp, e := client.Do(req)
			if e != nil {
				http.Error(w, e.Error(), 502)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				http.Error(w, "guest stop failed", 502)
				return
			}
			io.WriteString(w, "Stopped\n")
			stop.Do(func() { close(done) })
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "invalid host", 403)
			return
		}
		p := path.Clean(r.URL.Path)
		if p == "/setup" || p == "/stop" || p == "/status" || p == "/logs" {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	ui := c.URL + "/control/#" + token
	log.Printf("Control panel: %s/control/ (access link saved privately in %s)", c.URL, state)
	if err = atomicWrite(filepath.Join(state, "open.html"), []byte("<meta http-equiv=\"refresh\" content=\"0;url="+ui+"\">"), 0600); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", ui).Run()
	case "linux":
		_ = exec.Command("xdg-open", ui).Run()
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		req, _ := http.NewRequest("POST", "http://guest/stop", nil)
		resp, e := client.Do(req)
		if e != nil {
			return e
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New("guest refused shutdown")
		}
		server.Close()
		return machine.Close()
	case <-done:
		time.Sleep(time.Second)
		server.Close()
		return nil
	case e := <-errCh:
		return e
	case <-machine.Done():
		server.Close()
		return errors.New("VM exited; see console.log")
	}
}

var controlHTML = `<!doctype html><html><meta charset="utf-8"><title>CARE Anywhere</title><style>body{font:18px system-ui;max-width:720px;margin:60px auto;padding:20px;background:#f0fdf4;color:#12372a}button,input{font:inherit;padding:12px;margin:8px 0}button{background:#047857;color:white;border:0;border-radius:8px;cursor:pointer}label{display:block}pre{white-space:pre-wrap}a{color:#047857}</style><h1>CARE Anywhere</h1><p>Your clinic, on this computer.</p><pre id="status">Connecting…</pre><form id="setup"><label>Administrator username <input name="username" required autocomplete="username"></label><label>Password <input name="password" type="password" required minlength="12" autocomplete="new-password"></label><button>Create clinic</button></form><p><a href="/" target="_blank" rel="noopener">Open CARE</a></p><button id="stop">Stop clinic</button><p>Encrypted backup and restore are available through the command line in this preview.</p><script>let token=location.hash.slice(1)||sessionStorage.getItem('token');if(token){sessionStorage.setItem('token',token);history.replaceState(null,'',location.pathname)}async function call(path,method='GET',body){let r=await fetch('/control/'+path,{method,headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body&&JSON.stringify(body)});let text=await r.text();if(!r.ok)throw Error(text);return text}async function status(){try{let s=JSON.parse(await call('status'));document.querySelector('#status').textContent=JSON.stringify(s,null,2);document.querySelector('#setup').hidden=s.configured}catch(e){document.querySelector('#status').textContent=e.message}}document.querySelector('#setup').onsubmit=async e=>{e.preventDefault();let b=e.target.querySelector('button');b.disabled=true;try{await call('setup','POST',Object.fromEntries(new FormData(e.target)));e.target.reset();await status()}catch(e){alert(e.message)}finally{b.disabled=false}};document.querySelector('#stop').onclick=async()=>{try{await call('stop','POST');document.querySelector('#status').textContent='Stopped'}catch(e){alert(e.message)}};status();setInterval(status,5000)</script></html>`

// Reject user strings before writing them to an EnvironmentFile.
func singleLine(s string) bool { return !strings.ContainsAny(s, "\r\n\x00") }
