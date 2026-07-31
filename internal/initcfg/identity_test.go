package initcfg

import "testing"

func TestComputeIdentityIsOrderIndependent(t *testing.T) {
	files := []PayloadFile{
		{Path: "kernel8.img", Content: []byte("kernel bytes")},
		{Path: "config.txt", Content: []byte("dtparam=spi=on")},
		{Path: InitramfsPayloadPath("/init"), Content: []byte("init binary")},
		{Path: InitramfsPayloadPath("/app"), Content: []byte("app binary")},
	}
	reversed := make([]PayloadFile, len(files))
	for i, f := range files {
		reversed[len(files)-1-i] = f
	}

	got, want := ComputeIdentity(reversed), ComputeIdentity(files)
	if got != want {
		t.Errorf("ComputeIdentity is sensitive to input order: %q (reversed) != %q (forward)", got, want)
	}
}

func TestComputeIdentityIsDeterministic(t *testing.T) {
	files := []PayloadFile{
		{Path: "kernel8.img", Content: []byte("kernel bytes")},
		{Path: InitramfsPayloadPath("/app"), Content: []byte("app binary")},
	}
	first := ComputeIdentity(files)
	second := ComputeIdentity(files)
	if first != second {
		t.Errorf("ComputeIdentity(%v) = %q then %q, want the same value both times", files, first, second)
	}
}

func TestComputeIdentityChangesWithContent(t *testing.T) {
	base := []PayloadFile{{Path: "app", Content: []byte("v1")}}
	changed := []PayloadFile{{Path: "app", Content: []byte("v2")}}

	if ComputeIdentity(base) == ComputeIdentity(changed) {
		t.Error("ComputeIdentity did not change when a file's content changed")
	}
}

func TestComputeIdentityChangesWithPath(t *testing.T) {
	base := []PayloadFile{{Path: "app", Content: []byte("same content")}}
	renamed := []PayloadFile{{Path: "different-name", Content: []byte("same content")}}

	if ComputeIdentity(base) == ComputeIdentity(renamed) {
		t.Error("ComputeIdentity did not change when a file's path changed")
	}
}

// TestComputeIdentityLengthPrefixingAvoidsAmbiguity guards the reason
// ComputeIdentity length-prefixes each field instead of concatenating
// path+content directly: without that, {Path: "ab", Content: "c"} and
// {Path: "a", Content: "bc"} would hash identically.
func TestComputeIdentityLengthPrefixingAvoidsAmbiguity(t *testing.T) {
	a := []PayloadFile{{Path: "ab", Content: []byte("c")}}
	b := []PayloadFile{{Path: "a", Content: []byte("bc")}}

	if ComputeIdentity(a) == ComputeIdentity(b) {
		t.Error("ComputeIdentity(ab|c) == ComputeIdentity(a|bc); path/content boundary is ambiguous")
	}
}

func TestComputeIdentityOfEmptySetIsStable(t *testing.T) {
	first := ComputeIdentity(nil)
	second := ComputeIdentity([]PayloadFile{})
	if first != second {
		t.Errorf("ComputeIdentity(nil) = %q, ComputeIdentity([]PayloadFile{}) = %q, want equal", first, second)
	}
}

func TestInitramfsPayloadPathPrefixesTheArchivePath(t *testing.T) {
	if got, want := InitramfsPayloadPath("/init"), "initramfs:/init"; got != want {
		t.Errorf("InitramfsPayloadPath(%q) = %q, want %q", "/init", got, want)
	}
}
