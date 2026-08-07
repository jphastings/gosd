//go:build linux

package blockmount

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/internal/diskfmt"
)

const (
	sysBlockDir     = "/sys/block"
	procMounts      = "/proc/mounts"
	procFilesystems = "/proc/filesystems"
)

// mountEntry is one line of /proc/mounts: the device node and where it is
// mounted.
type mountEntry struct {
	source string
	target string
}

func parseMounts() ([]mountEntry, error) {
	raw, err := os.ReadFile(procMounts)
	if err != nil {
		return nil, fmt.Errorf("reading %s failed: %w", procMounts, err)
	}
	var entries []mountEntry
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		entries = append(entries, mountEntry{source: unescapeMount(fields[0]), target: unescapeMount(fields[1])})
	}
	return entries, nil
}

// unescapeMount reverses the octal escaping (\040 for space, etc.) the kernel
// applies to whitespace in /proc/mounts fields.
func unescapeMount(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(field)
}

// MountedAt reports whether something is mounted at mountpoint and, if so, the
// device node behind it.
func MountedAt(mountpoint string) (string, bool, error) {
	entries, err := parseMounts()
	if err != nil {
		return "", false, err
	}
	for _, e := range entries {
		if e.target == mountpoint {
			return e.source, true, nil
		}
	}
	return "", false, nil
}

// MountedSources returns the set of device nodes currently mounted, e.g.
// "/dev/mmcblk1p1".
func MountedSources() (map[string]bool, error) {
	entries, err := parseMounts()
	if err != nil {
		return nil, err
	}
	sources := make(map[string]bool, len(entries))
	for _, e := range entries {
		sources[e.source] = true
	}
	return sources, nil
}

// MountedTargets returns every currently mounted device node mapped to where
// it is mounted, e.g. "/dev/mmcblk1p1" -> "/storage" — the same /proc/mounts
// data as MountedSources, kept alongside its mountpoint so a caller can name
// it in an actionable error (see gadget.MassStorage.Create's mounted-device
// rejection).
func MountedTargets() (map[string]string, error) {
	entries, err := parseMounts()
	if err != nil {
		return nil, err
	}
	targets := make(map[string]string, len(entries))
	for _, e := range entries {
		targets[e.source] = e.target
	}
	return targets, nil
}

// Unmount releases the filesystem mounted at mountpoint. Unmounting an
// already-unmounted mountpoint reports EINVAL, which callers switching modes
// can treat as already-released.
func Unmount(mountpoint string) error {
	if err := unix.Unmount(mountpoint, 0); err != nil {
		return fmt.Errorf("unmounting %s failed: %w", mountpoint, err)
	}
	return nil
}

// Mount mounts device read-write at mountpoint as a filesystem of kind fs,
// creating the mountpoint if it does not exist.
func Mount(device, mountpoint string, fs diskfmt.FS) error {
	fsType := fs.MountType()
	if fsType == "" {
		return fmt.Errorf("cannot mount %s: %q is not a filesystem GoSD knows how to mount", device, string(fs))
	}
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return fmt.Errorf("creating mountpoint %s failed: %w", mountpoint, err)
	}
	if err := unix.Mount(device, mountpoint, fsType, unix.MS_NOSUID|unix.MS_NODEV, mountData(fs)); err != nil {
		return fmt.Errorf("mount(%s, %s, %s) failed: %w", device, mountpoint, fsType, err)
	}
	return nil
}

// mountData is the comma-separated option string for a filesystem. The options
// are per-driver and mount(2) rejects one it does not know, so "flush" — which
// only Linux's vfat driver has — must not reach exfat. The vfat option itself
// comes from vfatMountOption, keyed off GOSD_DATA_FLUSH — gosd-init's
// computed effective setting, the only channel this process has to it (see
// that function's doc and bean gosd-9m1k).
func mountData(fs diskfmt.FS) string {
	if fs == diskfmt.FAT32 {
		return vfatMountOption(os.Getenv)
	}
	return ""
}

// Mountable reports whether the running kernel can mount a filesystem of kind
// fs. GoSD kernels are built without loadable modules, so every filesystem the
// kernel will ever have is already listed in /proc/filesystems — which makes
// this an exact answer rather than a guess, and lets a format be refused before
// it destroys anything.
func Mountable(fs diskfmt.FS) (bool, error) {
	fsType := fs.MountType()
	if fsType == "" {
		return false, nil
	}
	raw, err := os.ReadFile(procFilesystems)
	if err != nil {
		return false, fmt.Errorf("reading %s to check for %s support failed: %w", procFilesystems, fs, err)
	}
	// Each line is an optional "nodev" marker then the filesystem name.
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[len(fields)-1] == fsType {
			return true, nil
		}
	}
	return false, nil
}

