package gadget

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

// stubFunction is a minimal Function used to exercise Apply/Close with more
// than one function linked into the config, without depending on ACM's
// specific behavior.
type stubFunction struct {
	name    string
	created bool
}

func (s *stubFunction) Name() string { return s.name }
func (s *stubFunction) Create(_ writableFS, _ string) error {
	s.created = true
	return nil
}

// failingFunction's Create always errors, so tests can force materialize()
// to fail partway through a multi-function gadget.
type failingFunction struct{ name string }

func (s *failingFunction) Name() string { return s.name }
func (s *failingFunction) Create(_ writableFS, _ string) error {
	return fmt.Errorf("stub: Create fails")
}

func seedUDC(f *fakeFS, name string) {
	_ = f.MkdirAll(udcClassDir, 0o755)
	_ = f.WriteFile(udcClassDir+"/"+name, nil, 0o644)
}

func testGadget(fns ...Function) *Gadget {
	return &Gadget{
		VendorID:     0x0525,
		ProductID:    0xa4a7,
		Manufacturer: "GoSD",
		Product:      "GoSD USB Serial",
		Serial:       "0001",
		Functions:    fns,
	}
}

func applyWithFake(t *testing.T, g *Gadget, f *fakeFS) error {
	t.Helper()
	return g.apply(f)
}

func TestApplyWritesGadgetIdentity(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	want := map[string]string{
		gadgetRoot + "/idVendor":                                "0x0525\n",
		gadgetRoot + "/idProduct":                               "0xa4a7\n",
		gadgetRoot + "/strings/0x409/manufacturer":              "GoSD\n",
		gadgetRoot + "/strings/0x409/product":                   "GoSD USB Serial\n",
		gadgetRoot + "/strings/0x409/serialnumber":              "0001\n",
		gadgetRoot + "/configs/c.1/strings/0x409/configuration": "GoSD USB Serial\n",
		gadgetRoot + "/configs/c.1/MaxPower":                    "250\n",
		gadgetRoot + "/UDC":                                     "20980000.usb\n",
	}
	for path, want := range want {
		got, ok := f.files[path]
		if !ok {
			t.Errorf("file %s was never written", path)
			continue
		}
		if string(got) != want {
			t.Errorf("file %s = %q, want %q", path, got, want)
		}
	}
}

func TestApplyCreatesAndLinksFunction(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	funcDir := gadgetRoot + "/functions/acm.usb0"
	if !f.dirs[funcDir] {
		t.Errorf("function directory %s was not created", funcDir)
	}
	link := gadgetRoot + "/configs/c.1/acm.usb0"
	target, ok := f.links[link]
	if !ok {
		t.Fatalf("symlink %s was not created", link)
	}
	if target != funcDir {
		t.Errorf("symlink %s -> %q, want %q", link, target, funcDir)
	}
}

func TestApplyWithMultipleFunctions(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	a := &stubFunction{name: "stub.a"}
	b := &stubFunction{name: "stub.b"}
	g := testGadget(a, b)

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	if !a.created || !b.created {
		t.Errorf("both functions should have Create called; a=%v b=%v", a.created, b.created)
	}
	for _, name := range []string{"stub.a", "stub.b"} {
		if !f.dirs[gadgetRoot+"/functions/"+name] {
			t.Errorf("function directory for %s was not created", name)
		}
		if _, ok := f.links[gadgetRoot+"/configs/c.1/"+name]; !ok {
			t.Errorf("symlink for %s was not created", name)
		}
	}
}

func TestApplyFailsWithNoFunctions(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget()

	err := applyWithFake(t, g, f)
	if err == nil {
		t.Fatal("Apply() = nil, want error for zero Functions")
	}
}

func TestApplyFailsWithNoUDC(t *testing.T) {
	f := newFakeFS()
	g := testGadget(ACM{})

	err := applyWithFake(t, g, f)
	if err == nil {
		t.Fatal("Apply() = nil, want error when no UDC is present")
	}
	if !errors.Is(err, ErrNoController) {
		t.Errorf("Apply() error = %v, want it to wrap ErrNoController so callers can errors.Is and degrade gracefully", err)
	}
	if len(f.callsOfKind("write")) == 0 {
		t.Fatal("expected identity/function files to be written before the UDC lookup fails")
	}
	for _, p := range f.callsOfKind("write") {
		if p == gadgetRoot+"/UDC" {
			t.Errorf("UDC should never be written when no controller is present")
		}
	}
}

