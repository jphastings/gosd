package blockmount

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// mountedAfterDiscover simulates something other than a sibling Run call
	// — a udev rule, another process — mounting the device in the window
	// between Discover choosing it and Format writing to it.
	mountedAfterDiscover bool

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
			MountedSources: func() (map[string]bool, error) {
				if f.mountedAfterDiscover {
					return map[string]bool{fakeDevice: true}, nil
				}
				return map[string]bool{}, nil
			},
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

// TestRunRefusesWhenDeviceBecomesMountedBetweenDiscoverAndFormat pins the
// second half of the gosd-45bv fix. runMu only rules out a sibling call to
// Run; it says nothing about the device being mounted by something else
// entirely (a udev rule, another process) in the window between Discover
// choosing it and Format writing to it. That window exists even for a
// single, uncontested Run call, so it needs its own re-check rather than
// relying on the mutex.
func TestRunRefusesWhenDeviceBecomesMountedBetweenDiscoverAndFormat(t *testing.T) {
	f := &fakeDeps{contents: diskfmt.Contents{Blank: true}, mountedAfterDiscover: true}

	_, err := Run(f.storage(), diskfmt.FAT32, "APPDATA", "/storage", false)
	if err == nil {
		t.Fatal("Run succeeded despite the device becoming mounted after Discover")
	}
	if f.formatted {
		t.Error("formatted a device that became mounted after Discover — exactly the corruption the re-check exists to prevent")
	}
}

// sharedFakeDevice is a single candidate device shared by two concurrent Run
// calls, standing in for the eMMC that both emmc.FormatAndMount and
// disk.FormatAndMount would discover on a Rockchip board (gosd-45bv). Its
// fields are deliberately unguarded: when runMu correctly serialises Run,
// only one goroutine is ever inside Discover..Mount at a time, so plain reads
// and writes are safe (runMu's Lock/Unlock give -race the happens-before
// edges it needs); if runMu ever failed to do that, concurrent unguarded
// access to these same fields is exactly the kind of race -race is run to
// catch.
type sharedFakeDevice struct {
	mounted  bool
	contents diskfmt.Contents
	formats  int
}

var errNoCandidateFake = errors.New("fake: no candidate device available")

func (d *sharedFakeDevice) deps(t *testing.T) Deps {
	// gate is the deterministic proof the bean asks for: rather than
	// inferring serialisation from timing (racy — a goroutine that simply
	// hasn't been scheduled yet looks identical to one correctly blocked on
	// a lock), it directly inspects runMu, which this test file shares the
	// package with. TryLock succeeding means nobody holds runMu, i.e. this
	// Discover call is running outside Run's critical section — impossible
	// if Run's locking is intact, and reported as a test failure the instant
	// it happens rather than only when it happens to corrupt the outcome.
	gate := func() {
		if runMu.TryLock() {
			runMu.Unlock()
			t.Error("Discover ran without runMu held — a sibling Run call could interleave with this one")
		}
	}
	return Deps{
		MountedAt: func(string) (string, bool, error) { return "", false, nil },
		Discover: func() (string, error) {
			gate()
			if d.mounted {
				return "", errNoCandidateFake
			}
			return fakeDevice, nil
		},
		Inspect:   func(string) (diskfmt.Contents, error) { return d.contents, nil },
		Mountable: func(diskfmt.FS) (bool, error) { return true, nil },
		MountedSources: func() (map[string]bool, error) {
			if d.mounted {
				return map[string]bool{fakeDevice: true}, nil
			}
			return map[string]bool{}, nil
		},
		Format: func(_, label string, fs diskfmt.FS) error {
			d.formats++
			d.contents = diskfmt.Contents{FS: fs, Label: label}
			return nil
		},
		Mount: func(string, string, diskfmt.FS) error {
			d.mounted = true
			return nil
		},
	}
}

