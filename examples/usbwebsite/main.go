// Command usbwebsite turns a GoSD board into a tiny self-contained website
// appliance that you edit by USB. On a standalone boot it serves its storage
// volume's contents as a static website over HTTP; plugged into a computer it
// presents that same volume as a removable USB drive, so you can drop or edit
// the site's files, then power it standalone again to serve them.
//
// The volume is the onboard eMMC on boards that have one fitted, and
// otherwise the SD card's data partition — so eMMC-less boards like the
// Raspberry Pi Zeros work too, as long as the image was built with
// `gosd build --data-size` (which creates that partition pre-formatted; the
// app never formats anything on the SD card). The board must be built with
// `gosd build --usb-gadget` so its USB port is in peripheral mode (see
// COMPATIBILITY.md for current per-board status). Without either volume it
// logs what to do and idles rather than exiting; without a USB controller it
// just serves.
//
// It demonstrates gadget.MassStorage (sharing a block device over USB) on top
// of the emmc package and gosd-init's data-partition auto-mount. The
// USB-vs-website decision is made once per boot: presenting the drive and
// mounting it locally must never be live at the same time (the host writes
// raw blocks with no knowledge of our filesystem), so the app either hands
// the device to a connected computer or keeps it mounted to serve — never
// both. It formats the eMMC as FAT32 rather than emmc's ext4 default (see
// emmcOptions): the whole "plug in, drag files, eject" workflow depends on
// the volume being natively readable from macOS and Windows, which ext4
// is not.
//
// A board whose eMMC already holds other content (a vendor image, a prior
// project) needs explicit consent before this app claims it: set the
// WEBSITE_WIPE_EMMC app setting in config/env/ (see docs/runtime.md's "App
// environment variables") to "yes" (or "1"/"true"/"on") to let it wipe and
// reformat that eMMC. Without consent it leaves the eMMC untouched, logs
// what to do about it, and idles rather than exiting — gosd-init restarts
// exited apps regardless of exit code, so exiting here would just crash-loop.
// The data-partition path needs no such consent: that partition is created by
// `gosd build` for app data, and this app only ever mounts and shares it.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jphastings/gosd/emmc"
	"github.com/jphastings/gosd/gadget"
)

const (
	emmcLabel      = "WEBSITE"
	emmcMountpoint = "/storage"
	httpAddr       = ":80"

	// dataMountpoint is where gosd-init mounts the data partition on
	// every boot; bootMountpoint is where it mounts the boot partition
	// (partition 1 of the same disk), from which the data partition's device
	// node can be derived when it isn't currently mounted.
	dataMountpoint = "/data"
	bootMountpoint = "/boot"

	// wipeConsentEnv is the config/env/ setting (see docs/runtime.md's "App
	// environment variables") a user sets to let usbwebsite claim an eMMC
	// that already holds other content. Unset, the app only ever formats an
	// eMMC that's blank or already carries its own label.
	wipeConsentEnv = "WEBSITE_WIPE_EMMC"

	udcDir = "/sys/class/udc"

	// vendorID and productID are the Linux kernel's own g_mass_storage gadget
	// placeholder USB IDs (Linux Foundation) — a recognized development
	// pairing, not a USB-IF-assigned VID for a shipping product.
	vendorID  = 0x1d6b
	productID = 0x0104

	// hostSettle bounds how long we wait, after presenting the drive, for a
	// connected computer to enumerate and configure it. A computer does this in
	// well under a second; a power-only supply never will, so this also caps
	// how long a standalone boot spends probing before it falls back to
	// serving.
	hostSettle = 4 * time.Second
	hostPoll   = 200 * time.Millisecond

	readHeaderTimeout = 10 * time.Second
)

// storage is the volume this boot serves and shares: where its filesystem is
// mounted, the block device behind it (handed raw to gadget.MassStorage),
// and how to release and restore the mount when switching between the two —
// which differs between the eMMC and data-partition backings.
type storage struct {
	mountpoint string
	device     string
	// source names the backing for log lines, e.g. "onboard eMMC".
	source  string
	unmount func() error
	remount func() error
}

func main() {
	st, ok := claimStorage()
	if !ok {
		// claimStorage has logged the outside action needed (consent, or a
		// rebuild with --data-size); idle so gosd-init's restart-on-exit
		// doesn't crash-loop us while we wait for it.
		idleForever()
	}
	fmt.Printf("gosd usbwebsite: %s ready at %s (device %s)\n", st.source, st.mountpoint, st.device)

	if presentedAsDrive(st) {
		// A computer is editing the files; stay a drive until it is unplugged
		// and the board reboots. Serving now would fight the host for the
		// device.
		fmt.Println("gosd usbwebsite: computer attached — sharing the website storage as a USB drive")
		fmt.Println("gosd usbwebsite: edit the files, eject the drive, then power the board standalone to serve them")
		idleForever()
	}

	serveWebsite(st.mountpoint)
}

