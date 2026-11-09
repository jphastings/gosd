package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlashRefusesAFileThatIsntAFlashableImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blank.img")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"flash", path})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "can't flash") {
		t.Fatalf("want a refusal, got %v", err)
	}
}
