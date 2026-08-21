// Package devreserve is how gosd-init tells the rest of the device which
// block devices belong to GoSD itself, so a library an app calls can refuse
// to hand one of them to a USB host (bean gosd-ix0r).
//
// Nothing else on a GoSD board can answer "is this the disk we booted
// from?". The image boots from an initramfs, so /proc/cmdline's root= names
// no block device, and sysfs marks no boot medium portably across SD, eMMC
// and virtio. gosd-init is the one component that knows, because it probed
// the candidates and mounted one — so it writes down what it found and
// everything else reads it here. Before this file existed,
// [github.com/jphastings/gosd/gadget.MassStorage] could only infer the boot
// disk from the mount table, which made refusing it a side effect of
// gosd-init happening to keep /boot mounted rather than a rule the package
// enforced.
//
// # File format
//
// [Path] is /run/gosd/reserved-devices.json — /run is tmpfs, mounted by
// gosd-init's early mounts, so the file is rebuilt from scratch on every
// boot and never outlives the kernel that wrote it. It is a JSON array of
// objects, one per reserved device:
//
//	[
//	  {"path": "/dev/mmcblk0p1", "role": "the boot partition this device started from"}
//	]
//
// "role" is prose the publisher writes and the reader only ever quotes back
// in an error, never interprets. That is what lets a future gosd-init
// reserve a device class this reader has never heard of — the config
// partition of bean gosd-onjv, say — and still have an already-compiled app
// refuse it with an explanation its owner can act on. The two ends of this
// handoff are NOT necessarily the same release: an app compiles the gadget
// package from whatever gosd version its own go.mod pins, while gosd-init
// is built by whichever gosd CLI ran the build. Named optional fields in a
// flat array keep that survivable in both directions; a future format that
// needs a different top-level shape must take a different filename rather
// than redefine this one.
//
// # What a reservation means
//
// An entry names one device node gosd-init itself depends on. A caller must
// refuse any backing store that would expose one: that device, or the whole
// disk a reserved partition sits on — see [Reservations.Exposes].
//
// The relation runs one way on purpose. A LUN over /dev/mmcblk0 contains
// the boot partition, so it is refused; a LUN over /dev/mmcblk0p2 (the data
// partition, which is the app's own storage) contains none of the reserved
// bytes and is allowed. That is what keeps the published list minimal:
// reserving partition 1 is what refuses the whole disk, and a device class
// added later becomes one more entry with no change to any reader.
//
// # What a reader must assume
//
// A missing file means "no reservations", not an error. An app's copy of
// the gadget package can be newer than the gosd-init that built its image,
// and refusing to work on an image whose gosd-init predates this file would
// break a device that is no less safe than it was before. Absence therefore
// degrades to whatever protection the caller already had, and callers
// document that it does.
//
// A file that IS present but cannot be read, is larger than [MaxBytes], or
// does not parse is the opposite case and must be reported rather than
// shrugged off: [Encode] cannot produce any of those, so it means something
// other than gosd-init wrote /run/gosd, and carrying on would mean not
// knowing what is reserved — exactly the accident this package exists to
// remove. That is a deliberate departure from the drop-it-wholesale
// discipline of [github.com/jphastings/gosd/internal/faultdrop] and
// [github.com/jphastings/gosd/internal/secretreg], where ignoring a bad
// file costs a crash report; here it would cost the refusal itself.
package devreserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Dir is the tmpfs directory the file lives in, shared with the rest of
// gosd-init's runtime state.
const Dir = "/run/gosd"

// Path is where gosd-init publishes its reserved devices and every reader
// looks for them.
const Path = Dir + "/reserved-devices.json"

// MaxBytes bounds the file, for the writer (which refuses to exceed it) and
// the reader (which refuses to trust anything larger). /run is tmpfs —
// memory, on a board that may have only 512MB — and the realistic file
// holds a single-figure number of short entries, so this is orders of
// magnitude of headroom rather than a limit anything approaches.
const MaxBytes = 8 * 1024

// MaxRoleBytes bounds one entry's role as rendered into an error message.
// A role is meant to be a noun phrase ("the boot partition this device
// started from"); anything longer is a bug in the publisher, and trimming
// it keeps a refusal readable on a serial console.
const MaxRoleBytes = 120

// Entry is one reserved device: the node gosd-init depends on, and the
// prose a refusal quotes to explain why it is off limits.
type Entry struct {
	Path string `json:"path"`
	Role string `json:"role,omitempty"`
}

// Describe names the device for an error message — its published role, or a
// neutral phrase when the publisher left one out (an older gosd-init, or a
// role this reader had to discard). A refusal must never be left
// unexplained just because the explanation was missing.
func (e Entry) Describe() string {
	if e.Role == "" {
		return "a device GoSD reserves for the board's own use"
	}
	return e.Role
}

// Reservations is the published set, in file order.
type Reservations []Entry

// Exposes reports the first reserved device a mass-storage LUN (or any
// other whole-volume share) backed by candidate would hand over: candidate
// itself, or a reserved partition of the disk candidate names. It returns
// that entry so the caller can name it, and what it is for, in the refusal.
func (r Reservations) Exposes(candidate string) (Entry, bool) {
	for _, e := range r {
		if Covers(candidate, e.Path) {
			return e, true
		}
	}
	return Entry{}, false
}

