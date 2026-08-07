//go:build !linux

package main

import "errors"

// filesystemSizeBytes is the off-Linux stub; diskstorage only runs
// meaningfully on a GoSD board. This keeps the example building (and its
// logic testable) on a developer's machine.
func filesystemSizeBytes(string) (int64, error) {
	return 0, errors.New("filesystem size reporting is only supported on Linux boards")
}
