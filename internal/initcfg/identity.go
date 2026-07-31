package initcfg

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

// PayloadFile is one (path, content) entry hashed into a build's image
// identity — see ComputeIdentity.
type PayloadFile struct {
	Path    string
	Content []byte
}

// ComputeIdentity derives gosd build's content-derived image identity (see
// Config.Identity): a SHA-256 digest over every entry of files, absorbed in
// a fixed order (sorted by Path) regardless of the order files were built
// in. That's what keeps identical rebuilds' identity equal even though a
// build gathers its inputs from Go maps, whose iteration order is
// randomized — including across the qemu CI path (docs/design/
// upgrade-path.md §4, which requires this).
//
// # Recipe
//
// The digest covers the boot payload set: every file gosd build writes to
// the GOSD-BOOT FAT partition, whether it lands directly at that
// partition's root (the kernel image, DTB(s), the board's own boot-config
// file — config.txt/cmdline.txt or extlinux.conf — any USB-gadget overlay,
// and the rendered gosd.toml template) or gets packed into the initramfs
// archive also shipped there (/init, /app, and everything under
// /lib/firmware) — with one deliberate exception: config.json itself.
//
// config.json is excluded entirely, not merely its Identity field. It's
// baked into that same initramfs, so its final bytes (Identity included)
// can't be known until Identity is computed — hashing it would make the
// digest depend on its own output. Excluding the whole file, not just the
// one field, keeps the recipe simple to state and to re-derive, at the
// cost of Identity being blind to whatever config.json carries that
// appears nowhere else in the payload. In practice that's just
// Config.DataExpand: Board/Hostname/Wifi/Env are also baked into
// config.json, but they're baked into the rendered gosd.toml template too
// (a real, hashed FAT-root file — see pipeline.Assemble), so changing
// --hostname/--wifi-ssid/--wifi-pass/--env still moves Identity via
// gosd.toml even though config.json's own copies of those values are
// invisible to it. That's an acceptable trade for what Identity is for:
// telling boot *payload* builds apart, not per-device provisioning —
// provisioning drift is §3's concern, not §4's.
//
// For each file, in Path order, the digest absorbs an 8-byte
// big-endian-length-prefixed Path followed by an 8-byte
// big-endian-length-prefixed Content; the length prefixes stop adjacent
// fields from being ambiguous (e.g. Path "ab"+Content "c" hashing
// differently than Path "a"+Content "bc").
//
// Callers key a FAT-root file by its literal boot-partition path (e.g.
// "kernel8.img" or "gosd.toml" — no leading slash: that's the convention
// every board's BootFiles already uses) and an initramfs member by its
// in-archive path passed through InitramfsPayloadPath (e.g.
// "initramfs:/init"). The "initramfs:" prefix — disjoint from every
// FAT-root path, which never starts with "/" — keeps the two namespaces
// from ever colliding, so a future reader re-deriving Identity from a
// built image alone (list the GOSD-BOOT root, drop config.json, unpack
// initramfs.cpio.zst and list its members minus /etc/gosd/config.json) can
// reproduce the exact input set with nothing beyond what's on the card.
func ComputeIdentity(files []PayloadFile) string {
	sorted := make([]PayloadFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	h := sha256.New()
	var lenBuf [8]byte
	for _, f := range sorted {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(f.Path)))
		h.Write(lenBuf[:])
		h.Write([]byte(f.Path))
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(f.Content)))
		h.Write(lenBuf[:])
		h.Write(f.Content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// InitramfsPayloadPath returns the ComputeIdentity Path key for a file at
// archivePath (e.g. "/init") inside the initramfs archive — see
// ComputeIdentity's recipe.
func InitramfsPayloadPath(archivePath string) string {
	return "initramfs:" + archivePath
}