// emmcOptions pins the eMMC to FAT32 rather than emmc's ext4 default
// (gosd-9sc4): usbwebsite's entire point is presenting this volume to a
// connected computer via gadget.MassStorage so its files can be dropped or
// edited directly, and ext4 is not natively readable from macOS or Windows —
// FAT32 is what makes that "plug in, drag files, eject" workflow work.
func emmcOptions(destructive bool) emmc.Options {
	return emmc.Options{Filesystem: emmc.FAT32, Destructive: destructive}
}

// claimStorage picks this boot's website volume: the onboard eMMC when the
// board has one (formatted on first use, with the same consent gate as
// before), otherwise the SD card's data partition, which `gosd build
// --data-size` creates pre-formatted and gosd-init mounts at /data. It
// returns ok=false — after logging the action that would fix it — when the
// board has neither volume or the eMMC needs consent the user hasn't given;
// unexpected errors exit so gosd-init restarts (and thereby retries) the app.
func claimStorage() (storage, bool) {
	destructive := wipeConsented()
	res := <-emmc.FormatAndMountWith(emmcLabel, emmcMountpoint, emmcOptions(destructive))
	switch {
	case res.Err == nil:
		return storage{
			mountpoint: res.MountPoint,
			device:     res.BlockDevice,
			source:     "onboard eMMC (" + emmcLabel + ")",
			unmount:    func() error { return emmc.Unmount(res.MountPoint) },
			// FormatAndMountWith is idempotent here: it only remounts, never
			// reformats, an eMMC that already carries this app's label as
			// FAT32.
			remount: func() error {
				r := <-emmc.FormatAndMountWith(emmcLabel, emmcMountpoint, emmcOptions(false))
				return r.Err
			},
		}, true
	case errors.Is(res.Err, emmc.ErrNoEMMC):
		return claimDataPartition()
	case !destructive && errors.Is(res.Err, emmc.ErrRefusedFormat):
		fmt.Printf("gosd usbwebsite: %v\n", res.Err)
		fmt.Printf("gosd usbwebsite: to let usbwebsite claim it, write yes into config/env/%s on the boot partition, then reboot\n", wipeConsentEnv)
		return storage{}, false
	default:
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: %v\n", res.Err)
		os.Exit(1)
	}
	return storage{}, false // unreachable: every case above returns or exits
}

// claimDataPartition claims the SD card's data partition as the website
// volume. gosd-init normally has it mounted at /data already; when it isn't
// mounted but its device node exists (a mount raced or failed at boot, or a
// warm restart after this app released it), the partition is mounted here.
// Nothing on this path ever formats or relabels the partition — it was
// created for app data by `gosd build`, and hosts only ever see the
// filesystem it already carries.
func claimDataPartition() (storage, bool) {
	part, err := findDataPartition()
	if err != nil {
		fmt.Println("gosd usbwebsite: no onboard eMMC on this board, and no data partition to fall back to")
		fmt.Printf("gosd usbwebsite: %v\n", err)
		fmt.Println("gosd usbwebsite: rebuild the image with `gosd build --usb-gadget --data-size 256MiB` (or larger) to give the website somewhere to live")
		return storage{}, false
	}
	if !part.mounted {
		if err := mountVFAT(part.device, dataMountpoint); err != nil {
			fmt.Printf("gosd usbwebsite: the data partition (%s) exists but could not be mounted: %v\n", part.device, err)
			fmt.Println("gosd usbwebsite: its filesystem may be damaged — repair it on a computer, or reflash the image")
			return storage{}, false
		}
	}
	return storage{
		mountpoint: dataMountpoint,
		device:     part.device,
		source:     "SD card data partition",
		unmount:    func() error { return unmountVFAT(dataMountpoint) },
		remount:    func() error { return mountVFAT(part.device, dataMountpoint) },
	}, true
}

// dataPartition locates the data partition: the device node backing it
// and whether it is currently mounted at dataMountpoint.
type dataPartition struct {
	device  string
	mounted bool
}

// procMounts is the kernel's mount table, read to locate the data
// partition and whether gosd-init already mounted it.
const procMounts = "/proc/mounts"

