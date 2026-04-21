//go:build windows

package app

import "errors"

// Windows does not expose syscall.Statfs, so skip the preflight space check.
func getFreeSpace(path string) (uint64, error) {
	return 0, errors.New("free space check not supported on windows")
}
