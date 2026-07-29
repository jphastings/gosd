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
	mounted     bool
	contents    diskfmt.Contents
	unmountable diskfmt.FS // a filesystem this fake kernel cannot mount
	discErr     error
	inspErr     error
	formatErr   error
	mountErr    error

	formatted   bool
	formatLabel string
	formatFS    diskfmt.FS
	didMount    bool
	mountTarget string
	mountFS     diskfmt.FS
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
			Format: func(_, label string, fs diskfmt.FS) error {
				f.formatted, f.formatLabel, f.formatFS = true, label, fs
				return f.formatErr
			},
			Mount: func(_, mountpoint string, fs diskfmt.FS) error {
				f.didMount, f.mountTarget, f.mountFS = true, mountpoint, fs
				return f.mountErr
			},
			Mountable: func(fs diskfmt.FS) (bool, error) { return fs != f.unmountable, nil },
		},
	}
}

func TestRunMountsOnlyWhenLabelAlreadyMatches(t *testing.T) {
	// A previous run of the same app already formatted the storage, so this run
	// must mount it without reformatting (which would wipe the data).
	f := &fakeDeps{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "APPDATA"}}

	device, err := Run(f.storage(), diskfmt.FAT32, "appdata", "/storage", false)
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

	if _, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false); err != nil {
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
		{"another app's FAT32 volume", diskfmt.Contents{FS: diskfmt.FAT32, Label: "OTHERAPP"}, `FAT32 labelled "OTHERAPP"`},
		{"another app's exFAT volume", diskfmt.Contents{FS: diskfmt.ExFAT, Label: "OTHERAPP"}, `exFAT labelled "OTHERAPP"`},
		{"an unreadable exFAT volume", diskfmt.Contents{OtherFS: "exFAT"}, "exFAT that GoSD could not read"},
		{"unrecognised content", diskfmt.Contents{}, "content GoSD does not recognise"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDeps{contents: tc.contents}

			_, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false)
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

// TestRunMountsAnExistingVolumeAsItsOwnFilesystem covers the drive that
// arrived exFAT-formatted and already carries the app's label: the app asked
// for FAT32, but converting it would destroy the data it came for, so it is
// mounted as the exFAT it is.
func TestRunMountsAnExistingVolumeAsItsOwnFilesystem(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{FS: diskfmt.ExFAT, Label: "APPDATA"}}

	if _, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.formatted {
		t.Error("reformatted an exFAT volume that already had the app's label")
	}
	if f.mountFS != diskfmt.ExFAT {
		t.Errorf("mounted as %s, want exFAT — mounting it as the caller's FAT32 would fail", f.mountFS)
	}
}

func TestRunFormatsWithTheRequestedFilesystem(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{Blank: true}}

	if _, err := Run(f.storage(), diskfmt.ExFAT, "APPDATA", "/storage", false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.formatFS != diskfmt.ExFAT || f.mountFS != diskfmt.ExFAT {
		t.Errorf("formatted as %s and mounted as %s, want exFAT for both", f.formatFS, f.mountFS)
	}
}

// TestRunRefusesAFilesystemTheKernelCannotMount pins the ordering that matters:
// a board whose kernel lacks the filesystem must be told so while its disk is
// still intact, never after a successful format that then cannot be mounted.
func TestRunRefusesAFilesystemTheKernelCannotMount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents diskfmt.Contents
		want     diskfmt.FS
	}{
		{"asked to format one", diskfmt.Contents{Blank: true}, diskfmt.ExFAT},
		{"asked to mount one already there", diskfmt.Contents{FS: diskfmt.ExFAT, Label: "APPDATA"}, diskfmt.FAT32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDeps{contents: tc.contents, unmountable: diskfmt.ExFAT}

			_, err := Run(f.storage(), tc.want, "APPDATA", "/storage", false)
			if !errors.Is(err, ErrUnsupportedFS) {
				t.Fatalf("Run error = %v, want ErrUnsupportedFS", err)
			}
			if !strings.Contains(err.Error(), "exFAT") {
				t.Errorf("Run error = %q, want it to name the missing filesystem", err)
			}
			if f.formatted || f.didMount {
				t.Errorf("touched the device (formatted=%v mounted=%v) despite the kernel being unable to mount it", f.formatted, f.didMount)
			}
		})
	}
}

func TestRunReformatsForeignContentWhenDestructive(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{Blank: false}} // non-FAT foreign content

	if _, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !f.formatted || !f.didMount {
		t.Errorf("formatted=%v mounted=%v, want both true under destructive=true", f.formatted, f.didMount)
	}
}

func TestRunIsIdempotentWhenAlreadyMounted(t *testing.T) {
	f := &fakeDeps{mounted: true, contents: diskfmt.Contents{Blank: true}}

	device, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false)
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

	_, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want the discovery sentinel", err)
	}
}

func TestRunRejectsBadLabelBeforeTouchingDevice(t *testing.T) {
	f := &fakeDeps{}

	if _, err := Run(f.storage(), diskfmt.FAT32, "waytoolongforfat", "/storage", true); err == nil {
		t.Fatal("Run accepted a 16-character label")
	}
	if f.formatted || f.didMount {
		t.Error("did device work despite an invalid label")
	}
}

func TestRunNamesTheStorageInItsErrors(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{Blank: true}, mountErr: errors.New("EIO")}

	_, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false)
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
