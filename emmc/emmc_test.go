package emmc

import (
	"errors"
	"testing"

	"github.com/jphastings/gosd/internal/emmcfmt"
)

// fakeDeps records what run did and scripts what each dependency returns, so
// the orchestration can be exercised without a real eMMC.
type fakeDeps struct {
	mounted   bool
	contents  emmcfmt.Contents
	discErr   error
	inspErr   error
	formatErr error
	mountErr  error

	formatted   bool
	formatLabel string
	didMount    bool
	mountDevice string
	mountTarget string
}

func (f *fakeDeps) deps() deps {
	return deps{
		mountedAt: func(string) (string, bool, error) {
			if f.mounted {
				return "/dev/mmcblk0", true, nil
			}
			return "", false, nil
		},
		discover: func() (string, error) {
			if f.discErr != nil {
				return "", f.discErr
			}
			return "/dev/mmcblk0", nil
		},
		inspect: func(string) (emmcfmt.Contents, error) { return f.contents, f.inspErr },
		format: func(_, label string) error {
			f.formatted, f.formatLabel = true, label
			return f.formatErr
		},
		mount: func(device, mountpoint string) error {
			f.didMount, f.mountDevice, f.mountTarget = true, device, mountpoint
			return f.mountErr
		},
	}
}

func TestRunMountsOnlyWhenLabelAlreadyMatches(t *testing.T) {
	// A previous run of the same app already formatted the eMMC, so this run
	// must mount it without reformatting (which would wipe the data).
	f := &fakeDeps{contents: emmcfmt.Contents{IsFAT: true, Label: "APPDATA"}}

	device, err := run(f.deps(), "appdata", "/storage", false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if device != "/dev/mmcblk0" {
		t.Errorf("run device = %q, want /dev/mmcblk0", device)
	}
	if f.formatted {
		t.Error("reformatted an eMMC that already had the app's label")
	}
	if !f.didMount || f.mountTarget != "/storage" {
		t.Errorf("mount = (%v, %q), want mounted at /storage", f.didMount, f.mountTarget)
	}
}

func TestRunFormatsBlankWithoutDestructive(t *testing.T) {
	f := &fakeDeps{contents: emmcfmt.Contents{Blank: true}}

	if _, err := run(f.deps(), "APPDATA", "/storage", false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !f.formatted || f.formatLabel != "APPDATA" {
		t.Errorf("format = (%v, %q), want formatted with APPDATA", f.formatted, f.formatLabel)
	}
	if !f.didMount {
		t.Error("did not mount after formatting a blank eMMC")
	}
}

func TestRunRefusesForeignContentWithoutDestructive(t *testing.T) {
	f := &fakeDeps{contents: emmcfmt.Contents{IsFAT: true, Label: "OTHERAPP"}}

	_, err := run(f.deps(), "APPDATA", "/storage", false)
	if err == nil {
		t.Fatal("run succeeded on foreign content without destructive=true")
	}
	if f.formatted || f.didMount {
		t.Errorf("touched the device (formatted=%v mounted=%v) when it should have refused", f.formatted, f.didMount)
	}
}

func TestRunReformatsForeignContentWhenDestructive(t *testing.T) {
	f := &fakeDeps{contents: emmcfmt.Contents{Blank: false}} // non-FAT foreign content

	if _, err := run(f.deps(), "APPDATA", "/storage", true); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !f.formatted || !f.didMount {
		t.Errorf("formatted=%v mounted=%v, want both true under destructive=true", f.formatted, f.didMount)
	}
}

func TestRunIsIdempotentWhenAlreadyMounted(t *testing.T) {
	f := &fakeDeps{mounted: true, contents: emmcfmt.Contents{Blank: true}}

	device, err := run(f.deps(), "APPDATA", "/storage", false)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if device != "/dev/mmcblk0" {
		t.Errorf("run device = %q, want the already-mounted device reported back", device)
	}
	if f.formatted || f.didMount {
		t.Error("did work despite the storage already being mounted")
	}
}

func TestRunSurfacesNoEMMC(t *testing.T) {
	f := &fakeDeps{discErr: ErrNoEMMC}

	_, err := run(f.deps(), "APPDATA", "/storage", false)
	if !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("run error = %v, want ErrNoEMMC", err)
	}
}

func TestRunRejectsBadLabelBeforeTouchingDevice(t *testing.T) {
	f := &fakeDeps{}

	if _, err := run(f.deps(), "waytoolongforfat", "/storage", true); err == nil {
		t.Fatal("run accepted a 16-character label")
	}
	if f.formatted || f.didMount {
		t.Error("did device work despite an invalid label")
	}
}

func TestChooseEMMCPrefersUnmountedMMCRegardlessOfNumber(t *testing.T) {
	// The eMMC is mmcblk1 here and the booted SD is mmcblk0, proving selection
	// is by type + not-in-use, not by device number.
	devices := []blockDevice{
		{name: "mmcblk0", kind: "SD", partitions: []string{"mmcblk0p1", "mmcblk0p2"}},
		{name: "mmcblk1", kind: "MMC"},
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true, "/dev/mmcblk0p2": true}

	got, err := chooseEMMC(devices, mounted)
	if err != nil {
		t.Fatalf("chooseEMMC: %v", err)
	}
	if got != "/dev/mmcblk1" {
		t.Errorf("chooseEMMC = %q, want /dev/mmcblk1", got)
	}
}

func TestChooseEMMCSkipsTheBootDevice(t *testing.T) {
	// Booting from the eMMC: its partitions are mounted, so it must be off
	// limits and discovery must report no eMMC rather than a wiped system.
	devices := []blockDevice{
		{name: "mmcblk0", kind: "MMC", partitions: []string{"mmcblk0p1", "mmcblk0p2"}},
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true, "/dev/mmcblk0p2": true}

	if _, err := chooseEMMC(devices, mounted); !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("chooseEMMC error = %v, want ErrNoEMMC", err)
	}
}

func TestChooseEMMCReportsNoEMMCWhenOnlySDPresent(t *testing.T) {
	devices := []blockDevice{{name: "mmcblk0", kind: "SD", partitions: []string{"mmcblk0p1"}}}

	if _, err := chooseEMMC(devices, nil); !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("chooseEMMC error = %v, want ErrNoEMMC", err)
	}
}

func TestValidateLabel(t *testing.T) {
	valid := []string{"A", "APPDATA", "ELEVENCHARS"}
	for _, label := range valid {
		if err := validateLabel(label); err != nil {
			t.Errorf("validateLabel(%q) = %v, want nil", label, err)
		}
	}
	invalid := []string{"", "TWELVECHARSX", "café"}
	for _, label := range invalid {
		if err := validateLabel(label); err == nil {
			t.Errorf("validateLabel(%q) = nil, want an error", label)
		}
	}
}
