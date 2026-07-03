package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const content = "bootcode.bin contents, pretend this is a firmware blob"

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestToDir_DownloadsAndVerifies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	f := File{URL: srv.URL, SHA256: sha256Hex(content)}

	path, err := ToDir(context.Background(), srv.Client(), f, cacheDir, "bootcode.bin")
	if err != nil {
		t.Fatalf("ToDir() error = %v", err)
	}
	if path != filepath.Join(cacheDir, "bootcode.bin") {
		t.Errorf("ToDir() path = %q, want %q", path, filepath.Join(cacheDir, "bootcode.bin"))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != content {
		t.Errorf("downloaded content = %q, want %q", got, content)
	}
}

func TestToDir_ChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	f := File{URL: srv.URL, SHA256: sha256Hex("this is not the content that will be served")}

	_, err := ToDir(context.Background(), srv.Client(), f, cacheDir, "bootcode.bin")
	if err == nil {
		t.Fatal("ToDir() error = nil, want checksum mismatch error")
	}

	dest := filepath.Join(cacheDir, "bootcode.bin")
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("corrupted download was left at %s; want it removed", dest)
	}

	entries, readErr := os.ReadDir(cacheDir)
	if readErr != nil {
		t.Fatalf("reading cache dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("cache dir has %d leftover entries after a checksum mismatch, want 0: %v", len(entries), entries)
	}
}

func TestToDir_CacheHitSkipsNetwork(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(content))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	f := File{URL: srv.URL, SHA256: sha256Hex(content)}

	if _, err := ToDir(context.Background(), srv.Client(), f, cacheDir, "bootcode.bin"); err != nil {
		t.Fatalf("first ToDir() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls after first fetch = %d, want 1", calls)
	}

	if _, err := ToDir(context.Background(), srv.Client(), f, cacheDir, "bootcode.bin"); err != nil {
		t.Fatalf("second ToDir() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("calls after cached fetch = %d, want 1 (network should not be hit)", calls)
	}
}

func TestToDir_StaleCacheIsReplaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	dest := filepath.Join(cacheDir, "bootcode.bin")
	if err := os.WriteFile(dest, []byte("stale, wrong content"), 0o644); err != nil {
		t.Fatalf("seeding stale cache file: %v", err)
	}

	f := File{URL: srv.URL, SHA256: sha256Hex(content)}
	if _, err := ToDir(context.Background(), srv.Client(), f, cacheDir, "bootcode.bin"); err != nil {
		t.Fatalf("ToDir() error = %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading refreshed file: %v", err)
	}
	if string(got) != content {
		t.Errorf("cache file content = %q, want refreshed content %q", got, content)
	}
}

func TestToDir_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	f := File{URL: srv.URL, SHA256: sha256Hex(content)}

	if _, err := ToDir(context.Background(), srv.Client(), f, cacheDir, "bootcode.bin"); err == nil {
		t.Fatal("ToDir() error = nil, want error for HTTP 404")
	}
}

func TestToDir_RequiresPinnedChecksum(t *testing.T) {
	cacheDir := t.TempDir()
	f := File{URL: "http://example.invalid/bootcode.bin"}

	if _, err := ToDir(context.Background(), nil, f, cacheDir, "bootcode.bin"); err == nil {
		t.Fatal("ToDir() error = nil, want error for missing checksum")
	}
}
