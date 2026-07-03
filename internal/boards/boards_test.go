package boards_test

import (
	"io"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/image"
)

type fakeBoard struct{ name string }

func (f fakeBoard) Name() string                  { return f.name }
func (fakeBoard) Artifacts() []boards.ArtifactRef { return nil }
func (fakeBoard) BootFiles(boards.BuildConfig, boards.Artifacts) (map[string]io.Reader, error) {
	return nil, nil
}
func (fakeBoard) RawWrites(boards.Artifacts) []image.RawWrite         { return nil }
func (fakeBoard) FirmwareFiles(boards.Artifacts) map[string]io.Reader { return nil }

func TestRegisterMakesABoardFindable(t *testing.T) {
	boards.Register(fakeBoard{name: "test-board-findable"})

	b, ok := boards.Find("test-board-findable")
	if !ok {
		t.Fatal("Find(test-board-findable) = not found, want it registered")
	}
	if b.Name() != "test-board-findable" {
		t.Errorf("Find(test-board-findable).Name() = %q, want test-board-findable", b.Name())
	}
}

func TestRegisterAppearsInAllAndIDs(t *testing.T) {
	boards.Register(fakeBoard{name: "test-board-listed"})

	foundInIDs := false
	for _, id := range boards.IDs() {
		if id == "test-board-listed" {
			foundInIDs = true
		}
	}
	if !foundInIDs {
		t.Errorf("IDs() = %v, want it to contain test-board-listed", boards.IDs())
	}

	foundInAll := false
	for _, b := range boards.All() {
		if b.Name() == "test-board-listed" {
			foundInAll = true
		}
	}
	if !foundInAll {
		t.Error("All() didn't contain the registered board")
	}
}

func TestFindUnknownBoardReturnsFalse(t *testing.T) {
	if _, ok := boards.Find("no-such-board"); ok {
		t.Fatal("Find(no-such-board) = found, want not found")
	}
}

func TestRegisterPanicsOnDuplicateName(t *testing.T) {
	boards.Register(fakeBoard{name: "test-board-duplicate"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Register with an already-registered name did not panic")
		}
	}()
	boards.Register(fakeBoard{name: "test-board-duplicate"})
}