// findDataPartition locates the data partition on the booted disk and
// reports whether it's currently mounted at dataMountpoint. An error means
// there is none to use — the caller's cue that this image was built without
// `--data-size`.
func findDataPartition() (dataPartition, error) {
	raw, err := os.ReadFile(procMounts)
	if err != nil {
		return dataPartition{}, fmt.Errorf("reading %s: %w", procMounts, err)
	}
	part, ok := dataPartitionFromMounts(string(raw))
	if !ok {
		return dataPartition{}, fmt.Errorf("nothing is mounted at %s and no boot disk shows in %s to derive it from", dataMountpoint, procMounts)
	}
	if !part.mounted {
		if _, err := os.Stat(part.device); err != nil {
			return dataPartition{}, fmt.Errorf("this image has no data partition (%s does not exist)", part.device)
		}
	}
	return part, nil
}

// dataPartitionFromMounts finds the data partition in a mount table
// (/proc/mounts format): the block device mounted at dataMountpoint when
// gosd-init mounted it at boot, otherwise the second partition of the disk
// the boot partition mounted from — the same p1→p2 relationship gosd-init's
// own candidate device lists encode. The read-only tmpfs gosd-init mounts
// over /data when no partition mounted is not a block device and never
// matches. Later mount entries win, matching the kernel's stacking order.
func dataPartitionFromMounts(mounts string) (dataPartition, bool) {
	var dataDevice, bootDevice string
	for _, line := range strings.Split(mounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "/dev/") {
			continue
		}
		switch fields[1] {
		case dataMountpoint:
			dataDevice = fields[0]
		case bootMountpoint:
			bootDevice = fields[0]
		}
	}
	if dataDevice != "" {
		return dataPartition{device: dataDevice, mounted: true}, true
	}
	if sibling := secondPartition(bootDevice); sibling != "" {
		return dataPartition{device: sibling}, true
	}
	return dataPartition{}, false
}

// secondPartition maps a boot-partition device node to its disk's second
// partition — "/dev/mmcblk0p1" to "/dev/mmcblk0p2", "/dev/vda1" to
// "/dev/vda2" — or "" when dev isn't a first-partition node.
func secondPartition(dev string) string {
	base, ok := strings.CutSuffix(dev, "1")
	if !ok || base == "" {
		return ""
	}
	return base + "2"
}

// wipeConsented reports whether the user has opted in, via wipeConsentEnv, to
// letting usbwebsite wipe and claim an eMMC that already holds other content.
func wipeConsented() bool {
	return isAffirmative(os.Getenv(wipeConsentEnv))
}

// isAffirmative recognizes the usual "yes" spellings for a boolean app
// setting, case-insensitively; anything else, including unset or empty,
// means no — the safe default.
func isAffirmative(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// idleForever blocks forever without exiting, so gosd-init's automatic
// restart-on-exit (which applies regardless of exit code) doesn't
// crash-loop this app while it waits on outside action — a user setting an
// env var, or plugging in a computer. A bare `select {}` isn't safe for this:
// with no other goroutine able to wake it, the Go runtime treats that as a
// deadlock and panics instead of blocking.
func idleForever() {
	for {
		time.Sleep(time.Hour)
	}
}

// presentedAsDrive tries to hand the storage volume to a connected computer
// as a USB mass-storage drive. It returns true only if a computer actually
// enumerated and configured it. On every other outcome — no USB gadget
// controller, no cable, a power-only supply, or a setup error — it leaves
// (or restores) the volume mounted at st.mountpoint and returns false, so
// the caller serves the website instead.
func presentedAsDrive(st storage) bool {
	udc, err := firstUDC()
	if err != nil {
		fmt.Printf("gosd usbwebsite: not offering a USB drive (%v)\n", err)
		return false
	}
	if udcState(udc) == "not attached" {
		// No USB cable/VBUS at all: definitely standalone, so skip the probe
		// and serve straight away.
		return false
	}

	// Give up our mount of the device before exposing it: a mass-storage LUN
	// and a local mount of the same block device must never be live at once.
	if err := st.unmount(); err != nil {
		fmt.Printf("gosd usbwebsite: could not release %s to share it (%v); serving instead\n", st.mountpoint, err)
		return false
	}

	g := &gadget.Gadget{
		VendorID:     vendorID,
		ProductID:    productID,
		Manufacturer: "GoSD",
		Product:      "GoSD Website Storage",
		Serial:       "usbwebsite-example",
		Functions: []gadget.Function{
			gadget.MassStorage{Path: st.device, Removable: true},
		},
	}
	if err := g.Apply(); err != nil {
		fmt.Printf("gosd usbwebsite: presenting the USB drive failed (%v); serving instead\n", err)
		remount(st)
		return false
	}

	if awaitConfigured(udc, hostSettle) {
		return true
	}

	// VBUS present but nothing enumerated us (e.g. a phone charger): tear the
	// drive back down, remount, and serve.
	fmt.Println("gosd usbwebsite: no computer enumerated the drive; serving the website instead")
	_ = g.Close()
	remount(st)
	return false
}

// serveWebsite serves dir as a static site forever. A brand-new volume has no
// index.html, so it drops in a starter page first.
func serveWebsite(dir string) {
	ensureStarterPage(dir)
	fmt.Printf("gosd usbwebsite: serving %s on %s\n", dir, httpAddr)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           http.FileServer(http.Dir(dir)),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: %v\n", err)
		os.Exit(1)
	}
}

