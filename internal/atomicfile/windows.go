//go:build windows

// Copyright (c) 2026 Open Healthcare Network Foundation. MIT.
package atomicfile

import "golang.org/x/sys/windows"

func replace(from, to string) error {
	f, e := windows.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	t, e := windows.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(f, t, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func restrictPrivateFile(path string) error {
	t, e := windows.OpenCurrentProcessToken()
	if e != nil {
		return e
	}
	defer t.Close()
	u, e := t.GetTokenUser()
	if e != nil {
		return e
	}
	d, e := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + u.User.Sid.String() + ")")
	if e != nil {
		return e
	}
	acl, _, e := d.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
