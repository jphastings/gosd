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
	"regexp"
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

// manifestFor returns the published bytes of a manifest whose sole board is
// testBoard, listing the given contents map's sha256/size — the release
// asset itself, since it is the asset's bytes (not a re-encoding of them)
// that the pinned digest anchors.
func manifestFor(t *testing.T, files map[string]string) []byte {
	t.Helper()

	entries := make([]FileEntry, 0, len(files))
	for name, content := range files {
		entries = append(entries, FileEntry{Name: name, SHA256: sha256Hex(content), Size: int64(len(content))})
	}
	data, err := json.Marshal(Manifest{
		Version: testVersion,
		Boards:  map[string]BoardFiles{testBoard: {Files: entries}},
	})
	if err != nil {
		t.Fatalf("encoding manifest: %v", err)
	}
	return data
}

// releaseServer serves manifest at /manifest.json and tarball at
// /<board>.tar.zst, recording how many requests it handled.
type releaseServer struct {
	*httptest.Server
	requests int
}

func newReleaseServer(t *testing.T, manifest, tarball []byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.requests++
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(manifest)
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
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))
	defer srv.Close()

	cacheDir := t.TempDir()

	dir, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor)
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

	cached, err := os.ReadFile(filepath.Join(cacheDir, testVersion, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest.json was not cached: %v", err)
	}
	if !bytes.Equal(cached, manifest) {
		t.Error("the cached manifest.json is not the published bytes, so it can never be re-checked against the pinned digest")
	}
}

func TestEnsureBoardCorruptedTarballFailsVerification(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent}
	// The manifest pins a hash for content that never matches what the
	// "tarball" actually contains — simulating a corrupted upload or a
	// tampered release.
	manifest := manifestFor(t, files)
	corruptTarball := tarZst(t, map[string]string{"kernel8.img": "corrupted bytes, not " + kernelContent})
	srv := newReleaseServer(t, manifest, corruptTarball)
	defer srv.Close()

	cacheDir := t.TempDir()

	_, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor)
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
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))

	cacheDir := t.TempDir()

	if _, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor); err != nil {
		t.Fatalf("first ensureBoard() (online): %v", err)
	}
	requestsAfterFirstCall := srv.requests

	srv.Close() // simulate going offline: any further request to srv.URL now fails to connect

	dir, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor)
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

// poisonCache does what a compromised editor extension or postinstall script
// with write access to the user's cache directory can do: replace a cached
// artifact with a backdoored one, and rewrite the cached manifest.json so it
// vouches for the replacement. The pair is internally consistent — only the
// digest compiled into gosd can tell it apart from the real release.
func poisonCache(t *testing.T, cacheDir, backdoor string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(cacheDir, testVersion, testBoard, "kernel8.img"), []byte(backdoor), 0o644); err != nil {
		t.Fatalf("planting the backdoored kernel: %v", err)
	}
	poisoned := manifestFor(t, map[string]string{"kernel8.img": backdoor})
	if err := os.WriteFile(filepath.Join(cacheDir, testVersion, "manifest.json"), poisoned, 0o644); err != nil {
		t.Fatalf("planting the poisoned manifest: %v", err)
	}
}

func TestEnsureBoardRefetchesWhenTheCachedManifestWasTamperedWith(t *testing.T) {
	const backdoor = "backdoored kernel"
	files := map[string]string{"kernel8.img": kernelContent}
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))
	defer srv.Close()

	cacheDir := t.TempDir()
	if _, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor); err != nil {
		t.Fatalf("first ensureBoard(): %v", err)
	}
	poisonCache(t, cacheDir, backdoor)

	dir, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor)
	if err != nil {
		t.Fatalf("ensureBoard() after cache tampering: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "kernel8.img"))
	if err != nil {
		t.Fatalf("reading kernel8.img: %v", err)
	}
	if string(got) == backdoor {
		t.Fatal("ensureBoard() returned the backdoored kernel: a self-consistent poisoned manifest in the cache was trusted")
	}
	if string(got) != kernelContent {
		t.Errorf("kernel8.img = %q, want the genuine %q", got, kernelContent)
	}

	cached, err := os.ReadFile(filepath.Join(cacheDir, testVersion, "manifest.json"))
	if err != nil {
		t.Fatalf("reading the cached manifest: %v", err)
	}
	if !bytes.Equal(cached, manifest) {
		t.Error("the poisoned manifest is still cached after a successful re-download")
	}
}

func TestEnsureBoardOfflineRefusesATamperedCacheRatherThanTrustingIt(t *testing.T) {
	const backdoor = "backdoored kernel"
	files := map[string]string{"kernel8.img": kernelContent}
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))

	cacheDir := t.TempDir()
	if _, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor); err != nil {
		t.Fatalf("first ensureBoard(): %v", err)
	}
	poisonCache(t, cacheDir, backdoor)
	srv.Close() // the re-download the tampering forces is not available

	if _, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor); err == nil {
		t.Fatal("ensureBoard() succeeded offline from a tampered cache, want an error")
	}
}

