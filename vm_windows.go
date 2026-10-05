package main

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Kept open until process exit: all VM children die before the state lock can
// be reused after a launcher crash. Assign before spawning to avoid a race.
var vmJob windows.Handle

func doctor() error {
	dll := windows.NewLazySystemDLL("WinHvPlatform.dll")
	if err := dll.Load(); err != nil {
		return fmt.Errorf("enable Windows Hypervisor Platform and restart: %w", err)
	}
	var present uint32
	r, _, _ := dll.NewProc("WHvGetCapability").Call(0, uintptr(unsafe.Pointer(&present)), unsafe.Sizeof(present), 0)
	if r != 0 || present == 0 {
		return fmt.Errorf("enable Windows Hypervisor Platform in Windows Features and hardware virtualization in firmware, then restart")
	}
	return nil
}

func prepareQEMU(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	if err = windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(job)
		return err
	}
	vmJob = job
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return nil
}
