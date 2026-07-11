package kernelbuild

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// defaultBuildRoot returns the default directory for kernel-build staging and
// cache entries. This deliberately is NOT os.UserCacheDir(): macOS may purge
// ~/Library/Caches under storage pressure at any time, and doing so mid-build
// yanks the live container bind mounts out from under a 20-75 minute build
// (and evicts entries that cost that long to rebuild). See bean gosd-l4y9.
//
// The chosen locations are durable state dirs that still live under the
// user's home, which Docker Desktop, colima and podman machine all share
// with their VMs:
//   - darwin:  ~/Library/Application Support/gosd/kernel-build
//   - windows: os.UserConfigDir()/gosd/kernel-build
//   - other:   $XDG_STATE_HOME/gosd/kernel-build, or
//     ~/.local/state/gosd/kernel-build
func defaultBuildRoot() (string, error) {
	return buildRootFor(runtime.GOOS, os.Getenv)
}

func buildRootFor(goos string, getenv func(string) string) (string, error) {
	switch goos {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "gosd", "kernel-build"), nil
	case "windows":
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "gosd", "kernel-build"), nil
	default:
		if state := getenv("XDG_STATE_HOME"); state != "" {
			return filepath.Join(state, "gosd", "kernel-build"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", "gosd", "kernel-build"), nil
	}
}

// vanishedStagingError explains a staging directory that disappeared on the
// host while (or right after) the container build ran. runErr may be nil when
// the build itself reported success but the outputs are gone.
func vanishedStagingError(stagingDir string, runErr error) error {
	msg := "the kernel-build staging directory " + stagingDir + " disappeared while the build was running" +
		" - a cache-cleaning tool or the OS (macOS purges under storage pressure) likely removed it;" +
		" free up disk space, close cache cleaners, and re-run"
	if runErr != nil {
		return errors.Join(errors.New(msg), runErr)
	}
	return errors.New(msg)
}