func TestEnsureBoardRejectsAManifestThatIsNotThePinnedOne(t *testing.T) {
	files := map[string]string{"kernel8.img": "some other release's kernel"}
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))
	defer srv.Close()

	cacheDir := t.TempDir()

	// The digest gosd was built with belongs to a different manifest: what
	// the server offers may be a substituted release or an intercepted
	// download, and either way it isn't the one this binary trusts.
	_, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex("the manifest this gosd was built against"), srv.urlFor)
	if err == nil {
		t.Fatal("ensureBoard() accepted a manifest that doesn't match the pinned digest")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %q, want it to report a checksum mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, testVersion, "manifest.json")); !os.IsNotExist(statErr) {
		t.Error("an unverified manifest was cached")
	}
}

func TestEnsureBoardRefusesWhenNoManifestDigestIsPinned(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent}
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))
	defer srv.Close()

	_, err := ensureBoard(context.Background(), srv.Client(), t.TempDir(), testBoard, testVersion, "", srv.urlFor)
	if err == nil {
		t.Fatal("ensureBoard() with no pinned manifest digest succeeded, want it to fail closed")
	}
	for _, want := range []string{"ManifestSHA256", "--artifacts-dir"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
	if srv.requests != 0 {
		t.Errorf("server received %d request(s); want an unverifiable download not to be attempted at all", srv.requests)
	}
}

func TestPinnedManifestDigestAccompaniesTheVersion(t *testing.T) {
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(ManifestSHA256) {
		t.Errorf("ManifestSHA256 = %q, want the lowercase-hex sha256 of artifacts/%s's manifest.json "+
			"(curl -sfL https://github.com/jphastings/gosd/releases/download/artifacts/%s/manifest.json | shasum -a 256)",
			ManifestSHA256, Version, Version)
	}
}

func TestFetchTarballRefusesAnArchiveThatKeepsExpanding(t *testing.T) {
	// One entry far larger than the cap: a compressed archive that expands
	// without bound would otherwise fill the disk before any digest in the
	// manifest gets a chance to reject its contents.
	tarball := tarZst(t, map[string]string{"kernel8.img": strings.Repeat("x", 4096)})
	srv := newReleaseServer(t, manifestFor(t, nil), tarball)
	defer srv.Close()

	destDir := t.TempDir()
	err := fetchTarball(context.Background(), srv.Client(), srv.urlFor(testBoard+".tar.zst"), destDir, 1024)
	if err == nil {
		t.Fatal("fetchTarball() extracted an archive larger than its cap, want an error")
	}
	if !strings.Contains(err.Error(), "expands to more than") {
		t.Errorf("error = %q, want it to explain that the archive expands past the cap", err)
	}
}

func TestEnsureBoardOfflineWithoutCacheIsActionable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	srv.Close() // closed before any successful call: nothing is cached, and the server is unreachable

	cacheDir := t.TempDir()

	_, err := ensureBoard(context.Background(), srv.Client(), cacheDir, testBoard, testVersion, sha256Hex("any pin"), func(name string) string {
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
	if _, err := ensureBoard(context.Background(), nil, t.TempDir(), "", testVersion, sha256Hex("any pin"), func(string) string { return "" }); err == nil {
		t.Fatal("ensureBoard() with an empty board name succeeded, want an error")
	}
}

func TestEnsureBoardUnknownBoardIsActionable(t *testing.T) {
	files := map[string]string{"kernel8.img": kernelContent}
	manifest := manifestFor(t, files)
	srv := newReleaseServer(t, manifest, tarZst(t, files))
	defer srv.Close()

	_, err := ensureBoard(context.Background(), srv.Client(), t.TempDir(), "not-a-real-board", testVersion, sha256Hex(string(manifest)), srv.urlFor)
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

	manifest := manifestFor(t, map[string]string{"../escape.img": "evil"})
	srv := newReleaseServer(t, manifest, buf.Bytes())
	defer srv.Close()

	_, err = ensureBoard(context.Background(), srv.Client(), t.TempDir(), testBoard, testVersion, sha256Hex(string(manifest)), srv.urlFor)
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
