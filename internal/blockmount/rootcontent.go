package blockmount

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/diskfmt"
)

// ext4ReservedRootEntries are the root-directory entries diskfmt's golden
// ext4 image (internal/diskfmt/ext4golden) itself ships with — present on
// every freshly Format-ed volume before an app has ever written anything, so
// their presence alone is not evidence of real content. What is in them can
// be: RootHasOtherContent counts a non-empty one as content. mke2fs creates
// lost+found unconditionally (it is not one of the -O feature toggles the
// golden's build pins, see build/ext4-golden/Dockerfile); nothing else is
// pre-populated.
//
// FAT32 and exFAT have no equivalent: a freshly formatted volume of either
// lists nothing at all. go-diskfs zeroes FAT32's whole root-directory cluster
// before writing the volume label into it, exFAT's formatter writes a root
// directory holding only the label, allocation-bitmap and up-case entries,
// and the kernel's readdir for both filesystems emits only real file entries
// — never the volume label or exFAT's metadata entries. If that were ever
// untrue, the effect would be to make a fresh volume look used, which
// adopts rather than reformats: the safe direction (see establish's doc).
func reservedRootEntries(fs diskfmt.FS) map[string]bool {
	if fs == diskfmt.EXT4 {
		return ext4ReservedRootEntries
	}
	return nil
}

var ext4ReservedRootEntries = map[string]bool{"lost+found": true}

// RootHasOtherContent reports whether the filesystem of kind fs mounted at
// mountpoint holds anything in its root directory beyond what a fresh format
// of that filesystem ships with (see reservedRootEntries) and
// EstablishedMarker itself — see Deps.RootHasOtherContent's doc for why
// establish needs this second opinion before ever reformatting, or adopting,
// a no-marker volume.
func RootHasOtherContent(mountpoint string, fs diskfmt.FS) (bool, error) {
	entries, err := os.ReadDir(mountpoint)
	if err != nil {
		return false, fmt.Errorf("reading the root directory of %s failed: %w", mountpoint, err)
	}
	reserved := reservedRootEntries(fs)
	for _, e := range entries {
		if e.Name() == EstablishedMarker {
			continue
		}
		if reserved[e.Name()] {
			// fsck moves orphaned files into lost+found, so one with
			// anything in it is a volume's data, not a fresh format's.
			inside, err := os.ReadDir(filepath.Join(mountpoint, e.Name()))
			if err != nil {
				return false, fmt.Errorf("reading %s on %s failed: %w", e.Name(), mountpoint, err)
			}
			if len(inside) == 0 {
				continue
			}
		}
		return true, nil
	}
	return false, nil
}
