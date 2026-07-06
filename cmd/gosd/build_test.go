package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
)

func TestResolveBoardsDefaultsToAll(t *testing.T) {
	got, err := resolveBoards(nil)
	if err != nil {
		t.Fatalf("resolveBoards(nil): %v", err)
	}
	if !reflect.DeepEqual(got, boards.All()) {
		t.Errorf("resolveBoards(nil) = %v, want all registered boards %v", got, boards.All())
	}
}

func TestResolveBoardsFiltersAndDeduplicates(t *testing.T) {
	got, err := resolveBoards([]string{"pi-zero-2w", "pi-zero-2w"})
	if err != nil {
		t.Fatalf("resolveBoards: %v", err)
	}
	if len(got) != 1 || got[0].Name() != "pi-zero-2w" {
		t.Errorf("resolveBoards([pi-zero-2w, pi-zero-2w]) = %v, want a single pi-zero-2w entry", got)
	}
}

func TestResolveBoardsRejectsUnknownBoard(t *testing.T) {
	if _, err := resolveBoards([]string{"not-a-board"}); err == nil {
		t.Fatal("resolveBoards([not-a-board]) succeeded, want an error")
	}
}

func TestParseDataSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1GiB", 1024 * 1024 * 1024},
		{"1gib", 1024 * 1024 * 1024},
		{"512MiB", 512 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
		{"64K", 64 * 1024},
		{"4096", 4096},
		{" 8 MiB ", 8 * 1024 * 1024},
	}
	for _, c := range cases {
		got, err := parseDataSize(c.in)
		if err != nil {
			t.Errorf("parseDataSize(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDataSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseDataSizeRejectsInvalidValues(t *testing.T) {
	for _, in := range []string{"", "-1", "-1GiB", "1GB", "lots", "1.5GiB"} {
		if _, err := parseDataSize(in); err == nil {
			t.Errorf("parseDataSize(%q) succeeded, want an error", in)
		}
	}
}

func mustFindBoard(t *testing.T, id string) boards.Board {
	t.Helper()
	b, ok := boards.Find(id)
	if !ok {
		t.Fatalf("boards.Find(%q) = not found; registered boards: %v", id, boards.IDs())
	}
	return b
}

func TestResolveOutputsSingleBoardUsesOutputAsFile(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	got, err := resolveOutputs(selected, "myapp", "/tmp/x.img")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	if got["pi-zero-2w"] != "/tmp/x.img" {
		t.Errorf("outputs[pi-zero-2w] = %q, want /tmp/x.img", got["pi-zero-2w"])
	}
}

func TestResolveOutputsSingleBoardDefaultsToAppNameBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	got, err := resolveOutputs(selected, "myapp", "")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	if got["pi-zero-2w"] != "myapp-pi-zero-2w.img" {
		t.Errorf("outputs[pi-zero-2w] = %q, want myapp-pi-zero-2w.img", got["pi-zero-2w"])
	}
}

func TestResolveOutputsMultiBoardTreatsOutputAsDirectory(t *testing.T) {
	selected := boards.All()

	got, err := resolveOutputs(selected, "myapp", "/tmp/out")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	for _, b := range selected {
		want := "/tmp/out/myapp-" + b.Name() + ".img"
		if got[b.Name()] != want {
			t.Errorf("outputs[%s] = %q, want %q", b.Name(), got[b.Name()], want)
		}
	}
}

func TestEnsureOutputDirCreatesMissingMultiBoardDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")

	if err := ensureOutputDir(dir, true); err != nil {
		t.Fatalf("ensureOutputDir(%q, true): %v", dir, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("ensureOutputDir(%q, true) did not create a directory there", dir)
	}
}

func TestEnsureOutputDirCreatesMissingSingleBoardParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "app-board.img")

	if err := ensureOutputDir(path, false); err != nil {
		t.Fatalf("ensureOutputDir(%q, false): %v", path, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Errorf("ensureOutputDir(%q, false) did not create the parent directory", path)
	}
}

func TestEnsureOutputDirMultiBoardRejectsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}

	err := ensureOutputDir(path, true)
	if err == nil {
		t.Fatalf("ensureOutputDir(%q, true) succeeded, want an error", path)
	}
	if got, want := err.Error(), "-o must be a directory when building multiple boards"; !strings.Contains(got, want) {
		t.Errorf("ensureOutputDir error = %q, want it to contain %q", got, want)
	}
}

func TestEnsureOutputDirEmptySingleBoardOutputIsNoop(t *testing.T) {
	if err := ensureOutputDir("", false); err != nil {
		t.Errorf("ensureOutputDir(\"\", false) = %v, want nil", err)
	}
}

func TestResolveOutputsMultiBoardDefaultsToCurrentDirectory(t *testing.T) {
	selected := boards.All()

	got, err := resolveOutputs(selected, "myapp", "")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	for _, b := range selected {
		want := "myapp-" + b.Name() + ".img"
		if got[b.Name()] != want {
			t.Errorf("outputs[%s] = %q, want %q", b.Name(), got[b.Name()], want)
		}
	}
}