// starterPage is the placeholder index.html a brand-new website volume gets,
// explaining how to add real content.
const starterPage = `<!doctype html>
<title>GoSD usbwebsite</title>
<h1>It works!</h1>
<p>This page is served by a GoSD board from its website storage.</p>
<p>Plug the board into a computer over USB and it appears as a removable drive
(labelled WEBSITE when backed by onboard eMMC, or usbweb-data — derived from
this app's name — when backed by the SD card). Replace this index.html (and
add whatever else you like), eject the drive, then power the board on its
own again to serve your site.</p>
`

// ensureStarterPage writes the placeholder index.html when the site has none,
// so a brand-new board serves something that explains how to add real
// content — and repairs it if it's present but truncated or empty, which is
// what a power cut mid-write can leave behind (this used to be written with a
// bare os.WriteFile, and the existence check alone then mistook that debris
// for the user's own content and never touched it again).
func ensureStarterPage(dir string) {
	index := filepath.Join(dir, "index.html")

	raw, err := os.ReadFile(index)
	switch {
	case err == nil:
		if !isTruncatedStarter(string(raw)) {
			return // the user's own content is already here
		}
		fmt.Println("gosd usbwebsite: index.html is a truncated/empty starter page (likely an interrupted write after a power cut); repairing it")
	case errors.Is(err, os.ErrNotExist):
		// no page yet; fall through to write the starter
	default:
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: could not check the existing index.html: %v\n", err)
		return
	}

	if err := writeFileDurably(index, []byte(starterPage)); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: could not write the starter page: %v\n", err)
	}
}

// isTruncatedStarter reports whether content is a strict prefix of
// starterPage, including empty — exactly what a power cut partway through
// writing the starter page can leave on a FAT volume with no fsync/rename
// protecting it. It never matches genuine user content: nobody's own
// index.html happens to begin with our exact starter text and then just
// stop partway through.
func isTruncatedStarter(content string) bool {
	return len(content) < len(starterPage) && strings.HasPrefix(starterPage, content)
}

// writeFileDurably writes data to path so that a power cut leaves either the
// old contents or the new, never a torn mix, and so that the new contents are
// on the card by the time it returns: write a temp file, fsync it, rename it
// over the real name, then fsync the renamed file and its directory. Both
// backings usbwebsite serves from (the eMMC's whole-device FAT and the SD
// card's data FAT32 partition) share the same weak crash-safety, so the
// same pattern applies — see docs/runtime.md's "Making a write durable".
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
	// A FAT rename only dirties directory blocks, which otherwise wait for
	// writeback expiry (~30s). Syncing the still-open file writes its new
	// directory entry with the real start cluster and size; syncing the
	// directory writes the entry the rename added and the one it removed.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// syncDir fsyncs a directory itself, so directory entries added or removed in
// it reach the card rather than waiting for writeback.
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

// firstUDC returns the board's first USB peripheral controller under
// /sys/class/udc, or an error naming why gadget mode is unavailable. It
// wraps gadget.ErrNoController on the not-found case (rather than just a
// bespoke string) so this pre-check participates in the same errors.Is
// contract as a direct gadget.Gadget.Apply() call would - the worked
// example for gosd-ctkj's sentinel, even though this app also needs the
// controller's name up front (to skip the unmount/Apply cycle entirely
// when no cable is attached - see presentedAsDrive), so it can't just call
// Apply and inspect its error the way an app with no such pre-check could.
func firstUDC() (string, error) {
	entries, err := os.ReadDir(udcDir)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", udcDir, err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("%w under %s; build with `gosd build --usb-gadget`", gadget.ErrNoController, udcDir)
	}
	return entries[0].Name(), nil
}

// udcState reads the controller's USB device state ("not attached", "powered",
// "configured", …), or "" if it can't be read.
func udcState(udc string) string {
	raw, err := os.ReadFile(udcDir + "/" + udc + "/state")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// awaitConfigured polls the controller until a host has configured the gadget
// (the USB "configured" state) or timeout elapses.
func awaitConfigured(udc string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if udcState(udc) == "configured" {
			return true
		}
		time.Sleep(hostPoll)
	}
	return false
}

// remount restores the volume's local mount after the drive is torn down, so
// the fall-back-to-serving path has a filesystem to serve.
func remount(st storage) {
	if err := st.remount(); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: remounting %s failed: %v\n", st.mountpoint, err)
	}
}
