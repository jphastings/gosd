// Command i2cscan is a minimal example demonstrating raw I2C access on
// GoSD: it opens every /dev/i2c-* character device present, politely probes
// each 7-bit address for a responding device, and additionally checks the
// two addresses (0x76/0x77) a common BME280/BMP280-family pressure/humidity
// breakout board answers on, reporting its chip-ID register if one's found.
//
// It talks to the bus directly via the standard Linux i2c-dev ioctls
// (I2C_SLAVE, I2C_RDWR) through golang.org/x/sys/unix, rather than a
// higher-level sensor driver library, so it has no dependency on which
// sensor (if any) is actually attached. For real applications, start from
// periph.io instead — see docs/runtime.md's "GPIO, I2C, SPI" section.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	// i2cSlave and i2cRdwr are the standard Linux i2c-dev ioctl request
	// numbers (linux/i2c-dev.h). golang.org/x/sys/unix wraps generic
	// syscalls, not device-specific ioctls like these, so they're defined
	// here directly; they're stable, arch-independent ABI constants, not
	// something the kernel is expected to change.
	i2cSlave = 0x0703
	i2cRdwr  = 0x0707

	// i2cMRd marks an i2c_msg as a read (linux/i2c.h); zero means write.
	i2cMRd = 0x0001

	// scanFirstAddr and scanLastAddr bound the polite scan to the 7-bit
	// address range i2cdetect itself uses: 0x00-0x02 and 0x78-0x7f are
	// reserved for special protocols (general call, 10-bit addressing,
	// SMBus alerts, ...), not device addresses, so probing them risks
	// confusing whatever's listening for those protocols instead of
	// finding a device.
	scanFirstAddr = 0x03
	scanLastAddr  = 0x77

	// bme280Addr and bme280AltAddr are the BME280/BMP280 family's two
	// possible addresses, selected in hardware by the sensor's SDO pin.
	bme280Addr    = 0x76
	bme280AltAddr = 0x77

	// chipIDReg is the register holding a fixed, chip-specific ID byte on
	// this sensor family - reading it is the standard "is this really the
	// chip I think it is" check before trusting any other register.
	chipIDReg = 0xd0
)

// knownChipIDs maps a BME280/BMP280-family chip-ID register value to the
// sensor it identifies. Any value found there that isn't in this map is
// reported as unknown rather than guessed at.
var knownChipIDs = map[byte]string{
	0x60: "BME280",
	0x58: "BMP280",
}

func main() {
	buses, err := filepath.Glob("/dev/i2c-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosd i2cscan: listing /dev/i2c-*: %v\n", err)
		os.Exit(1)
	}
	if len(buses) == 0 {
		fmt.Println("gosd i2cscan: no /dev/i2c-* device found - is I2C enabled for this board? see docs/runtime.md")
		return
	}
	sort.Strings(buses)

	for _, bus := range buses {
		scanBus(bus)
	}
}

// scanBus politely scans one bus's full address range, then separately
// checks for a BME280/BMP280-family chip-ID response, printing what it
// finds (or that it found nothing).
func scanBus(bus string) {
	f, err := os.OpenFile(bus, os.O_RDWR, 0)
	if err != nil {
		fmt.Printf("%s: opening failed: %v\n", bus, err)
		return
	}
	defer func() { _ = f.Close() }()

	fd := int(f.Fd())

	found := false
	for addr := scanFirstAddr; addr <= scanLastAddr; addr++ {
		if probe(fd, addr) {
			found = true
			fmt.Printf("%s: device present at 0x%02x\n", bus, addr)
		}
	}
	if !found {
		fmt.Printf("%s: no device found on the bus\n", bus)
	}

	for _, addr := range []int{bme280Addr, bme280AltAddr} {
		id, ok := readChipID(fd, addr)
		if !ok {
			continue
		}
		name, known := knownChipIDs[id]
		if !known {
			name = "unknown chip"
		}
		fmt.Printf("%s: chip-id 0x%02x at 0x%02x (%s)\n", bus, id, addr, name)
	}
}

// probe politely checks for a device at addr: it sets addr as the bus's
// target slave address, then attempts to read a single byte. i2cdetect's
// default probe is a zero-length "quick write", which some write-sensitive
// devices (e.g. certain EEPROMs) can misinterpret as the start of a write
// cycle; a single-byte read (i2cdetect's -r mode) is the gentler of the two
// standard probe styles, which is why this example uses it instead.
func probe(fd, addr int) bool {
	if err := unix.IoctlSetInt(fd, i2cSlave, addr); err != nil {
		return false
	}
	var b [1]byte
	_, err := unix.Read(fd, b[:])
	return err == nil
}

// readChipID performs a write-then-read transaction (the chip-ID register
// address, then one byte back) as a single I2C_RDWR ioctl, so the repeated
// start between the write and the read is atomic. A plain write() followed
// by a separate read() risks another transaction — or the device
// deselecting — landing in between, which this sensor family's
// register-read protocol doesn't tolerate.
func readChipID(fd, addr int) (byte, bool) {
	if err := unix.IoctlSetInt(fd, i2cSlave, addr); err != nil {
		return 0, false
	}

	reg := [1]byte{chipIDReg}
	var resp [1]byte

	msgs := [2]i2cMsg{
		{addr: uint16(addr), buf: unsafe.Pointer(&reg[0]), length: 1},
		{addr: uint16(addr), flags: i2cMRd, buf: unsafe.Pointer(&resp[0]), length: 1},
	}
	data := i2cRdwrIoctlData{
		msgs:  unsafe.Pointer(&msgs[0]),
		nmsgs: uint32(len(msgs)),
	}

	//nolint:staticcheck // SA1019: unix.SYS_IOCTL is only "deprecated" on darwin (libSystem wrappers
	// preferred there); this code only ever runs on the Linux boards GoSD targets, where a raw
	// ioctl syscall is the normal, supported way to reach i2c-dev. Cross-GOOS false positive, same
	// pattern as netup/interfaces.go's nolint.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(i2cRdwr), uintptr(unsafe.Pointer(&data)))
	if errno != 0 {
		return 0, false
	}
	return resp[0], true
}

// i2cMsg and i2cRdwrIoctlData mirror the Linux kernel's struct i2c_msg and
// struct i2c_rdwr_ioctl_data (linux/i2c-dev.h, linux/i2c.h) field-for-field.
// The pointer fields are typed unsafe.Pointer (not uintptr): the kernel
// still sees a plain native-width pointer either way, but keeping them as
// unsafe.Pointer means Go's garbage collector can see and track the
// referenced buffers for as long as this struct is reachable, rather than
// risking a buffer being collected out from under an opaque integer.
// Native pointer width also makes this correct on both the 32-bit armv6
// pi-zero-w and every other (64-bit) board without any arch-specific code.
type i2cMsg struct {
	addr   uint16
	flags  uint16
	length uint16
	buf    unsafe.Pointer
}

type i2cRdwrIoctlData struct {
	msgs  unsafe.Pointer
	nmsgs uint32
}
