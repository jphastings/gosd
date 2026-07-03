package radxazero3e_test

import (
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
)

func TestName(t *testing.T) {
	if got := radxazero3e.New().Name(); got != "radxa-zero-3e" {
		t.Errorf("Name() = %q, want radxa-zero-3e", got)
	}
}

func TestBootFilesReturnsAClearNotImplementedError(t *testing.T) {
	_, err := radxazero3e.New().BootFiles(boards.BuildConfig{}, boards.Artifacts{})
	if err == nil {
		t.Fatal("BootFiles() succeeded, want a clear not-implemented error")
	}
	if !strings.Contains(err.Error(), "gosd-gbsz") {
		t.Errorf("error = %q, want it to point at bean gosd-gbsz", err)
	}
}

func TestRawWritesAndFirmwareFilesAreEmpty(t *testing.T) {
	b := radxazero3e.New()
	if got := b.RawWrites(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("RawWrites() = %v, want empty", got)
	}
	if got := b.FirmwareFiles(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("FirmwareFiles() = %v, want empty", got)
	}
}
