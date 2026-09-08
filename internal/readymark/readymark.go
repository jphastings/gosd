// Package readymark defines the empty marker file the public ready
// package's Signal function creates, and reads it back for gosd-init's boot
// sequence — the shared contract between the two, so the writer and the
// reader cannot drift (the internal/wifictl precedent).
//
// # Why a file, not a socket
//
// gosd-init has no listener beyond mDNS, so this travels the same way a
// declared fault does (see internal/faultdrop): the app writes a file to
// /run/gosd, a tmpfs directory gosd-init already mounts, and gosd-init
// polls for it.
//
// Unlike faultdrop or wifictl, the marker carries no content at all — its
// mere presence is the whole signal, so there is nothing to encode, parse,
// or truncate, and no torn-read to guard against: an empty file exists in
// its entirety the instant its directory entry is created.
package readymark

import (
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/faultdrop"
)

// Dir is the tmpfs directory the ready marker lives in, shared with the
// rest of gosd-init's runtime state.
const Dir = faultdrop.Dir

// Path is where ready.Signal creates the marker file and gosd-init polls
// for it.
const Path = Dir + "/ready"

// Mark creates the empty marker file at dir/"ready", creating dir first if
// it doesn't exist. Calling it more than once is harmless: a marker that's
// already there is left exactly as it was.
func Mark(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, filepath.Base(Path)), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// Marked reports whether the marker file exists at dir/"ready".
func Marked(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.Base(Path)))
	return err == nil
}