// ReadBlockDevices enumerates every block device under /sys/block with the
// attributes candidate selection weighs. Deciding which of them may be
// formatted is the caller's Rank function, not this scan's.
func ReadBlockDevices() ([]Device, error) {
	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s failed: %w", sysBlockDir, err)
	}
	devices := make([]Device, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		devices = append(devices, Device{
			Name:        name,
			Kind:        readAttr(name, "device/type"),
			Partitions:  readPartitions(name),
			SizeSectors: readUint(name, "size"),
			ReadOnly:    readAttr(name, "ro") == "1",
		})
	}
	return devices, nil
}

func readAttr(name, attr string) string {
	raw, err := os.ReadFile(filepath.Join(sysBlockDir, name, attr))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func readUint(name, attr string) uint64 {
	v, err := strconv.ParseUint(readAttr(name, attr), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// readPartitions lists a block device's partition node names. Partition naming
// differs by device class ("sda1" but "nvme0n1p1"), so they are identified
// structurally instead: the kernel gives every partition directory under
// /sys/block/<name> a "partition" attribute, and gives it to nothing else.
func readPartitions(name string) []string {
	entries, err := os.ReadDir(filepath.Join(sysBlockDir, name))
	if err != nil {
		return nil
	}
	var parts []string
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(sysBlockDir, name, entry.Name(), "partition")); err == nil {
			parts = append(parts, entry.Name())
		}
	}
	return parts
}

// ext4IoctlResizeFS is EXT4_IOC_RESIZE_FS (fs/ext4/ext4.h: `_IOW('f', 16,
// __u64)`), computed from asm-generic/ioctl.h's _IOC() bit layout the same
// way examples/spiloopback derives SPI_IOC_MESSAGE: golang.org/x/sys/unix
// wraps generic syscalls, not device/filesystem-specific ioctls like this
// one, so there is no constant to import.
const (
	ext4IoctlMagic  = 'f'
	ext4IoctlNR     = 16
	ext4IoctlDir    = 1 // _IOC_WRITE: the kernel reads the __u64 argument.
	ioctlDirShift   = 30
	ioctlTypeShift  = 8
	ioctlSizeShift  = 16
	ext4IoctlSize64 = 8 // sizeof(__u64)
)

const ext4IoctlResizeFS = ext4IoctlDir<<ioctlDirShift | ext4IoctlMagic<<ioctlTypeShift | ext4IoctlNR | ext4IoctlSize64<<ioctlSizeShift

// blockDeviceSizeBytes reads device's size directly with the BLKGETSIZE64
// ioctl into a uint64, not the unix.IoctlGetInt/IoctlGetUint32 helpers'
// machine-word-sized buffers: the kernel always writes a full 8-byte answer
// regardless of the calling process's word size, and reading that into a
// narrower buffer silently truncates on 32-bit ARM (bean gosd-fjio — the
// same bug internal/diskfmt.openDisk's doc comment describes avoiding by a
// different means, lseek, for the same underlying reason). GrowEXT4 needs
// the ioctl specifically, not lseek's whole-file size: BLKGETSIZE64 reports
// the partition's usable size, which is what the filesystem is grown to
// fill.
func blockDeviceSizeBytes(device string) (int64, error) {
	f, err := os.Open(device)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	var size uint64
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size))); errno != 0 { //nolint:staticcheck // SA1019: unix.SYS_IOCTL is fine here; this file is linux-only.
		return 0, fmt.Errorf("BLKGETSIZE64 on %s failed: %w", device, errno)
	}
	return int64(size), nil
}

// GrowEXT4 expands the ext4 filesystem already mounted at mountpoint (backed
// by device) to fill device's actual size, via EXT4_IOC_RESIZE_FS. The
// target size is derived fresh from the block device itself
// (blockDeviceSizeBytes) rather than assumed from anything computed earlier
// in this process — the partition's real size is a fact about the device,
// not something to trust secondhand — and converted to a block count with
// the filesystem's own block size (read back via statfs, not hardcoded:
// diskfmt's golden image is built at 4KiB today, but this call should not
// silently start growing to the wrong target size if that ever changes).
//
// Callers must only invoke this against a filesystem just proven durable by
// a Mount that followed a SyncDevice of a Format that just completed — see
// runEXT4's doc for why that ordering is what makes an online grow safe.
//
// Before returning success, GrowEXT4 syncfs(2)s the filesystem: an online
// resize is a journaled metadata operation, and while ext4's journal
// (JBD2) commits transactions in strict order — so EstablishMarker's later
// fsync of the marker file would, in practice, also force this transaction
// to commit — that is an implementation detail of how ext4 happens to
// sequence commits, not a documented contract this package should lean on.
// Grow's own postcondition is meant to be self-contained: once it returns
// nil, the grow is durable, full stop, regardless of what any later step
// does or doesn't fsync.
func GrowEXT4(device, mountpoint string) (err error) {
	deviceBytes, err := blockDeviceSizeBytes(device)
	if err != nil {
		return fmt.Errorf("reading %s's size to grow the ext4 filesystem at %s failed: %w", device, mountpoint, err)
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(mountpoint, &stat); err != nil {
		return fmt.Errorf("reading the block size of the ext4 filesystem at %s failed: %w", mountpoint, err)
	}
	blockSize := int64(stat.Bsize)
	if blockSize <= 0 {
		return fmt.Errorf("the ext4 filesystem at %s reported a block size of %d, which cannot be used to compute a target block count", mountpoint, blockSize)
	}
	targetBlocks := uint64(deviceBytes / blockSize)

	f, err := os.Open(mountpoint)
	if err != nil {
		return fmt.Errorf("opening %s to grow its ext4 filesystem failed: %w", mountpoint, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %s after growing its ext4 filesystem: %w", mountpoint, cerr)
		}
	}()

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ext4IoctlResizeFS, uintptr(unsafe.Pointer(&targetBlocks))); errno != 0 { //nolint:staticcheck // SA1019: unix.SYS_IOCTL is fine here; this file is linux-only.
		return fmt.Errorf("growing the ext4 filesystem at %s to %d blocks (device %s is %d bytes) failed: %w", mountpoint, targetBlocks, device, deviceBytes, errno)
	}
	// Syncfs, not Sync: this flushes every dirty page belonging to the
	// filesystem mounted at mountpoint (including the resize's metadata
	// changes), unlike SyncDevice's plain fsync of a device node, which is
	// only safe to use before this filesystem is mounted at all (see
	// SyncDevice's doc and runEXT4's ordering argument).
	if err := unix.Syncfs(int(f.Fd())); err != nil {
		return fmt.Errorf("flushing the grown ext4 filesystem at %s to the medium failed: %w", mountpoint, err)
	}
	return nil
}

