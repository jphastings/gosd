// Command spiloopback is a minimal example demonstrating raw SPI access on
// GoSD: it opens every /dev/spidev* character device present and performs a
// full-duplex SPI_IOC_MESSAGE transfer of a fixed test pattern, reporting
// whether the bytes it reads back match the bytes it sent.
//
// This only proves anything if MOSI is physically jumpered to MISO on the
// bus under test (a classic, wiring-only SPI self-test): with that jumper in
// place, whatever this example writes should immediately be looped back and
// read on the same transfer. Without the jumper, the transfer still
// succeeds (SPI has no acknowledgement, so an unconnected or wrongly-wired
// bus doesn't error) - it just reports a mismatch, which is expected in that
// case rather than a fault.
//
// It talks to the bus directly via the standard Linux spidev ioctl
// (SPI_IOC_MESSAGE) through golang.org/x/sys/unix, rather than a
// higher-level driver library, so it has no dependency on what (if
// anything) is attached. For real applications, start from periph.io
// instead - see docs/runtime.md's "GPIO, I2C, SPI" section.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	// spiIOCMagic and spiIOCNRTransfer are the standard Linux spidev ioctl
	// identifiers (linux/spi/spidev.h): magic number 'k' and the request
	// number for the "do a transfer" ioctl. golang.org/x/sys/unix wraps
	// generic syscalls, not device-specific ioctls like this one, so it's
	// computed here directly from the same _IOC() encoding the kernel
	// headers use, rather than hardcoding a magic constant that's only
	// valid for one specific message count.
	spiIOCMagic      = 0x6b
	spiIOCNRTransfer = 0

	// These mirror asm-generic/ioctl.h's _IOC() bit layout, used below to
	// compute the SPI_IOC_MESSAGE(N) request number for N=1 the same way
	// the kernel's own macro would.
	iocDirShift  = 30
	iocTypeShift = 8
	iocSizeShift = 16
	iocWrite     = 1

	// defaultSpeedHz is a conservative default clock speed: fast enough to
	// be useful, slow enough to work reliably on breadboard wiring and
	// short jumpers without needing per-board tuning.
	defaultSpeedHz = 500000

	// defaultBitsPerWord is the overwhelmingly common SPI word size; 0
	// in the ioctl struct means "let the driver use its default", which
	// is this on every controller GoSD targets.
	defaultBitsPerWord = 8
)

// testPattern is the fixed byte sequence written on every transfer. It
// deliberately isn't all-zero or all-0xff (both of which a floating,
// unconnected MISO line can mimic on some controllers), and it isn't
// symmetric, so a correctly-looped-back read is an unambiguous match.
var testPattern = []byte("GoSD SPI loopback self-test 0123456789 - jumper MOSI to MISO")

