//go:build !windows && !linux && !darwin

package sampling

import "errors"

func diskSpace(dir string) (free, total uint64, err error) {
	return 0, 0, errors.New("free disk space unknown on this system")
}
