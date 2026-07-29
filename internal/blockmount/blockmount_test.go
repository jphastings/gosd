package blockmount

import (
	"errors"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/diskfmt"
)

// fakeDeps records what Run did and scripts what each dependency returns, so
// the orchestration can be exercised without real storage.
type fakeDeps struct {
	mounted   bool
	contents  diskfmt.Contents
	discErr   error
	inspErr   error
	formatErr error
	mountErr  error

	formatted   bool
	formatLabel string
	didMount    bool
	mountTarget string
}

const fakeDevice = "/dev/fake0"

func (f *fakeDeps) storage() Storage {
	return Storage{
		Pkg:  "fakepkg",
		Noun: "widget",
		Deps: Deps{
			MountedAt: func(string) (string, bool, error) {
				if f.mounted {
					return fakeDevice, true, nil
				}
				return "", false, nil
			},
			Discover: func() (string, error) {
				if f.discErr != nil {
					return "", f.discErr
				}
				return fakeDevice, nil
			},
			Inspect: func(string) (diskfmt.Contents, error) { return f.contents, f.inspErr },
			Format: func(_, label string) error {
				f.formatted, f.formatLabel = true, label
				return f.formatErr
			},
			Mount: func(_, mountpoint string) error {
				f.didMount, f.mountTarget = true, mountpoint
				return f.mountErr
			},
		},
	}
}

func TestRunMountsOnlyWhenLabelAlreadyMatches(t *testing.T) {
	// A previous run of the same app already formatted the storage, so this run
	// must mount it without reformatting (which would wipe the data).
	f := &fakeDeps{contents: diskfmt.Contents{IsFAT: true, Label: "APPDATA"}}

	device, err := Run(f.storage(), "appdata", "/storage", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if device != fakeDevice {
		t.Errorf("Run device = %q, want %q", device, fakeDevice)
	}
	if f.formatted {
		t.Error("reformatted storage that already had the app's label")
	}
	if !f.didMount || f.mountTarget != "/storage" {
		t.Errorf("mount = (%v, %q), want mounted at /storage", f.didMount, f.mountTarget)
	}
}

func TestRunFormatsBlankWithoutDestructive(t *testing.T) {
	// Blank media never needs consent, even without destructive=true — this
	// pins the other side of ErrRefusedFormat's contract alongside
	// TestRunRefusesForeignContentWithoutDestructive below: Run only ever wraps
	// ErrRefusedFormat when the device holds *other* content, never for blank
	// media.
	f := &fakeDeps{contents: diskfmt.Contents{Blank: true}}

	if _, err := Run(f.storage(), "APPDATA", "/storage", false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !f.formatted || f.formatLabel != "APPDATA" {
		t.Errorf("format = (%v, %q), want formatted with APPDATA", f.formatted, f.formatLabel)
	}
	if !f.didMount {
		t.Error("did not mount after formatting blank media")
	}
}

func TestRunRefusesForeignContentWithoutDestructive(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents diskfmt.Contents
		describe string
	}{
		{"another app's FAT volume", diskfmt.Contents{IsFAT: true, Label: "OTHERAPP"}, `a FAT volume labelled "OTHERAPP"`},
		{"an exFAT disk", diskfmt.Contents{OtherFS: "exFAT"}, "an exFAT filesystem, which GoSD cannot mount"},
		{"unrecognised content", diskfmt.Contents{}, "non-FAT content"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDeps{contents: tc.contents}

			_, err := Run(f.storage(), "APPDATA", "/storage", false)
			if !errors.Is(err, ErrRefusedFormat) {
				t.Fatalf("Run error = %v, want ErrRefusedFormat", err)
			}
			// The message must say what is actually there, so an app author
			// can tell "someone else's data" from "a filesystem we can't read".
			if !strings.Contains(err.Error(), tc.describe) {
				t.Errorf("Run error = %q, want it to mention %q", err, tc.describe)
			}
			if f.formatted || f.didMount {
				t.Errorf("touched the device (formatted=%v mounted=%v) when it should have refused", f.formatted, f.didMount)
			}
		})
	}
}

func TestRunReformatsForeignContentWhenDestructive(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{Blank: false}} // non-FAT foreign content

	if _, err := Run(f.storage(), "APPDATA", "/storage", true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !f.formatted || !f.didMount {
		t.Errorf("formatted=%v mounted=%v, want both true under destructive=true", f.formatted, f.didMount)
	}
}

func TestRunIsIdempotentWhenAlreadyMounted(t *testing.T) {
	f := &fakeDeps{mounted: true, contents: diskfmt.Contents{Blank: true}}

	device, err := Run(f.storage(), "APPDATA", "/storage", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if device != fakeDevice {
		t.Errorf("Run device = %q, want the already-mounted device reported back", device)
	}
	if f.formatted || f.didMount {
		t.Error("did work despite the storage already being mounted")
	}
}

func TestRunSurfacesTheDiscoveryError(t *testing.T) {
	// Each public package has its own "nothing found" sentinel; Run must pass
	// whatever discovery reports through unchanged so errors.Is still matches.
	sentinel := errors.New("no storage of this kind found")
	f := &fakeDeps{discErr: sentinel}

	_, err := Run(f.storage(), "APPDATA", "/storage", false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want the discovery sentinel", err)
	}
}

func TestRunRejectsBadLabelBeforeTouchingDevice(t *testing.T) {
	f := &fakeDeps{}

	if _, err := Run(f.storage(), "waytoolongforfat", "/storage", true); err == nil {
		t.Fatal("Run accepted a 16-character label")
	}
	if f.formatted || f.didMount {
		t.Error("did device work despite an invalid label")
	}
}

func TestRunNamesTheStorageInItsErrors(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{Blank: true}, mountErr: errors.New("EIO")}

	_, err := Run(f.storage(), "APPDATA", "/storage", false)
	if err == nil {
		t.Fatal("Run succeeded despite a failing mount")
	}
	if !strings.Contains(err.Error(), "widget") || !strings.Contains(err.Error(), fakeDevice) {
		t.Errorf("Run error = %q, want it to name both the storage kind and the device", err)
	}
}

func TestValidateLabel(t *testing.T) {
	valid := []string{"A", "APPDATA", "ELEVENCHARS"}
	for _, label := range valid {
		if err := ValidateLabel("pkg", label); err != nil {
			t.Errorf("ValidateLabel(%q) = %v, want nil", label, err)
		}
	}
	invalid := []string{"", "TWELVECHARSX", "café"}
	for _, label := range invalid {
		err := ValidateLabel("pkg", label)
		if err == nil {
			t.Errorf("ValidateLabel(%q) = nil, want an error", label)
			continue
		}
		if !strings.HasPrefix(err.Error(), "pkg: ") {
			t.Errorf("ValidateLabel(%q) = %q, want it prefixed with the caller's package name", label, err)
		}
	}
}