// assertNoGadgetState fails t if any file, symlink or directory under
// gadgetRoot survives in f — used to confirm a failed Apply's unwind left no
// configfs state behind (gosd-0r40).
func assertNoGadgetState(t *testing.T, f *fakeFS) {
	t.Helper()
	if f.dirs[gadgetRoot] {
		t.Errorf("gadget root %s still exists", gadgetRoot)
	}
	for path := range f.files {
		if strings.HasPrefix(path, gadgetRoot) {
			t.Errorf("file %s still exists", path)
		}
	}
	for path := range f.links {
		if strings.HasPrefix(path, gadgetRoot) {
			t.Errorf("symlink %s still exists", path)
		}
	}
	for path := range f.dirs {
		if strings.HasPrefix(path, gadgetRoot+"/") {
			t.Errorf("directory %s still exists", path)
		}
	}
}

// TestApplyUnwindsOnMissingUDC is the regression test for gosd-0r40: a
// failed Apply (here, failing at the UDC step because no controller is
// present) must leave no configfs state behind, so a second Apply — once the
// underlying condition is fixed — succeeds instead of hitting EEXIST on the
// function symlink materialize() re-creates.
func TestApplyUnwindsOnMissingUDC(t *testing.T) {
	f := newFakeFS()
	g := testGadget(ACM{})

	if err := applyWithFake(t, g, f); err == nil {
		t.Fatal("Apply() = nil, want error when no UDC is present")
	}
	assertNoGadgetState(t, f)

	seedUDC(f, "20980000.usb")
	g2 := testGadget(ACM{})
	if err := applyWithFake(t, g2, f); err != nil {
		t.Fatalf("second Apply() after unwind = %v, want nil", err)
	}
}

// TestApplyUnwindsPartialMaterializeFailure covers the other failure shape
// the bean calls out: materialize() itself failing mid-tree (here, the
// second of two functions failing to Create after the first was fully
// created and linked) must unwind just as cleanly as a UDC-step failure.
func TestApplyUnwindsPartialMaterializeFailure(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(&stubFunction{name: "stub.a"}, &failingFunction{name: "stub.b"})

	if err := applyWithFake(t, g, f); err == nil {
		t.Fatal("Apply() = nil, want error when a Function fails to Create")
	}
	assertNoGadgetState(t, f)
}

func TestApplyTwiceWithoutCloseFails(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("first Apply() = %v, want nil", err)
	}

	if err := applyWithFake(t, g, f); err == nil {
		t.Fatal("Apply() a second time without Close = nil, want error")
	}
}

func TestCloseBeforeApplyIsNoOp(t *testing.T) {
	g := testGadget(ACM{})
	if err := g.Close(); err != nil {
		t.Fatalf("Close() before Apply() = %v, want nil", err)
	}
}

func TestCloseUnbindsBeforeRemoving(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	unbindWrite := f.indexOfCall("write", gadgetRoot+"/UDC")
	firstRemove := -1
	for i, c := range f.calls {
		if c.kind == "remove" {
			firstRemove = i
			break
		}
	}
	if unbindWrite == -1 || firstRemove == -1 || unbindWrite > firstRemove {
		t.Errorf("expected the final UDC write (unbind) at index %d to precede the first remove at index %d", unbindWrite, firstRemove)
	}
}

func TestCloseRemovesEverythingApplyCreated(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	if f.dirs[gadgetRoot] {
		t.Errorf("gadget root %s still exists after Close()", gadgetRoot)
	}
	for path := range f.files {
		if strings.HasPrefix(path, gadgetRoot) {
			t.Errorf("file %s still exists after Close()", path)
		}
	}
	for path := range f.links {
		if strings.HasPrefix(path, gadgetRoot) {
			t.Errorf("symlink %s still exists after Close()", path)
		}
	}
}

// configs/, functions/ and strings/ under the gadget, and strings/ under a
// config, are configfs default groups the kernel creates alongside their
// parent — a direct rmdir on one fails (EPERM in production). Close must
// remove only the user-created nodes, in the canonical teardown order, and
// let removing each default group's parent tear it down for free
// (gosd-cjs2).
func TestCloseRemovesOnlyUserCreatedNodesInCanonicalOrder(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	want := []string{
		gadgetRoot + "/configs/c.1/acm.usb0",
		gadgetRoot + "/configs/c.1/strings/0x409",
		gadgetRoot + "/configs/c.1",
		gadgetRoot + "/functions/acm.usb0",
		gadgetRoot + "/strings/0x409",
		gadgetRoot,
	}
	got := f.callsOfKind("remove")
	if len(got) != len(want) {
		t.Fatalf("Close() issued removes %v, want %v", got, want)
	}
	for i, path := range want {
		if got[i] != path {
			t.Errorf("remove[%d] = %s, want %s", i, got[i], path)
		}
	}
}

