package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func doctor() error {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("enable hardware virtualization and grant your user access to /dev/kvm: %w", err)
	}
	return f.Close()
}

func prepareQEMU(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	return nil
}
