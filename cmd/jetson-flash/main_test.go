package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlashRefusesAFileThatIsntAGosdImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blank.img")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"flash", path})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "isn't a gosd image") {
		t.Fatalf("want a not-a-gosd-image error, got %v", err)
	}
}