// SyncDevice fsyncs a whole-device or partition block-device node, flushing
// its page-cache-buffered writes (e.g. a filesystem Format just wrote) to
// the medium. Mirrors cmd/gosd-init/internal/dataexpand's syncDevice — same
// operation, same reason: the write it follows must be provably durable
// before anything downstream (here, a Mount) is allowed to trust it.
func SyncDevice(device string) (err error) {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return f.Sync()
}

// EstablishEXT4Marker writes EXT4EstablishedMarker into the root of the
// filesystem mounted at mountpoint, fsyncs the marker file itself, then
// fsyncs mountpoint (its parent directory). Both fsyncs matter and neither
// substitutes for the other: the first makes the file's own data (empty,
// here, but the call is the same for any file) and inode durable; ext4, like
// most Linux filesystems, does not guarantee a new directory entry is
// durable just because the file it names is — the entry lives in the parent
// directory's own data, which needs its own fsync. Skipping either one would
// let a crash leave a marker that exists in the page cache but not on the
// medium, or (subtler) a marker file that is itself durable but whose
// directory entry is not — either way, a future boot's MarkerEstablished
// check might see a marker that reboots back into nonexistence, precisely
// the probe-is-not-proof failure this whole mechanism exists to avoid.
func EstablishEXT4Marker(mountpoint string) error {
	path := filepath.Join(mountpoint, EXT4EstablishedMarker)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("creating the establishment marker at %s failed: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsyncing the establishment marker at %s failed: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing the establishment marker at %s failed: %w", path, err)
	}

	dir, err := os.Open(mountpoint)
	if err != nil {
		return fmt.Errorf("opening %s to fsync the establishment marker's directory entry failed: %w", mountpoint, err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("fsyncing %s after creating the establishment marker failed: %w", mountpoint, err)
	}
	return nil
}

// EXT4MarkerEstablished reports whether the filesystem already mounted at
// mountpoint carries EXT4EstablishedMarker.
func EXT4MarkerEstablished(mountpoint string) (bool, error) {
	_, err := os.Stat(filepath.Join(mountpoint, EXT4EstablishedMarker))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking for the establishment marker at %s failed: %w", mountpoint, err)
}

// ext4ReservedRootEntries are the root-directory entries diskfmt's golden
// ext4 image (internal/diskfmt/ext4golden) itself ships with — present on
// every freshly Format-ed volume before an app has ever written anything, so
// their presence alone is not evidence of real content. mke2fs creates
// lost+found unconditionally (it is not one of the -O feature toggles the
// golden's build pins, see build/ext4-golden/Dockerfile); nothing else is
// pre-populated.
var ext4ReservedRootEntries = map[string]bool{"lost+found": true}

// RootHasOtherContent reports whether the ext4 filesystem mounted at
// mountpoint holds anything in its root directory beyond ext4ReservedRootEntries
// and EXT4EstablishedMarker itself — see Deps.RootHasOtherContent's doc for
// why runEXT4 needs this second opinion before ever reformatting a
// no-marker volume.
func RootHasOtherContent(mountpoint string) (bool, error) {
	entries, err := os.ReadDir(mountpoint)
	if err != nil {
		return false, fmt.Errorf("reading the root directory of %s failed: %w", mountpoint, err)
	}
	for _, e := range entries {
		if e.Name() == EXT4EstablishedMarker || ext4ReservedRootEntries[e.Name()] {
			continue
		}
		return true, nil
	}
	return false, nil
}
