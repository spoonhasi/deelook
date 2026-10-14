package sampling

import "golang.org/x/sys/windows"

// diskSpace is the free (for this user) and total bytes of the disk dir is on.
func diskSpace(dir string) (free, total uint64, err error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, err
	}
	err = windows.GetDiskFreeSpaceEx(p, &free, &total, nil)
	return free, total, err
}
