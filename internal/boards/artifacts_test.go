package boards_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestResolveArtifactsPrefersArtifactsDirOverFetching(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kernel8.img"), []byte("fake kernel"), 0o644); err != nil {
		t.Fatalf("seeding fake artifact: %v", err)
	}

	refs := []boards.ArtifactRef{
		{Name: "kernel8.img", URL: "http://example.invalid/kernel8.img", SHA256: "0000"},
	}
	art, err := boards.ResolveArtifacts(context.Background(), "pi-zero-2w", refs, dir, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	r, err := art.Open("kernel8.img")
	if err != nil {
		t.Fatalf("Open(kernel8.img): %v", err)
	}
	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}
	if string(got) != "fake kernel" {
		t.Errorf("artifact content = %q, want the local --artifacts-dir file's content", got)
	}
}

func TestResolveArtifactsFetchesWhenNotFoundLocally(t *testing.T) {
	const content = "firmware bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	refs := []boards.ArtifactRef{{Name: "start.elf", URL: srv.URL, SHA256: sha256Hex(content)}}
	art, err := boards.ResolveArtifacts(context.Background(), "pi-zero-2w", refs, "", t.TempDir(), nil)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	r, err := art.Open("start.elf")
	if err != nil {
		t.Fatalf("Open(start.elf): %v", err)
	}
	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}
	if string(got) != content {
		t.Errorf("artifact content = %q, want %q", got, content)
	}
}

func TestResolveArtifactsWithNoURLNoLocalFileAndNoFallbackIsActionable(t *testing.T) {
	refs := []boards.ArtifactRef{{Name: "kernel8.img"}}

	_, err := boards.ResolveArtifacts(context.Background(), "pi-zero-2w", refs, t.TempDir(), t.TempDir(), nil)
	if err == nil {
		t.Fatal("ResolveArtifacts() succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--artifacts-dir") {
		t.Errorf("error = %q, want it to mention --artifacts-dir", err)
	}
}

func TestResolveArtifactsFallsBackToBoardArtifactsFuncForRefsWithNoURL(t *testing.T) {
	boardDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(boardDir, "Image"), []byte("ci-built kernel"), 0o644); err != nil {
		t.Fatalf("seeding fake CI-built artifact: %v", err)
	}

	var gotBoard, gotCacheDir string
	fetchBoardArtifacts := func(_ context.Context, cacheDir, board string) (string, error) {
		gotBoard, gotCacheDir = board, cacheDir
		return boardDir, nil
	}

	cacheDir := t.TempDir()
	refs := []boards.ArtifactRef{{Name: "Image"}}
	art, err := boards.ResolveArtifacts(context.Background(), "radxa-zero-3e", refs, "", cacheDir, fetchBoardArtifacts)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	if gotBoard != "radxa-zero-3e" {
		t.Errorf("fetchBoardArtifacts was called with board = %q, want radxa-zero-3e", gotBoard)
	}
	if gotCacheDir != cacheDir {
		t.Errorf("fetchBoardArtifacts was called with cacheDir = %q, want %q", gotCacheDir, cacheDir)
	}

	r, err := art.Open("Image")
	if err != nil {
		t.Fatalf("Open(Image): %v", err)
	}
	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}
	if string(got) != "ci-built kernel" {
		t.Errorf("artifact content = %q, want %q", got, "ci-built kernel")
	}
}

func TestResolveArtifactsCallsBoardArtifactsFuncAtMostOnce(t *testing.T) {
	boardDir := t.TempDir()
	for _, name := range []string{"Image", "rk3566-radxa-zero-3e.dtb"} {
		if err := os.WriteFile(filepath.Join(boardDir, name), []byte("fake "+name), 0o644); err != nil {
			t.Fatalf("seeding fake CI-built artifact %q: %v", name, err)
		}
	}

	calls := 0
	fetchBoardArtifacts := func(context.Context, string, string) (string, error) {
		calls++
		return boardDir, nil
	}

	refs := []boards.ArtifactRef{{Name: "Image"}, {Name: "rk3566-radxa-zero-3e.dtb"}}
	if _, err := boards.ResolveArtifacts(context.Background(), "radxa-zero-3e", refs, "", t.TempDir(), fetchBoardArtifacts); err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	if calls != 1 {
		t.Errorf("fetchBoardArtifacts was called %d times for 2 no-URL refs, want 1 (memoized per ResolveArtifacts call)", calls)
	}
}

func TestResolveArtifactsSurfacesBoardArtifactsFuncErrorActionably(t *testing.T) {
	wantErr := errors.New("network unreachable")
	fetchBoardArtifacts := func(context.Context, string, string) (string, error) {
		return "", wantErr
	}

	refs := []boards.ArtifactRef{{Name: "Image"}}
	_, err := boards.ResolveArtifacts(context.Background(), "radxa-zero-3e", refs, "", t.TempDir(), fetchBoardArtifacts)
	if err == nil {
		t.Fatal("ResolveArtifacts() succeeded, want an error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "--artifacts-dir") {
		t.Errorf("error = %q, want it to mention --artifacts-dir as an alternative", err)
	}
}

func TestResolveArtifactsWithNoURLNotFoundInDownloadedArtifactsIsActionable(t *testing.T) {
	fetchBoardArtifacts := func(context.Context, string, string) (string, error) {
		return t.TempDir(), nil // empty: doesn't contain the requested artifact
	}

	refs := []boards.ArtifactRef{{Name: "Image"}}
	_, err := boards.ResolveArtifacts(context.Background(), "radxa-zero-3e", refs, "", t.TempDir(), fetchBoardArtifacts)
	if err == nil {
		t.Fatal("ResolveArtifacts() succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--artifacts-dir") {
		t.Errorf("error = %q, want it to mention --artifacts-dir", err)
	}
}

func TestArtifactsOpenUnresolvedNameErrors(t *testing.T) {
	art, err := boards.ResolveArtifacts(context.Background(), "pi-zero-2w", nil, "", "", nil)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	if _, err := art.Open("missing"); err == nil {
		t.Fatal("Open(missing) succeeded, want an error")
	}
}

func TestArtifactsMustOpenPanicsOnUnresolvedName(t *testing.T) {
	art, err := boards.ResolveArtifacts(context.Background(), "pi-zero-2w", nil, "", "", nil)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustOpen(missing) did not panic")
		}
	}()
	art.MustOpen("missing")
}
