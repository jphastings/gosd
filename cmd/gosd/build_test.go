package main

import (
	"reflect"
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