// TestRunSerializesConcurrentCallsForTheSameDevice is the gosd-45bv
// regression test: two goroutines calling Run concurrently for the same
// underlying candidate device — standing in for emmc.FormatAndMount and
// disk.FormatAndMount racing over one idle eMMC — must never both format it.
//
// Both calls are released together off a start barrier (a gate, not a sleep)
// so they genuinely contend rather than happening to run one after the
// other; sharedFakeDevice.deps's own gate then deterministically fails the
// test the instant either Discover call runs without runMu held, regardless
// of how the scheduler actually interleaved the two goroutines. Run with
// -race as a second, independent check: if runMu ever let the two calls
// overlap, their concurrent unguarded access to sharedFakeDevice's fields
// would also be reported as a data race.
func TestRunSerializesConcurrentCallsForTheSameDevice(t *testing.T) {
	dev := &sharedFakeDevice{contents: diskfmt.Contents{Blank: true}}
	start := make(chan struct{})
	results := make([]error, 2)
	var wg sync.WaitGroup

	run := func(i int, pkg, noun, label, mountpoint string) {
		defer wg.Done()
		<-start
		_, results[i] = Run(Storage{Pkg: pkg, Noun: noun, Deps: dev.deps(t)}, diskfmt.FAT32, label, mountpoint, false)
	}

	wg.Add(2)
	go run(0, "emmcfake", "eMMC", "APPDATA", "/storage-a")
	go run(1, "diskfake", "disk", "BULK", "/storage-b")
	close(start)
	wg.Wait()

	if dev.formats != 1 {
		t.Fatalf("device was formatted %d times, want exactly 1", dev.formats)
	}

	var successes, refusals int
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, errNoCandidateFake):
			refusals++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("results = %v, want exactly one success (it formatted) and one refusal (the device was already in use by the time it ran)", results)
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
	valid := []string{"A", "APPDATA", "ELEVENCHARS", "AB CD", "GOSD-DATA"}
	for _, label := range valid {
		if err := ValidateLabel("pkg", label); err != nil {
			t.Errorf("ValidateLabel(%q) = %v, want nil", label, err)
		}
	}
	invalid := []string{"", "TWELVECHARSX", "café", "APPDATA ", " APPDATA", " APPDATA ", "   "}
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

// TestValidateLabelRejectsEdgeSpacesActionably pins the fix for gosd-xq9l: a
// label with a leading or trailing space provably cannot round-trip through
// either filesystem's format→Inspect (both strip edge padding on read), so
// ValidateLabel must catch it up front — with an actionable message, not a
// bare complaint — rather than let it reach Run, where the mismatch would
// reformat, and destroy, the app's own data on every subsequent boot.
func TestValidateLabelRejectsEdgeSpacesActionably(t *testing.T) {
	for _, tc := range []struct {
		name  string
		label string
	}{
		{"trailing space", "APPDATA "},
		{"leading space", " APPDATA"},
		{"both edges", " APPDATA "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLabel("pkg", tc.label)
			if err == nil {
				t.Fatalf("ValidateLabel(%q) = nil, want an error", tc.label)
			}
			if !strings.Contains(err.Error(), "space") {
				t.Errorf("ValidateLabel(%q) = %q, want it to name the problem (a space)", tc.label, err)
			}
			// Actionable: point at the fix, not just the complaint.
			trimmed := strings.Trim(tc.label, " ")
			if !strings.Contains(err.Error(), trimmed) {
				t.Errorf("ValidateLabel(%q) = %q, want it to suggest the trimmed label %q as the fix", tc.label, err, trimmed)
			}
		})
	}
}

// TestValidateLabelAllowsInteriorSpaces confirms the bean's other half: only
// the edges are the problem. A space in the middle of a label lands inside
// neither filesystem's edge-padding trim, so it survives format→Inspect
// unchanged and must stay admitted.
func TestValidateLabelAllowsInteriorSpaces(t *testing.T) {
	if err := ValidateLabel("pkg", "AB CD"); err != nil {
		t.Errorf("ValidateLabel(%q) = %v, want nil — only edge spaces are the problem", "AB CD", err)
	}
}

