package boards_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	art, err := boards.ResolveArtifacts(context.Background(), refs, dir, t.TempDir())
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
	art, err := boards.ResolveArtifacts(context.Background(), refs, "", t.TempDir())
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

func TestResolveArtifactsWithNoURLAndNoLocalFileIsActionable(t *testing.T) {
	refs := []boards.ArtifactRef{{Name: "kernel8.img"}}

	_, err := boards.ResolveArtifacts(context.Background(), refs, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("ResolveArtifacts() succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--artifacts-dir") {
		t.Errorf("error = %q, want it to mention --artifacts-dir", err)
	}
}

func TestArtifactsOpenUnresolvedNameErrors(t *testing.T) {
	art, err := boards.ResolveArtifacts(context.Background(), nil, "", "")
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	if _, err := art.Open("missing"); err == nil {
		t.Fatal("Open(missing) succeeded, want an error")
	}
}

func TestArtifactsMustOpenPanicsOnUnresolvedName(t *testing.T) {
	art, err := boards.ResolveArtifacts(context.Background(), nil, "", "")
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
