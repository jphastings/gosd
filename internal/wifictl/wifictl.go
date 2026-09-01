// Package wifictl defines the request/status file protocol a runtime
// wifi.Join call uses to ask gosd-init to join a new WiFi network, and reads
// both files back. It is the shared contract between the public wifi
// package (the writer of request.json) and gosd-init's wifiup reconciler
// (the writer of status.json) — one package so the two sides cannot drift
// (epic gosd-ojbm, decision 1).
//
// # Why a file, not a socket
//
// gosd-init has no listener beyond mDNS ("mDNS is the only network listener
// in gosd-init" is a fleet-wide locked decision), so a runtime request
// travels the same way a declared fault does (see internal/faultdrop): the
// app writes a file to /run/gosd, a tmpfs directory gosd-init already
// mounts, and gosd-init polls for it.
//
// # File format
//
// [Dir] is /run/gosd/wifi. request.json holds the network wifi.Join wants
// joined:
//
//	{"id": "3f2a...", "ssid": "home-network", "passphrase": "...", "persist": true}
//
// status.json holds the outcome gosd-init found, keyed by the same id so a
// caller polling it can tell a fresh answer from a stale one:
//
//	{"id": "3f2a...", "state": "joined"}
//	{"id": "3f2a...", "state": "failed", "error": "4-way handshake timed out"}
//
// There is only ever one request in flight: a new request.json replaces the
// old one atomically, and the reconciler abandons whatever it was doing for
// the previous id (epic decision 6, last-write-wins).
//
// Both files are written to a temp file in the same directory and renamed
// into place, so a reader never observes a half-written file — /run is
// tmpfs, so this buys atomicity against a torn read, not crash durability;
// there is no fsync here and there should not be, the same reasoning as
// internal/faultdrop and internal/secretreg. Both are mode 0600 in a 0700
// directory: request.json can carry a plaintext passphrase, and nothing
// else on the device has a reason to read either file.
//
// # What a reader must assume
//
// [ReadRequest] and [ReadStatus] distinguish "no file" (ok=false, err=nil —
// the normal case before anything has asked to join, or after gosd-init has
// nothing to report yet) from "a file is there but isn't trustworthy"
// (err!=nil). Unlike internal/faultdrop and internal/secretreg, this
// package leaves the unparseable case to its caller rather than silently
// dropping it: the public wifi package and gosd-init's reconciler have
// different, legitimate policies for a file that doesn't parse (poll again;
// self-heal and report failure), so neither is baked in here.
package wifictl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Dir is the tmpfs directory the request/status protocol lives in. Callers
// on a real device use this; tests point at a temp directory instead so the
// protocol can be exercised without touching /run.
const Dir = "/run/gosd/wifi"

// RequestFile and StatusFile are the two protocol files' names within Dir
// (or whatever directory a caller substitutes for it).
const (
	RequestFile = "request.json"
	StatusFile  = "status.json"
)

// State is where gosd-init has got to with one join request. Joined and
// Failed are terminal — see [State.Terminal].
type State string

const (
	Joining State = "joining"
	Joined  State = "joined"
	Failed  State = "failed"
)

// Terminal reports whether s is an outcome a caller can stop polling for.
func (s State) Terminal() bool {
	return s == Joined || s == Failed
}

// Request is what a runtime wifi.Join call asks gosd-init to do.
type Request struct {
	// ID identifies this request so its eventual Status can be matched
	// back to it, even if a later request has since replaced it.
	ID string `json:"id"`
	// SSID is the network to join.
	SSID string `json:"ssid"`
	// Passphrase authenticates to SSID under WPA2-PSK. Empty means an
	// open network — the fleet's WiFi scope allows both, nothing else.
	Passphrase string `json:"passphrase,omitempty"`
	// Persist asks gosd-init to write SSID/Passphrase into the card's
	// config tree once the join succeeds, so the next boot rejoins.
	Persist bool `json:"persist,omitempty"`
}

// Status is the outcome gosd-init reports for the request carrying the same
// ID.
type Status struct {
	// ID matches the Request this status answers.
	ID string `json:"id"`
	// State is where the join has got to.
	State State `json:"state"`
	// Error is the failure reason, verbatim from nl80211 where possible,
	// set only when State is Failed.
	Error string `json:"error,omitempty"`
}

// WriteRequest atomically writes req to dir/RequestFile, replacing whatever
// request was there before.
func WriteRequest(dir string, req Request) error {
	return writeJSON(filepath.Join(dir, RequestFile), req)
}

// WriteStatus atomically writes status to dir/StatusFile, replacing
// whatever status was there before.
func WriteStatus(dir string, status Status) error {
	return writeJSON(filepath.Join(dir, StatusFile), status)
}

// ReadRequest reads dir/RequestFile. ok is false with a nil error when no
// request has been written; err is non-nil when a file is there but is not
// valid JSON for a [Request] — the caller decides what to do with an
// untrustworthy file, this function only reports that it is one.
func ReadRequest(dir string) (req Request, ok bool, err error) {
	return readJSON[Request](filepath.Join(dir, RequestFile))
}

// ReadStatus reads dir/StatusFile. ok is false with a nil error when no
// status has been written; err is non-nil when a file is there but is not
// valid JSON for a [Status].
func ReadStatus(dir string) (status Status, ok bool, err error) {
	return readJSON[Status](filepath.Join(dir, StatusFile))
}

// writeJSON is the atomic-write helper shared by WriteRequest and
// WriteStatus: encode, write to a temp file beside the target, rename into
// place. The directory is created 0700 and the file 0600 — request.json can
// carry a plaintext passphrase, and nothing else on the device has a reason
// to read either file.
func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// readJSON reads and decodes path into a T, distinguishing "not there" from
// "there but not this shape" the way the package doc promises.
func readJSON[T any](path string) (v T, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}

	if err := json.Unmarshal(data, &v); err != nil {
		return v, false, fmt.Errorf("%s does not hold valid JSON: %w", path, err)
	}
	return v, true, nil
}