// A Close whose teardown fails leaves the gadget marked applied, because it
// is: the configfs state Apply created outlived the attempt to remove it.
func TestFailedCloseKeepsTheGadgetApplied(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	f.refuse[gadgetRoot] = errors.New("device or resource busy")
	if err := g.Close(); err == nil {
		t.Fatal("Close() = nil, want the error from the refused teardown")
	}

	if err := applyWithFake(t, g, f); err == nil {
		t.Fatal("Apply() after a failed Close = nil, want a refusal: the gadget's configfs state is still there")
	}

	delete(f.refuse, gadgetRoot)
	if err := g.Close(); err != nil {
		t.Fatalf("retried Close() = %v, want nil once the kernel stops refusing", err)
	}
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() after a successful retry = %v, want nil", err)
	}
}

// Apply's contract is that a failure leaves nothing behind and needs no
// Close, so an unwind that cannot hold up its end has to say so rather than
// let the caller believe a clean slate it doesn't have.
func TestApplyReportsAnUnwindItCouldNotComplete(t *testing.T) {
	f := newFakeFS()
	f.refuse[gadgetRoot] = errors.New("device or resource busy")
	g := testGadget(ACM{})

	err := applyWithFake(t, g, f)
	if err == nil {
		t.Fatal("Apply() = nil, want the no-UDC error")
	}
	if !strings.Contains(err.Error(), udcClassDir) {
		t.Errorf("Apply() = %q, want it to still lead with the failure the caller has to act on", err)
	}
	if !strings.Contains(err.Error(), "device or resource busy") {
		t.Errorf("Apply() = %q, want it to also report the unwind it could not complete", err)
	}
}

func TestCloseThenApplyRoundTrips(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(ACM{})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("first Apply() = %v, want nil", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("second Apply() = %v, want nil", err)
	}

	if !f.dirs[gadgetRoot+"/functions/acm.usb0"] {
		t.Error("re-Apply() did not recreate the function directory")
	}
}

func TestFirstUDCReturnsLowestSortedEntry(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "b-controller")
	seedUDC(f, "a-controller")

	got, err := firstUDC(f)
	if err != nil {
		t.Fatalf("firstUDC() = %v, want nil", err)
	}
	if got != "a-controller" {
		t.Errorf("firstUDC() = %q, want %q", got, "a-controller")
	}
}

func TestApplyDefaultsConfigurationWhenProductEmpty(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := &Gadget{VendorID: 1, ProductID: 2, Functions: []Function{ACM{}}}

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	got := string(f.files[gadgetRoot+"/configs/c.1/strings/0x409/configuration"])
	if got != "gosd\n" {
		t.Errorf("configuration = %q, want %q", got, "gosd\n")
	}
}

// TestFakeFSSymlinkFailsOnExistingTarget pins fakeFS.Symlink's os.Symlink-
// matching EEXIST behavior (gosd-0r40's test prerequisite): without it, a
// second Symlink call to a path a stranded prior Apply already linked would
// silently overwrite instead of failing the way real configfs does, making
// the unwind regression tests above unable to catch a re-Apply hitting
// EEXIST.
func TestFakeFSSymlinkFailsOnExistingTarget(t *testing.T) {
	f := newFakeFS()
	if err := f.MkdirAll("/some/dir", 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v, want nil", err)
	}
	if err := f.Symlink("/target", "/some/dir/link"); err != nil {
		t.Fatalf("first Symlink() = %v, want nil", err)
	}

	err := f.Symlink("/other-target", "/some/dir/link")
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("second Symlink() to the same path = %v, want error wrapping fs.ErrExist", err)
	}
	if got := f.links["/some/dir/link"]; got != "/target" {
		t.Errorf("failed second Symlink() overwrote the link target: got %q, want %q", got, "/target")
	}
}

func ExampleGadget() {
	g := Gadget{
		VendorID:     0x0525,
		ProductID:    0xa4a7,
		Manufacturer: "GoSD",
		Product:      "GoSD USB Serial",
		Functions:    []Function{ACM{}},
	}
	fmt.Println(g.Functions[0].Name())
	// Output: acm.usb0
}
