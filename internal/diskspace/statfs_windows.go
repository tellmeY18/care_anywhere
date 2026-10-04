package diskspace

import "golang.org/x/sys/windows"

func statVolume(path string) (Usage, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return Usage{}, e
	}
	var free, total, available uint64
	e = windows.GetDiskFreeSpaceEx(p, &available, &total, &free)
	return Usage{Free: available, Total: total}, e
}
