package artifacts

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

const (
	kernelContent = "pretend this is kernel8.img"
	dtbContent    = "pretend this is a dtb"
	testVersion   = "v9.9.9"
	testBoard     = "pi-zero-2w"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// tarZst builds a zstd-compressed tar archive containing files (name ->
// content), matching the flat layout build-artifacts.yml publishes.
func tarZst(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("creating zstd writer: %v", err)
	}
	tw := tar.NewWriter(zw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o644}); err != nil {
			t.Fatalf("writing tar header for %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("writing tar content for %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zstd writer: %v", err)
	}
	return buf.Bytes()
}

// testManifest returns a Manifest whose sole board is testBoard, with a
// files list matching the given contents map's sha256/size.
func testManifest(files map[string]string) Manifest {
	entries := make([]FileEntry, 0, len(files))
	for name, content := range files {
		entries = append(entries, FileEntry{Name: name, SHA256: sha256Hex(content), Size: int64(len(content))})
	}
	return Manifest{
		Version: testVersion,
		Boards: map[string]BoardFiles{
			testBoard: {Files: entries},
		},
	}
}

// releaseServer serves manifestJSON at /manifest.json and tarball at
// /<board>.tar.zst, recording how many requests it handled.
type releaseServer struct {
	*httptest.Server
	requests int
}

func newReleaseServer(t *testing.T, manifest Manifest, tarball []byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.requests++
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("encoding manifest response: %v", err)
			}
		case "/" + testBoard + ".tar.zst":
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	return rs
}

func (rs *releaseServer) urlFor(name string) string {
	return rs.URL + "/" + name
}

func TestEnsureBoardDownloadsVerifiesAndCaches(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent, "dtb.dtb": dtbContent}
	tarball := tarZst(t, files)
	manifest := testManifest(files)
	srv := newReleaseServer(t, manifest, tarball)
	defer srv.Close()

	cacheDir := t.TempDir()

	dir, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, srv.urlFor)
	if err != nil {
		t.Fatalf("ensureBoard: %v", err)
	}
	if want := filepath.Join(cacheDir, testVersion, testBoard); dir != want {
		t.Errorf("ensureBoard() dir = %q, want %q", dir, want)
	}

	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading extracted %s: %v", name, err)
		}
		if string(got) != content {
			t.Errorf("%s content = %q, want %q", name, got, content)
		}
	}

	if _, err := os.Stat(filepath.Join(cacheDir, testVersion, "manifest.json")); err != nil {
		t.Errorf("manifest.json was not cached: %v", err)
	}
}

func TestEnsureBoardCorruptedTarballFailsVerification(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent}
	// The manifest pins a hash for content that never matches what the
	// "tarball" actually contains — simulating a corrupted upload or a
	// tampered release.
	manifest := testManifest(files)
	corruptTarball := tarZst(t, map[string]string{"kernel8.img": "corrupted bytes, not " + kernelContent})
	srv := newReleaseServer(t, manifest, corruptTarball)
	defer srv.Close()

	cacheDir := t.TempDir()

	_, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, srv.urlFor)
	if err == nil {
		t.Fatal("ensureBoard() succeeded, want a checksum-verification error")
	}
	if !strings.Contains(err.Error(), "verification") && !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error = %q, want it to mention verification/checksum failure", err)
	}

	if _, statErr := os.Stat(filepath.Join(cacheDir, testVersion, testBoard)); !os.IsNotExist(statErr) {
		t.Errorf("board directory was left in place after a failed verification")
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, testVersion, "manifest.json")); !os.IsNotExist(statErr) {
		t.Errorf("manifest.json was cached despite the tarball failing verification")
	}
}

func TestEnsureBoardOfflineWithCacheSkipsNetwork(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent}
	tarball := tarZst(t, files)
	manifest := testManifest(files)
	srv := newReleaseServer(t, manifest, tarball)

	cacheDir := t.TempDir()

	if _, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, srv.urlFor); err != nil {
		t.Fatalf("first ensureBoard() (online): %v", err)
	}
	requestsAfterFirstCall := srv.requests

	srv.Close() // simulate going offline: any further request to srv.URL now fails to connect

	dir, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, srv.urlFor)
	if err != nil {
		t.Fatalf("second ensureBoard() (offline, cached) failed: %v", err)
	}
	if want := filepath.Join(cacheDir, testVersion, testBoard); dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	if srv.requests != requestsAfterFirstCall {
		t.Errorf("second call made %d more request(s) to the server; want the cache hit to touch the network 0 times", srv.requests-requestsAfterFirstCall)
	}
}

func TestEnsureBoardOfflineWithoutCacheIsActionable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	srv.Close() // closed before any successful call: nothing is cached, and the server is unreachable

	cacheDir := t.TempDir()

	_, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, func(name string) string {
		return srv.URL + "/" + name
	})
	if err == nil {
		t.Fatal("ensureBoard() succeeded, want an error")
	}
	for _, want := range []string{"offline", "--artifacts-dir"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestEnsureBoardRequiresABoardName(t *testing.T) {
	if _, err := ensureBoard(context.Background(), nil, t.TempDir(), "", testVersion, func(string) string { return "" }); err == nil {
		t.Fatal("ensureBoard() with an empty board name succeeded, want an error")
	}
}

func TestEnsureBoardUnknownBoardIsActionable(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent}
	manifest := testManifest(files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))
	defer srv.Close()

	_, err := ensureBoard(context.Background(), srv.Client(), t.TempDir(), "not-a-real-board", testVersion, srv.urlFor)
	if err == nil {
		t.Fatal("ensureBoard() for an unknown board succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "not-a-real-board") {
		t.Errorf("error = %q, want it to name the unknown board", err)
	}
}

func TestEnsureBoardRejectsUnsafeTarPaths(t *testing.T) {
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("creating zstd writer: %v", err)
	}
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: "../escape.img", Size: 4, Mode: 0o644}); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zstd writer: %v", err)
	}

	manifest := testManifest(map[string]string{"../escape.img": "evil"})
	srv := newReleaseServer(t, manifest, buf.Bytes())
	defer srv.Close()

	_, err = ensureBoard(context.Background(), srv.Client(), t.TempDir(), testBoard, testVersion, srv.urlFor)
	if err == nil {
		t.Fatal("ensureBoard() with a path-escaping tar entry succeeded, want an error")
	}
}

// roundTripFunc adapts a function into an http.RoundTripper, so the test
// below can assert on the request URL EnsureBoard builds without ever
// opening a real connection.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEnsureBoardProductionEntrypointTargetsThisRepoAtThePinnedVersion(t *testing.T) {
	wantPrefix := "https://github.com/jphastings/gosd/releases/download/artifacts/" + Version + "/"
	var gotURL string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		return nil, errors.New("network disabled in this test")
	})}

	if _, err := EnsureBoard(context.Background(), client, t.TempDir(), testBoard); err == nil {
		t.Fatal("EnsureBoard() succeeded, want an error (the test's transport always fails)")
	}
	if !strings.HasPrefix(gotURL, wantPrefix) {
		t.Errorf("EnsureBoard requested %q, want it to start with %q", gotURL, wantPrefix)
	}
}
