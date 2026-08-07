package cloudflaredpin

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestByGOARCHIsArm64Only pins the capability table's current shape (see the
// package doc comment: ByGOARCH IS the capability table cmd/gosd's
// --ingress validation consults). A regression here silently changes which
// boards gosd build --ingress cloudflared accepts.
func TestByGOARCHIsArm64Only(t *testing.T) {
	if _, ok := ByGOARCH["arm64"]; !ok {
		t.Error(`ByGOARCH["arm64"] is missing; --ingress cloudflared should support arm64 boards`)
	}
	if _, ok := ByGOARCH["arm"]; ok {
		t.Error(`ByGOARCH["arm"] is present; the official arm release is GOARM=7 and faults on pi-zero-w's armv6 (see package doc comment) - it must have no entry`)
	}
	if got, want := len(ByGOARCH), 1; got != want {
		t.Errorf("len(ByGOARCH) = %d, want %d; update this test if a new GOARCH is intentionally added", got, want)
	}
}

// TestArm64ArtifactIsWellFormed checks every field a fetch.ToDir call and an
// --artifacts-dir override actually depend on: a non-empty URL naming
// Version, a valid lowercase-hex SHA-256, and a non-empty Name.
func TestArm64ArtifactIsWellFormed(t *testing.T) {
	art, ok := ByGOARCH["arm64"]
	if !ok {
		t.Fatal(`ByGOARCH["arm64"] is missing`)
	}

	if art.Name == "" {
		t.Error("Name is empty; it's the --artifacts-dir well-known override file name")
	}
	if art.URL == "" {
		t.Error("URL is empty")
	}
	if Version == "" {
		t.Fatal("Version is empty")
	}
	if got := art.URL; !strings.Contains(got, Version) {
		t.Errorf("URL = %q, want it to reference the pinned Version %q", got, Version)
	}

	raw, err := hex.DecodeString(art.SHA256)
	if err != nil {
		t.Fatalf("SHA256 %q is not valid hex: %v", art.SHA256, err)
	}
	if len(raw) != 32 {
		t.Errorf("SHA256 %q decodes to %d bytes, want 32 (a SHA-256 digest)", art.SHA256, len(raw))
	}
}
