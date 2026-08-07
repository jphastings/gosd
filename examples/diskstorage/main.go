// Command diskstorage exercises the public disk package's default
// filesystem: disk.FormatAndMountWith's zero-value Options formats ext4 (see
// disk.Options' docs, epic gosd-lfu0), not FAT32 — proving the DEFAULT, not
// an explicit disk.EXT4 token. It durably writes a boot counter to the
// mounted volume with docs/runtime.md's four-step fsync pattern (the app
// contract the ext4 journal does not replace — the journal buys metadata
// crash-consistency and mount-time replay, not data durability), then
// serves HTTP so a harness can poll for readiness the same way
// examples/hello does.
//
// It is CI's qemu-disk-ext4 job's test app (.github/workflows/ci.yml):
// booted twice against the same pair of virtio disks with qemu hard-killed
// in between (no clean shutdown — the point, same as qemu-expand-data's
// kill-and-reboot), the counter reaching "2" on the second boot is the
// proof the volume was adopted rather than reformatted, and last boot's
// write survived the ext4 journal replay intact — the same shape as
// examples/hello's boots=N proof for /data. The job separately greps the
// logged filesystem size to confirm the volume was grown to the attached
// disk's size rather than staying at internal/diskfmt/ext4golden's fixed
// 512MiB image.
//
// Off a board with no second disk attached, it degrades gracefully like
// examples/emmcstorage does for ErrNoEMMC: log and exit cleanly rather than
// treating the absence as a failure.
package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jphastings/gosd/disk"
)

const (
	label      = "DISKTEST"
	mountpoint = "/storage"
)

func main() {
	// disk.Options{} is the zero value: Filesystem's zero value is
	// disk.EXT4 (the epic's deliberate breaking default), Device
	// discovers automatically, and Destructive stays false.
	res := <-disk.FormatAndMountWith(label, mountpoint, disk.Options{})
	if res.Err != nil {
		if errors.Is(res.Err, disk.ErrNoDisk) {
			fmt.Println("gosd disk: no usable disk attached - nothing to do")
			return
		}
		fmt.Fprintf(os.Stderr, "gosd disk: %v\n", res.Err)
		os.Exit(1)
	}

	fmt.Printf("gosd disk: %s ready at %s (device %s)\n", label, res.MountPoint, res.BlockDevice)

	boots := bumpBootCounter(res.MountPoint)
	fmt.Printf("gosd disk: boots=%s\n", boots)

	reportFilesystemSize(res.MountPoint)

	serve(boots)
}

func serve(boots string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "boots=%s\n", boots)
	})

	listener, err := net.Listen("tcp", ":80")
	if err != nil {
		listener, err = net.Listen("tcp", ":8080")
		if err != nil {
			fmt.Fprintf(os.Stderr, "gosd disk: failed to listen on :80 or :8080: %v\n", err)
			os.Exit(1)
		}
	}
	if err := http.Serve(listener, nil); err != nil {
		fmt.Fprintf(os.Stderr, "gosd disk: server stopped: %v\n", err)
		os.Exit(1)
	}
}

// bumpBootCounter mirrors examples/hello's boot-counter demonstration,
// aimed at the disk package's mountpoint instead of /data: a count that
// only reaches "2" if the previous boot's durably-written file survived —
// which for an established ext4 volume means it was adopted, not
// reformatted, and its content came back intact through mount-time journal
// replay.
func bumpBootCounter(mountpoint string) string {
	counterPath := filepath.Join(mountpoint, "disk-boots")

	count := 0
	if raw, err := os.ReadFile(counterPath); err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
	}
	count++

	if err := writeFileDurably(counterPath, []byte(strconv.Itoa(count)+"\n")); err != nil {
		fmt.Fprintf(os.Stderr, "gosd disk: persisting the boot counter failed: %v\n", err)
		return "write-failed"
	}
	return strconv.Itoa(count)
}

// writeFileDurably is docs/runtime.md's "Making a write durable" sequence:
// write a temp file, fsync it, rename it over the real name, then fsync the
// renamed file and its directory. It applies unchanged on ext4: the journal
// gives crash *consistency* (no torn writes) and replays cleanly on mount,
// but only fsync gives *durability* against a power cut that lands before
// the next journal commit — see docs/runtime.md's "Storage" section.
func writeFileDurably(path string, data []byte) error {
	tmp := path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// syncDir fsyncs a directory itself, so directory entries added or removed
// in it reach the disk rather than waiting for writeback.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// reportFilesystemSize logs the mounted volume's total size — CI's proof
// that Grow (EXT4_IOC_RESIZE_FS, see internal/blockmount) actually ran: an
// established volume's size tracks the backing disk, not
// internal/diskfmt/ext4golden's fixed 512MiB golden image. Failing to read
// it is logged, not fatal — the boot-counter proof above is the load-bearing
// assertion.
func reportFilesystemSize(mountpoint string) {
	total, err := filesystemSizeBytes(mountpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosd disk: reading filesystem size failed: %v\n", err)
		return
	}
	fmt.Printf("gosd disk: filesystem size %d bytes\n", total)
}
