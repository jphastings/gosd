package boards_test

import (
	"io"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/image"
)

type fakeBoard struct{ name string }

func (f fakeBoard) Name() string                  { return f.name }
func (fakeBoard) Arch() boards.Arch               { return boards.Arch{GOARCH: "arm64"} }
func (fakeBoard) Artifacts() []boards.ArtifactRef { return nil }
func (fakeBoard) BootFiles(boards.BuildConfig, boards.Artifacts) (map[string]io.Reader, error) {
	return nil, nil
}
func (fakeBoard) RawWrites(boards.Artifacts) []image.RawWrite         { return nil }
func (fakeBoard) FirmwareFiles(boards.Artifacts) map[string]io.Reader { return nil }
func (fakeBoard) UsbGadgetSupport() boards.GadgetSupport {
	return boards.GadgetSupport{Supported: true}
}

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

func TestRegisterInternalIsFindableButExcludedFromAllAndIDs(t *testing.T) {
	boards.RegisterInternal(fakeBoard{name: "test-board-internal"})

	b, ok := boards.Find("test-board-internal")
	if !ok {
		t.Fatal("Find(test-board-internal) = not found, want an internal board still findable by explicit ID")
	}
	if b.Name() != "test-board-internal" {
		t.Errorf("Find(test-board-internal).Name() = %q, want test-board-internal", b.Name())
	}

	for _, id := range boards.IDs() {
		if id == "test-board-internal" {
			t.Error("IDs() contains an internal-only board; it must be excluded from user-facing listings")
		}
	}
	for _, b := range boards.All() {
		if b.Name() == "test-board-internal" {
			t.Error("All() contains an internal-only board; it must be excluded from the default build set")
		}
	}
	if !boards.IsInternal("test-board-internal") {
		t.Error("IsInternal(test-board-internal) = false, want true")
	}
	if boards.IsInternal("test-board-listed") {
		t.Error("IsInternal(test-board-listed) = true, want false: it was registered via Register, not RegisterInternal")
	}
}

func TestBuildTag(t *testing.T) {
	cases := map[string]string{
		"pi-zero-2w":    "gosd_pi_zero_2w",
		"nanopi-zero2":  "gosd_nanopi_zero2",
		"radxa-zero-3e": "gosd_radxa_zero_3e",
	}

	for name, want := range cases {
		if got := boards.BuildTag(fakeBoard{name: name}); got != want {
			t.Errorf("BuildTag(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestKnownArchesKeyedByTheirOwnKey(t *testing.T) {
	for token, arch := range boards.KnownArches {
		if arch.Key() != token {
			t.Errorf("KnownArches[%q].Key() = %q, want %q", token, arch.Key(), token)
		}
	}
}

func TestRegisterAndRegisterInternalShareOneNamespace(t *testing.T) {
	boards.Register(fakeBoard{name: "test-board-shared-namespace"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("RegisterInternal with a name already used by Register did not panic")
		}
	}()
	boards.RegisterInternal(fakeBoard{name: "test-board-shared-namespace"})
}