// TestValidateLabelRejectsByte7SpaceActionably pins the fix for gosd-f83b: a
// space at byte index 7 (the FAT short-name field's last byte) cannot
// round-trip once the label is longer than 8 bytes, for a narrower reason
// than gosd-xq9l's edge-space fix — go-diskfs's FAT directory-entry parser
// trims the short-name and extension fields independently, so this interior
// space is indistinguishable from padding to that trim and silently vanishes.
func TestValidateLabelRejectsByte7SpaceActionably(t *testing.T) {
	for _, tc := range []struct {
		name  string
		label string
	}{
		{"9 bytes", "ABCDEFG H"},
		{"10 bytes", "ABCDEFG HI"},
		{"11 bytes", "ABCDEFG HIJ"},
		{"byte 7 space plus an unaffected interior space", "ABCDEFG H I"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLabel("pkg", tc.label)
			if err == nil {
				t.Fatalf("ValidateLabel(%q) = nil, want an error", tc.label)
			}
			if !strings.Contains(err.Error(), "space") || !strings.Contains(err.Error(), "8th character") {
				t.Errorf("ValidateLabel(%q) = %q, want it to name the problem (a space, and where)", tc.label, err)
			}
		})
	}
}

// TestValidateLabelAllows8ByteLabelsWithATrailingContentByte confirms the
// byte-7 rule's own boundary: for a label of 8 bytes or fewer, byte 7 (if it
// exists at all) is padding, not label content, so a space there is exactly
// the already-forbidden trailing-space case, not a new one — an 8-byte label
// ending in a non-space character at that position must stay admitted.
func TestValidateLabelAllows8ByteLabelsWithATrailingContentByte(t *testing.T) {
	if err := ValidateLabel("pkg", "ABCDEFGH"); err != nil {
		t.Errorf("ValidateLabel(%q) = %v, want nil — exactly 8 bytes, byte 7 is real content", "ABCDEFGH", err)
	}
}

// TestAllPositionsRoundTripOrAreRejected is the exhaustive round-trip test
// gosd-f83b asks for: for every label length from 1 to 11 bytes, and every
// byte position within it, build the shape with a space at that position and
// distinct printable-ASCII characters everywhere else. Either ValidateLabel
// must reject it (an edge space, an all-spaces label, or the byte-7 split) or
// a real format→Inspect round trip — on both FAT32 and exFAT — must recover
// the label exactly, and labelMatches must recognise it as already
// provisioned. This proves the rejected set is exactly right: not too wide
// (nothing round-trippable is refused) and not too narrow (nothing that
// silently corrupts is admitted).
func TestAllPositionsRoundTripOrAreRejected(t *testing.T) {
	const alphabet = "ABCDEFGHIJK" // 11 distinct non-space bytes, one per position

	for n := 1; n <= maxLabelLen; n++ {
		for p := 0; p < n; p++ {
			label := []byte(alphabet[:n])
			label[p] = ' '

			name := fmt.Sprintf("len=%d/space@%d", n, p)
			t.Run(name, func(t *testing.T) {
				wantRejected := p == 0 || p == n-1 || (n > fatShortNameSplit+1 && p == fatShortNameSplit)

				err := ValidateLabel("pkg", string(label))
				if wantRejected {
					if err == nil {
						t.Fatalf("ValidateLabel(%q) = nil, want an error — this shape cannot round-trip", label)
					}
					return
				}
				if err != nil {
					t.Fatalf("ValidateLabel(%q) = %v, want nil — this shape should round-trip", label, err)
				}

				for _, fs := range []diskfmt.FS{diskfmt.FAT32, diskfmt.ExFAT} {
					path := filepath.Join(t.TempDir(), "device.img")
					f, err := os.Create(path)
					if err != nil {
						t.Fatalf("creating backing file: %v", err)
					}
					if err := f.Truncate(64 * 1024 * 1024); err != nil {
						t.Fatalf("sizing backing file: %v", err)
					}
					if err := f.Close(); err != nil {
						t.Fatalf("closing backing file: %v", err)
					}

					if err := diskfmt.Format(path, string(label), fs); err != nil {
						t.Fatalf("Format(%q, %s): %v", label, fs, err)
					}
					contents, err := diskfmt.Inspect(path)
					if err != nil {
						t.Fatalf("Inspect(%s): %v", fs, err)
					}
					if contents.Label != string(label) {
						t.Errorf("%s: label %q round-tripped to %q", fs, label, contents.Label)
					}
					if !labelMatches(contents, string(label)) {
						t.Errorf("%s: labelMatches(%+v, %q) = false, want true", fs, contents, label)
					}
				}
			})
		}
	}
}

