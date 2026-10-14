//go:build linux || darwin

package sampling

import "golang.org/x/sys/unix"

// diskSpace is the free (for this user) and total bytes of the disk dir is on.
func diskSpace(dir string) (free, total uint64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), uint64(st.Blocks) * uint64(st.Bsize), nil
}
