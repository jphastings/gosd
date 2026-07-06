package gadget

import (
	"fmt"
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
	if len(f.callsOfKind("write")) == 0 {
		t.Fatal("expected identity/function files to be written before the UDC lookup fails")
	}
	for _, p := range f.callsOfKind("write") {
		if p == gadgetRoot+"/UDC" {
			t.Errorf("UDC should never be written when no controller is present")
		}
	}
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
