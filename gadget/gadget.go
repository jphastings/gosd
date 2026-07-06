// Package gadget presents a GoSD board as a USB peripheral (a "gadget") via
// Linux's configfs USB gadget API — no cgo, no exec, just directory
// creation, attribute-file writes, and symlinks under
// /sys/kernel/config/usb_gadget. See the kernel documentation
// Documentation/usb/gadget_configfs.rst for the on-disk layout this package
// materializes.
//
// A gosd app opts into gadget mode itself, at runtime, by constructing a
// Gadget and calling Apply(); gosd-init never does this on an app's behalf.
// The board image must also have been built with `gosd build --usb-gadget`
// so the board's USB controller is actually in peripheral mode by the time
// Apply runs (see internal/boards/pizero2w's dwc2 overlay) — without that,
// Apply's UDC bind step fails with an actionable error.
package gadget

import (
	"fmt"
)

// gadgetRoot is the configfs directory this package's gadget lives under.
// "gosd" is just this gadget's name within usb_gadget/ — configfs supports
// multiple named gadgets, but GoSD apps only ever need one.
const gadgetRoot = "/sys/kernel/config/usb_gadget/gosd"

// udcClassDir lists the board's USB peripheral controllers (dwc2 on Pi,
// dwc3 on Radxa) once one is bound and in peripheral mode. Apply binds to
// the first entry, matching the bean's locked decision — neither supported
// board ever exposes more than one.
const udcClassDir = "/sys/class/udc"

// Function is a USB gadget function that can be linked into a Gadget's
// config, such as ACM (CDC-ACM serial). It's implemented by this package's
// own function types and not designed to be implemented outside it: Create
// takes the unexported writableFS, so an external type can't satisfy this
// interface. Configfs function drivers are gosd's to define, not a
// third-party plugin surface.
type Function interface {
	// Name returns the configfs "functions/<name>" directory name, e.g.
	// "acm.usb0" — the "<type>.<instance>" form configfs's gadget driver
	// requires.
	Name() string
	// Create writes any attribute files this function type needs inside
	// dir (already created as functions/<Name()>) before it's linked into
	// a config. ACM needs none today.
	Create(fsys writableFS, dir string) error
}

// Gadget declaratively describes a USB peripheral identity and the
// functions it exposes. Apply materializes it; Close tears it down. The
// zero value is a valid, not-yet-applied Gadget.
type Gadget struct {
	VendorID, ProductID           uint16
	Manufacturer, Product, Serial string
	Functions                     []Function

	// fs and udc are set by Apply and cleared by Close; fs != nil marks
	// the gadget as currently applied (see the Apply guard below).
	fs  writableFS
	udc string
}

// Apply materializes the configfs tree for g under gadgetRoot, links every
// Function into config c.1, and binds the first available UDC — after this
// returns without error, the board is enumerating on its USB port. Apply
// fails if it's already been called without an intervening Close, since a
// silent re-apply would otherwise surface as a confusing kernel EBUSY deep
// inside a later write.
func (g *Gadget) Apply() error {
	return g.apply(osFS{})
}

// apply is Apply's real implementation, taking fsys as a parameter so tests
// can exercise it against the fake in fakes_test.go instead of the real
// filesystem.
func (g *Gadget) apply(fsys writableFS) error {
	if g.fs != nil {
		return fmt.Errorf("gadget: Apply called while already applied; call Close first")
	}
	if len(g.Functions) == 0 {
		return fmt.Errorf("gadget: Apply requires at least one Function")
	}

	if err := g.materialize(fsys); err != nil {
		return err
	}

	udc, err := firstUDC(fsys)
	if err != nil {
		return err
	}
	if err := fsys.WriteFile(gadgetRoot+"/UDC", []byte(udc+"\n"), 0o644); err != nil {
		return fmt.Errorf("gadget: binding UDC %q: %w", udc, err)
	}

	g.fs, g.udc = fsys, udc
	return nil
}