// TestValidateLabelRejectsNULViaThePrintableASCIICheck confirms NUL is
// already refused — by the existing printable-ASCII rule, since it is a
// control character — so the edge-space check does not need to duplicate it.
func TestValidateLabelRejectsNULViaThePrintableASCIICheck(t *testing.T) {
	err := ValidateLabel("pkg", "APP\x00DATA")
	if err == nil {
		t.Fatal("ValidateLabel accepted a label containing NUL")
	}
	if !strings.Contains(err.Error(), "printable ASCII") {
		t.Errorf("ValidateLabel(NUL label) = %q, want the printable-ASCII complaint (not a bespoke NUL check)", err)
	}
}

// TestLabelMatchesComparesTrimmed is the belt-and-braces half of gosd-xq9l:
// even if ValidateLabel's edge-space rejection were ever bypassed, Run's
// idempotency comparison (labelMatches) must not compare the untrimmed
// caller string, or a trailing-space label would mismatch forever and
// reformat — wiping the app's own data — on every single boot.
func TestLabelMatchesComparesTrimmed(t *testing.T) {
	contents := diskfmt.Contents{FS: diskfmt.FAT32, Label: "APPDATA"}

	for _, label := range []string{"APPDATA", "APPDATA ", " APPDATA", "appdata"} {
		if !labelMatches(contents, label) {
			t.Errorf("labelMatches(%+v, %q) = false, want true — an edge space or case difference must still match", contents, label)
		}
	}
	if labelMatches(contents, "OTHERAPP") {
		t.Error("labelMatches matched an unrelated label")
	}
	if labelMatches(diskfmt.Contents{Label: "APPDATA"}, "APPDATA") {
		t.Error("labelMatches matched content with no filesystem (Blank or unreadable)")
	}
}

// TestAdmittedLabelsRoundTripToWhatRunCompares is the round-trip test the
// bean asks for: it pins the invariant that every label class ValidateLabel
// admits is stable across boots — format it for real, Inspect it back for
// real, and confirm Run's own comparison (labelMatches) would recognise the
// result as "already provisioned" rather than reformatting again next boot.
func TestAdmittedLabelsRoundTripToWhatRunCompares(t *testing.T) {
	labels := []string{"A", "APPDATA", "ELEVENCHARS", "AB CD", "GOSD-DATA"}

	for _, fs := range []diskfmt.FS{diskfmt.FAT32, diskfmt.ExFAT} {
		for _, label := range labels {
			t.Run(string(fs)+"/"+label, func(t *testing.T) {
				if err := ValidateLabel("pkg", label); err != nil {
					t.Fatalf("test bug: %q is not a label ValidateLabel admits: %v", label, err)
				}

				path := filepath.Join(t.TempDir(), "device.img")
				f, err := os.Create(path)
				if err != nil {
					t.Fatalf("creating backing file: %v", err)
				}
				if err := f.Truncate(64 * 1024 * 1024); err != nil {
					t.Fatalf("sizing backing file: %v", err)
				}
				if err := f.Close(); err != nil {
					t.Fatalf("closing backing file: %v", err)
				}

				if err := diskfmt.Format(path, label, fs); err != nil {
					t.Fatalf("Format(%q, %s): %v", label, fs, err)
				}
				contents, err := diskfmt.Inspect(path)
				if err != nil {
					t.Fatalf("Inspect: %v", err)
				}
				if !labelMatches(contents, label) {
					t.Errorf("after format→Inspect, label %q round-tripped to %q, which Run would not recognise as already provisioned — every future boot would reformat and wipe the app's data", label, contents.Label)
				}
			})
		}
	}
}