// Covers reports whether handing out every byte of whole would also hand
// out dev: they name the same device, or dev is a partition of the disk
// whole names. The containment is one-directional by design — see the
// package doc's "What a reservation means".
func Covers(whole, dev string) bool {
	if whole == "" || dev == "" {
		return false
	}
	whole, dev = path.Clean(whole), path.Clean(dev)
	return whole == dev || isPartitionOf(whole, dev)
}

// isPartitionOf reports whether child names a partition of the whole device
// parent, using Linux's device-partition naming convention: a parent whose
// name ends in a digit (nvme0n1, mmcblk0) numbers its partitions with a "p"
// separator (nvme0n1p1, mmcblk0p1) — otherwise a partition number would be
// indistinguishable from another whole device's name — while every other
// parent (sda, vda) appends the partition digit directly (sda1). Restricted
// to /dev paths: a disk-image file backing a LUN follows no such
// convention, so e.g. "/data/image.bin" and a same-directory
// "/data/image.bin2" must never be treated as device and partition.
func isPartitionOf(parent, child string) bool {
	if parent == "" || !strings.HasPrefix(parent, "/dev/") || !strings.HasPrefix(child, parent) {
		return false
	}
	suffix := strings.TrimPrefix(child, parent)
	if suffix == "" {
		return false
	}
	if parent[len(parent)-1] >= '0' && parent[len(parent)-1] <= '9' {
		rest, ok := strings.CutPrefix(suffix, "p")
		if !ok {
			return false
		}
		suffix = rest
	}
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Encode renders the whole reserved set as the file's exact bytes, and
// refuses anything [Parse] would not read back as the same set: an entry
// with no device path (which would reserve nothing at all), or an encoding
// larger than [MaxBytes]. Refusing is the point — silently writing a file a
// reader will reject is indistinguishable, from the reader's side, from
// gosd-init never having published anything.
func Encode(entries []Entry) ([]byte, error) {
	for _, e := range entries {
		if e.Path == "" {
			return nil, errors.New("a reserved device has no path: every entry must name the device node it reserves")
		}
	}
	if entries == nil {
		entries = []Entry{}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("%d reserved devices encode to %d bytes, more than the %d a reader will trust", len(entries), len(data), MaxBytes)
	}
	return data, nil
}

// Parse turns the file's bytes into reservations, reporting an error rather
// than a partial set for anything it cannot read as the documented shape —
// see the package doc for why a bad file here is reported instead of
// dropped. An entry with no path is skipped rather than rejected: it
// reserves nothing, so it cannot weaken the set the rest of the file
// describes.
func Parse(data []byte) (Reservations, error) {
	if len(data) == 0 {
		return nil, errors.New("the reserved-device list is empty")
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("the reserved-device list is %d bytes, more than the %d gosd-init ever writes", len(data), MaxBytes)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("the reserved-device list is not the JSON array of {path, role} objects gosd-init writes: %w", err)
	}

	reserved := make(Reservations, 0, len(entries))
	for _, e := range entries {
		if e.Path == "" {
			continue
		}
		reserved = append(reserved, Entry{Path: e.Path, Role: cleanRole(e.Role)})
	}
	return reserved, nil
}

// cleanRole makes a published role safe to quote into an error: it drops
// one that isn't valid UTF-8 or carries control characters (a refusal that
// rewrites a serial console with escape sequences is worse than one with no
// explanation — and [Entry.Describe] still explains it), and trims an
// over-long one on a rune boundary.
func cleanRole(role string) string {
	if !utf8.ValidString(role) || strings.ContainsFunc(role, unicode.IsControl) {
		return ""
	}
	if len(role) <= MaxRoleBytes {
		return role
	}
	cut := MaxRoleBytes
	for cut > 0 && !utf8.RuneStart(role[cut]) {
		cut--
	}
	return role[:cut]
}

// Read returns the reservations published at name. A file that isn't there
// is no reservations and no error — see the package doc; every other
// failure, including a file too large to be one of ours, is reported.
//
// The size is checked before the read, not after: gosd-init keeps the file
// under [MaxBytes], but this runs as PID 1's peer on a board with as little
// as 512MB, and reading an arbitrarily large file into memory just to
// reject it is the failure that avoids.
func Read(name string) (Reservations, error) {
	info, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxBytes {
		return nil, fmt.Errorf("%s is %d bytes, more than the %d gosd-init ever writes there", name, info.Size(), MaxBytes)
	}

	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	reserved, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return reserved, nil
}

// Write publishes entries at name so that a reader watching for that exact
// path sees either the whole previous file or the whole new one: write a
// .tmp beside it, then rename, which is atomic within a directory. There is
// no fsync in that sequence and there should not be, since the file lives
// on tmpfs — there is no durability to buy, and a power cut takes the whole
// filesystem with it either way.
//
// Mode 0644: unlike the secrets registration file beside it, this names
// device nodes rather than credentials, and a reader is any process on the
// device that needs to know what not to publish.
func Write(name string, entries []Entry) error {
	data, err := Encode(entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}

	tmp := name + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
