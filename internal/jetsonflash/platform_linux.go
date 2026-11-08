//go:build linux

package jetsonflash

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	nvidiaVendor = "0955"
	t210Recovery = "7f21"
	endpointOut  = 0x01
	endpointIn   = 0x81

	usbdevfsClaimInterface = 0x8004550f // _IOR('U', 15, unsigned int)
	// usbdevfsBulk is _IOWR('U', 2, struct usbdevfs_bulktransfer); the
	// struct is 16 bytes on 32-bit kernels and 24 on 64-bit ones.
	usbdevfsBulk = 0xc0005502 | unsafe.Sizeof(bulkTransfer{})<<16
)

type bulkTransfer struct {
	ep, len, timeoutMS uint32
	data               unsafe.Pointer
}

type usbfs struct{ fd int }

// openRecovery waits up to wait for a Tegra210 in USB recovery mode to
// appear, then opens it through usbfs and claims its interface.
func openRecovery(wait time.Duration) (conn, error) {
	deadline := time.Now().Add(wait)
	for {
		node, err := findRecoveryDevice()
		if err != nil {
			return nil, err
		}
		if node != "" {
			return openUSBFS(node)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("no Tegra210 in USB recovery mode (%s:%s) appeared within %s; put the module into recovery mode (on a Turing Pi 2: tpi power -n N off; tpi usb flash -n N --bmc; tpi power -n N on)", nvidiaVendor, t210Recovery, wait)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func findRecoveryDevice() (string, error) {
	const devices = "/sys/bus/usb/devices"
	entries, err := os.ReadDir(devices)
	if err != nil {
		return "", fmt.Errorf("listing USB devices: %w", err)
	}
	for _, e := range entries {
		dir := filepath.Join(devices, e.Name())
		if sysfsAttr(dir, "idVendor") != nvidiaVendor || sysfsAttr(dir, "idProduct") != t210Recovery {
			continue
		}
		bus, err1 := strconv.Atoi(sysfsAttr(dir, "busnum"))
		dev, err2 := strconv.Atoi(sysfsAttr(dir, "devnum"))
		if err1 != nil || err2 != nil {
			continue // mid-enumeration; the next poll will see it whole
		}
		return fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, dev), nil
	}
	return "", nil
}

func sysfsAttr(dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return strings.TrimSpace(string(b))
}

func openUSBFS(node string) (conn, error) {
	fd, err := unix.Open(node, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w; jetson-flash needs write access to usbfs, so run it as root", node, err)
	}
	// ponytail: interface 0, the recovery device's only one; read the
	// descriptors if a Tegra ever presents more.
	if err := unix.IoctlSetPointerInt(fd, usbdevfsClaimInterface, 0); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("claiming %s's USB interface: %w", node, err)
	}
	return &usbfs{fd: fd}, nil
}

func (u *usbfs) Write(p []byte, timeout time.Duration) error {
	n, err := u.bulk(endpointOut, p, timeout)
	if err == nil && n != len(p) {
		err = fmt.Errorf("short USB write: %d of %d bytes", n, len(p))
	}
	return err
}

func (u *usbfs) Read(p []byte, timeout time.Duration) (int, error) {
	return u.bulk(endpointIn, p, timeout)
}

func (u *usbfs) Close() error { return unix.Close(u.fd) }

func (u *usbfs) bulk(ep uint32, p []byte, timeout time.Duration) (int, error) {
	t := bulkTransfer{ep: ep, len: uint32(len(p)), timeoutMS: uint32(timeout.Milliseconds())}
	if len(p) > 0 {
		t.data = unsafe.Pointer(&p[0])
	}
	n, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(u.fd), usbdevfsBulk, uintptr(unsafe.Pointer(&t)))
	runtime.KeepAlive(p)
	switch errno {
	case 0:
		return int(n), nil
	case unix.ETIMEDOUT:
		return 0, errTimeout
	}
	return 0, fmt.Errorf("USB bulk transfer on endpoint %#02x: %w", ep, errno)
}
