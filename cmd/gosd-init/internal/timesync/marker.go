package timesync

import (
	"fmt"
	"os"
	"path"
)

// DefaultTimeSyncedPath is the marker file gosd-init creates once the
// system clock has been synchronized for the first time. App authors
// should gate TLS calls (or just retry past failures) on this file's
// existence, since certificate validity checks fail against an unsynced
// clock that still reads somewhere near the epoch.
const DefaultTimeSyncedPath = "/run/gosd/time-synced"

// WriteTimeSynced creates the (empty) marker file at markerPath, and the
// directory it lives in, mirroring netup.MarkNetworkUp's shape exactly
// (same rationale: an existence check is all any caller needs).
func WriteTimeSynced(markerPath string) error {
	if dir := path.Dir(markerPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(markerPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("creating %s: %w", markerPath, err)
	}
	return f.Close()
}

// NetworkUpMarkerExists reports whether the network-up marker file
// (netup.DefaultNetworkUpPath in production) exists at markerPath. Passed
// path rather than a hardcoded import of netup's constant so this package
// has no dependency on the networking packages at all — main.go's wiring
// is what ties the two paths together.
func NetworkUpMarkerExists(markerPath string) (bool, error) {
	_, err := os.Stat(markerPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking %s: %w", markerPath, err)
}
