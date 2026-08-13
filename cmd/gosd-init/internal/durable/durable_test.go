package durable_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/durable"
	"github.com/jphastings/gosd/internal/configtree"
)

func TestWriteFileReplacesContentsAndLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostname")

	if err := durable.WriteFile(path, []byte("first")); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	if err := durable.WriteFile(path, []byte("second")); err != nil {
		t.Fatalf("WriteFile() second call = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if string(data) != "second" {
		t.Errorf("contents = %q, want %q", data, "second")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing the directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the file itself", len(entries))
	}
}

func TestWriteFileLeavesTheOldContentsWhenItCannotWrite(t *testing.T) {
	// The temporary file can't be created inside a directory that isn't
	// there, so nothing is written and nothing is destroyed — the point of
	// writing through a temporary name in the first place.
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "hostname")

	if err := durable.WriteFile(path, []byte("value")); err == nil {
		t.Fatal("WriteFile() into a missing directory = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Errorf("stat of the missing directory = %v, want it still absent", err)
	}
}

func TestTheTemporaryFileIsNeverReadAsASetting(t *testing.T) {
	// A power cut between the write and the rename leaves the temporary
	// file behind. Inside the card's config tree that leftover must not
	// read as a setting of its own — which is exactly what a name the
	// device ignores, and the build refuses, buys.
	tmp := filepath.Base(durable.TempName(filepath.Join("/boot", "config", "hostname")))

	if !configtree.IgnoredName(tmp) {
		t.Errorf("a leftover %q would be read as a setting; it must be a name the device ignores", tmp)
	}
}