// materialize writes every configfs file/directory/symlink Apply needs,
// short of the final UDC bind (kept separate so tests can drive it against
// a fake without also needing a fake UDC most of the time).
func (g *Gadget) materialize(fsys writableFS) error {
	if err := fsys.MkdirAll(gadgetRoot, 0o755); err != nil {
		return fmt.Errorf("gadget: creating %s: %w", gadgetRoot, err)
	}

	writes := map[string]string{
		gadgetRoot + "/idVendor":  fmt.Sprintf("0x%04x\n", g.VendorID),
		gadgetRoot + "/idProduct": fmt.Sprintf("0x%04x\n", g.ProductID),
	}
	for path, content := range writes {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("gadget: writing %s: %w", path, err)
		}
	}

	if err := fsys.MkdirAll(gadgetRoot+"/strings/0x409", 0o755); err != nil {
		return fmt.Errorf("gadget: creating strings dir: %w", err)
	}
	strs := map[string]string{
		gadgetRoot + "/strings/0x409/manufacturer": g.Manufacturer,
		gadgetRoot + "/strings/0x409/product":      g.Product,
		gadgetRoot + "/strings/0x409/serialnumber": g.Serial,
	}
	for path, value := range strs {
		if err := fsys.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
			return fmt.Errorf("gadget: writing %s: %w", path, err)
		}
	}

	configuration := g.Product
	if configuration == "" {
		configuration = "gosd"
	}
	if err := fsys.MkdirAll(gadgetRoot+"/configs/c.1/strings/0x409", 0o755); err != nil {
		return fmt.Errorf("gadget: creating config dir: %w", err)
	}
	if err := fsys.WriteFile(gadgetRoot+"/configs/c.1/strings/0x409/configuration", []byte(configuration+"\n"), 0o644); err != nil {
		return fmt.Errorf("gadget: writing config description: %w", err)
	}
	if err := fsys.WriteFile(gadgetRoot+"/configs/c.1/MaxPower", []byte("250\n"), 0o644); err != nil {
		return fmt.Errorf("gadget: writing MaxPower: %w", err)
	}

	for _, fn := range g.Functions {
		funcDir := gadgetRoot + "/functions/" + fn.Name()
		if err := fsys.MkdirAll(funcDir, 0o755); err != nil {
			return fmt.Errorf("gadget: creating function dir %s: %w", funcDir, err)
		}
		if err := fn.Create(fsys, funcDir); err != nil {
			return fmt.Errorf("gadget: configuring function %s: %w", fn.Name(), err)
		}
		if err := fsys.Symlink(funcDir, gadgetRoot+"/configs/c.1/"+fn.Name()); err != nil {
			return fmt.Errorf("gadget: linking function %s into config c.1: %w", fn.Name(), err)
		}
	}

	return nil
}

// firstUDC returns the first entry under udcClassDir, the board's USB
// peripheral controller — sorted order, per Apply's doc comment.
func firstUDC(fsys writableFS) (string, error) {
	entries, err := fsys.ReadDir(udcClassDir)
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("gadget: no USB peripheral controller found under %s; is the board's USB port in peripheral mode? (Pi Zero 2W: build with --usb-gadget; Radxa Zero 3E: no flag needed)", udcClassDir)
	}
	return entries[0].Name(), nil
}

// Close unbinds the UDC and removes every directory/symlink Apply created.
// It's safe to call on a Gadget that was never applied (a no-op), matching
// io.Closer convention. If a removal step fails, Close still attempts every
// remaining step rather than stopping early, so a partial failure doesn't
// strand extra configfs state; it returns the first error encountered.
func (g *Gadget) Close() error {
	if g.fs == nil {
		return nil
	}
	fsys := g.fs

	var firstErr error
	fail := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Unbind before removing anything: the kernel refuses to tear down a
	// bound gadget's functions/configs out from under it.
	fail(fsys.WriteFile(gadgetRoot+"/UDC", []byte("\n"), 0o644))

	// Every directory removed below mirrors one materialize() created via
	// MkdirAll, in reverse (leaves first): MkdirAll silently creates
	// intermediate directories too (e.g. "configs" and "configs/c.1/
	// strings" alongside "configs/c.1/strings/0x409"), and each still has
	// to be rmdir'd individually.
	for _, fn := range g.Functions {
		fail(fsys.Remove(gadgetRoot + "/configs/c.1/" + fn.Name()))
	}
	fail(fsys.Remove(gadgetRoot + "/configs/c.1/strings/0x409"))
	fail(fsys.Remove(gadgetRoot + "/configs/c.1/strings"))
	fail(fsys.Remove(gadgetRoot + "/configs/c.1"))
	fail(fsys.Remove(gadgetRoot + "/configs"))
	for _, fn := range g.Functions {
		fail(fsys.Remove(gadgetRoot + "/functions/" + fn.Name()))
	}
	fail(fsys.Remove(gadgetRoot + "/functions"))
	fail(fsys.Remove(gadgetRoot + "/strings/0x409"))
	fail(fsys.Remove(gadgetRoot + "/strings"))
	fail(fsys.Remove(gadgetRoot))

	g.fs, g.udc = nil, ""
	return firstErr
}
