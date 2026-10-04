package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type fireVM struct {
	cmd    *exec.Cmd
	done   chan struct{}
	socket string
	log    *os.File
}

func doctor() error {
	f, e := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if e != nil {
		return fmt.Errorf("CARE needs hardware virtualization (KVM). Ask your administrator to enable virtualization and grant your user access to /dev/kvm, then log out and back in: %w", e)
	}
	f.Close()
	fmt.Println("Linux: KVM accessible.")
	return nil
}
func bootVM(bundle, state string, m manifest) (machine, error) {
	if _, ok := m.Files["firecracker"]; !ok {
		return nil, fmt.Errorf("this bundle does not include a verified Firecracker runtime; install the Linux alpha package")
	}
	if e := doctor(); e != nil {
		return nil, e
	}
	socket := filepath.Join(state, "vsock.sock")
	os.Remove(socket)
	api := filepath.Join(state, "firecracker.sock")
	os.Remove(api)
	cfg := map[string]any{
		"boot-source":    map[string]any{"kernel_image_path": filepath.Join(bundle, m.Kernel), "initrd_path": filepath.Join(bundle, m.Initrd), "boot_args": "console=ttyS0 reboot=k panic=1 care.shutdown=reboot init=" + m.System + "/init"},
		"machine-config": map[string]any{"vcpu_count": 2, "mem_size_mib": 4096},
		"drives":         []any{map[string]any{"drive_id": "system", "path_on_host": filepath.Join(bundle, "system.img"), "is_root_device": false, "is_read_only": true}, map[string]any{"drive_id": "data", "path_on_host": filepath.Join(state, "data.img"), "is_root_device": false, "is_read_only": false}},
		"vsock":          map[string]any{"guest_cid": 3, "uds_path": socket},
	}
	b, e := json.Marshal(cfg)
	if e != nil {
		return nil, e
	}
	if e = atomicWrite(filepath.Join(state, "firecracker.json"), b, 0600); e != nil {
		return nil, e
	}
	output, e := os.OpenFile(filepath.Join(state, "console.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(filepath.Join(bundle, "firecracker"), "--api-sock", api, "--config-file", filepath.Join(state, "firecracker.json"))
	// A crashed launcher must not leave a disk-writing VM behind after its lock dies.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Stdout = output
	cmd.Stderr = output
	if e = cmd.Start(); e != nil {
		output.Close()
		return nil, e
	}
	v := &fireVM{cmd: cmd, done: make(chan struct{}), socket: socket, log: output}
	go func() { cmd.Wait(); close(v.done); output.Close() }()
	return v, nil
}
func (v *fireVM) Done() <-chan struct{} { return v.done }
func (v *fireVM) Dial(ctx context.Context, _, _ string) (net.Conn, error) {
	c, e := (&net.Dialer{}).DialContext(ctx, "unix", v.socket)
	if e != nil {
		return nil, e
	}
	c.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprint(c, "CONNECT 8080\n")
	line, e := bufio.NewReader(c).ReadString('\n')
	if e != nil || len(line) < 3 || line[:3] != "OK " {
		c.Close()
		return nil, fmt.Errorf("vsock handshake: %s %v", line, e)
	}
	c.SetDeadline(time.Time{})
	return c, nil
}
func (v *fireVM) Close() error {
	select {
	case <-v.done:
		return nil
	default:
	}
	transport := &http.Transport{DialContext: v.Dial}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := client.Post("http://guest/stop", "application/json", nil)
	if err == nil {
		resp.Body.Close()
	}
	select {
	case <-v.done:
		return nil
	case <-time.After(150 * time.Second):
		return fmt.Errorf("guest must be shut down through the control endpoint")
	}
}