func main() {
	buses, err := filepath.Glob("/dev/spidev*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosd spiloopback: listing /dev/spidev*: %v\n", err)
		os.Exit(1)
	}
	if len(buses) == 0 {
		fmt.Println("gosd spiloopback: no /dev/spidev* device found - is SPI enabled for this board? see docs/runtime.md")
		return
	}
	sort.Strings(buses)

	exitCode := 0
	for _, bus := range buses {
		if !loopbackTest(bus) {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

// loopbackTest transfers testPattern full-duplex on bus and reports whether
// the bytes read back match the bytes sent, returning false only when the
// transfer itself failed (a mismatched read is reported, not treated as a
// failure, since it's the expected result when MOSI isn't jumpered to MISO).
func loopbackTest(bus string) bool {
	f, err := os.OpenFile(bus, os.O_RDWR, 0)
	if err != nil {
		fmt.Printf("%s: opening failed: %v\n", bus, err)
		return false
	}
	defer func() { _ = f.Close() }()

	rx := make([]byte, len(testPattern))
	if err := transfer(int(f.Fd()), testPattern, rx); err != nil {
		fmt.Printf("%s: SPI_IOC_MESSAGE transfer failed: %v\n", bus, err)
		return false
	}

	if bytes.Equal(rx, testPattern) {
		fmt.Printf("%s: loopback OK - %d bytes read back exactly as sent (MOSI is jumpered to MISO)\n", bus, len(testPattern))
	} else {
		fmt.Printf("%s: loopback mismatch - sent %q, got %q back (jumper MOSI to MISO on this bus to self-test)\n", bus, testPattern, rx)
	}
	return true
}

// transfer performs one full-duplex SPI_IOC_MESSAGE(1) ioctl, writing tx and
// filling rx with whatever the controller clocked in during the same
// transfer - full duplex, so both happen on the same clock edges, unlike a
// separate write() then read().
func transfer(fd int, tx, rx []byte) error {
	if len(tx) != len(rx) {
		return fmt.Errorf("tx and rx buffers must be the same length (%d vs %d)", len(tx), len(rx))
	}

	xfer := spiIOCTransfer{
		txBuf:       uint64(uintptr(unsafe.Pointer(&tx[0]))),
		rxBuf:       uint64(uintptr(unsafe.Pointer(&rx[0]))),
		length:      uint32(len(tx)),
		speedHz:     defaultSpeedHz,
		bitsPerWord: defaultBitsPerWord,
	}

	//nolint:staticcheck // SA1019: unix.SYS_IOCTL is only "deprecated" on darwin (libSystem
	// wrappers preferred there); this code only ever runs on the Linux boards GoSD targets,
	// where a raw ioctl syscall is the normal, supported way to reach spidev. Cross-GOOS false
	// positive, same pattern as netup/interfaces.go's nolint and examples/i2cscan's readChipID.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), spiIOCMessage(1), uintptr(unsafe.Pointer(&xfer)))
	// Unlike i2cscan's i2c_msg struct (whose buf fields are typed
	// unsafe.Pointer, so the GC already sees them), spi_ioc_transfer's
	// tx_buf/rx_buf are fixed-width __u64 in the real kernel ABI - always
	// 8 bytes, even on the 32-bit armv6 pi-zero-w - so the pointers above
	// had to go through uintptr first, which the GC can't trace back to
	// tx/rx. KeepAlive holds both reachable until the syscall (which
	// dereferences those addresses in the kernel) has returned.
	runtime.KeepAlive(tx)
	runtime.KeepAlive(rx)
	if errno != 0 {
		return errno
	}
	return nil
}

// spiIOCMessage computes the SPI_IOC_MESSAGE(n) ioctl request number for a
// transfer of n spi_ioc_transfer structs, the same way the kernel's own
// macro (linux/spi/spidev.h) does: _IOW(SPI_IOC_MAGIC, 0, char[n *
// sizeof(spi_ioc_transfer)]). It's a function of n, not a constant, because
// the request number encodes the payload size - this example only ever
// sends one message, but hardcoding "the ioctl number for one message"
// would silently need updating if that changed.
func spiIOCMessage(n int) uintptr {
	size := n * int(unsafe.Sizeof(spiIOCTransfer{}))
	return uintptr(iocWrite<<iocDirShift | spiIOCMagic<<iocTypeShift | spiIOCNRTransfer | size<<iocSizeShift)
}

// spiIOCTransfer mirrors the Linux kernel's struct spi_ioc_transfer
// (linux/spi/spidev.h) field-for-field, including its padding, so its size
// and layout exactly match what SPI_IOC_MESSAGE expects. tx_buf/rx_buf are
// plain u64 fields in the real struct (not typed pointers - the ioctl ABI
// is architecture-width-agnostic by design), so the pointers above are
// converted through uintptr/uint64 explicitly rather than stored as
// unsafe.Pointer fields here.
type spiIOCTransfer struct {
	txBuf       uint64
	rxBuf       uint64
	length      uint32
	speedHz     uint32
	delayUsecs  uint16
	bitsPerWord uint8
	csChange    uint8
	txNbits     uint8
	rxNbits     uint8
	wordDelay   uint8
	pad         uint8
}
