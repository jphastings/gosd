package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/inject"
)

func TestResolveBoardsDefaultsToAll(t *testing.T) {
	got, err := resolveBoards(nil)
	if err != nil {
		t.Fatalf("resolveBoards(nil): %v", err)
	}
	if !reflect.DeepEqual(got, boards.All()) {
		t.Errorf("resolveBoards(nil) = %v, want all registered boards %v", got, boards.All())
	}
}

func TestResolveBoardsFiltersAndDeduplicates(t *testing.T) {
	got, err := resolveBoards([]string{"pi-zero-2w", "pi-zero-2w"})
	if err != nil {
		t.Fatalf("resolveBoards: %v", err)
	}
	if len(got) != 1 || got[0].Name() != "pi-zero-2w" {
		t.Errorf("resolveBoards([pi-zero-2w, pi-zero-2w]) = %v, want a single pi-zero-2w entry", got)
	}
}

func TestResolveBoardsRejectsUnknownBoard(t *testing.T) {
	if _, err := resolveBoards([]string{"not-a-board"}); err == nil {
		t.Fatal("resolveBoards([not-a-board]) succeeded, want an error")
	}
}

func TestValidateUsbGadgetSkippedWhenFlagNotSet(t *testing.T) {
	incapable := []boards.Board{mustFindBoard(t, "nanopi-zero2")}
	if err := validateUsbGadget(incapable, false); err != nil {
		t.Errorf("validateUsbGadget(nanopi-zero2, false) = %v, want nil: --usb-gadget wasn't passed", err)
	}
}

func TestValidateUsbGadgetRejectsIncapableBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "nanopi-zero2")}

	err := validateUsbGadget(selected, true)
	if err == nil {
		t.Fatal("validateUsbGadget([nanopi-zero2], true) succeeded, want an error")
	}
	for _, want := range []string{"nanopi-zero2", "COMPATIBILITY.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validateUsbGadget error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestValidateUsbGadgetRejectsPi3B(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-3b")}

	err := validateUsbGadget(selected, true)
	if err == nil {
		t.Fatal("validateUsbGadget([pi-3b], true) succeeded, want an error: the 3B's USB is hard-wired through its LAN9514 hub")
	}
	for _, want := range []string{"pi-3b", "LAN9514", "COMPATIBILITY.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validateUsbGadget error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestValidateUsbGadgetAcceptsCapableBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}
	if err := validateUsbGadget(selected, true); err != nil {
		t.Errorf("validateUsbGadget([pi-zero-2w], true) = %v, want nil: pi-zero-2w supports --usb-gadget", err)
	}
}

func TestValidateUsbGadgetMixedBoardsNamesOnlyTheIncapableOne(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w"), mustFindBoard(t, "nanopi-zero2")}

	err := validateUsbGadget(selected, true)
	if err == nil {
		t.Fatal("validateUsbGadget([pi-zero-2w, nanopi-zero2], true) succeeded, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nanopi-zero2") {
		t.Errorf("validateUsbGadget error = %q, want it to name nanopi-zero2", msg)
	}
	if !strings.Contains(msg, "--board") {
		t.Errorf("validateUsbGadget error = %q, want it to suggest restricting with --board since pi-zero-2w does support --usb-gadget", msg)
	}
	if !strings.Contains(msg, "pi-zero-2w") {
		t.Errorf("validateUsbGadget error = %q, want it to name the capable board pi-zero-2w as the suggested restriction", msg)
	}
}

func TestValidateUsbGadgetAllBoardsDefaultOnlyNamesIncapableOnes(t *testing.T) {
	// boards.All() is the no---board default build set, which now
	// includes nanopi-zero2 (public since bean gosd-wskc) - the exact
	// scenario that produced the confusing runtime "build with gosd build
	// --usb-gadget" advice this check exists to catch earlier.
	err := validateUsbGadget(boards.All(), true)
	if err == nil {
		t.Fatal("validateUsbGadget(boards.All(), true) succeeded, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nanopi-zero2") {
		t.Errorf("validateUsbGadget error = %q, want it to name nanopi-zero2", msg)
	}
	for _, capableBoard := range []string{"pi-zero-2w", "pi-zero-w", "radxa-zero-3e", "rock-4se"} {
		if strings.Contains(msg, capableBoard) && !strings.Contains(msg, "--board") {
			t.Errorf("validateUsbGadget error mentions capable board %q outside of a --board suggestion: %q", capableBoard, msg)
		}
	}
}

func TestValidateConsoleBaudRateAcceptsUnset(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	if err := validateConsoleBaudRate(cmd, 0); err != nil {
		t.Errorf("validateConsoleBaudRate(0) = %v, want nil: 0 means --console-baud wasn't passed", err)
	}
	if buf.Len() != 0 {
		t.Errorf("validateConsoleBaudRate(0) printed %q, want no warning for the unset sentinel", buf.String())
	}
}

func TestValidateConsoleBaudRateRejectsNegative(t *testing.T) {
	cmd := &cobra.Command{}
	if err := validateConsoleBaudRate(cmd, -1); err == nil {
		t.Fatal("validateConsoleBaudRate(-1) succeeded, want an error")
	}
}

func TestValidateConsoleBaudRateAcceptsCommonRateSilently(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	if err := validateConsoleBaudRate(cmd, 115200); err != nil {
		t.Errorf("validateConsoleBaudRate(115200) = %v, want nil", err)
	}
	if buf.Len() != 0 {
		t.Errorf("validateConsoleBaudRate(115200) printed %q, want no warning for a common rate", buf.String())
	}
}

func TestValidateConsoleBaudRateWarnsOnUncommonRateButSucceeds(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	if err := validateConsoleBaudRate(cmd, 42); err != nil {
		t.Errorf("validateConsoleBaudRate(42) = %v, want nil: uncommon rates warn, they don't fail", err)
	}
	if !strings.Contains(buf.String(), "42") {
		t.Errorf("validateConsoleBaudRate(42) printed %q, want a warning mentioning the uncommon rate", buf.String())
	}
}

func TestValidateConsoleBaudSkippedWhenUnset(t *testing.T) {
	incapable := []boards.Board{mustFindBoard(t, "qemu-virt")}
	if err := validateConsoleBaud(incapable, 0); err != nil {
		t.Errorf("validateConsoleBaud(qemu-virt, 0) = %v, want nil: --console-baud wasn't passed", err)
	}
}

func TestValidateConsoleBaudRejectsIncapableBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "qemu-virt")}

	err := validateConsoleBaud(selected, 115200)
	if err == nil {
		t.Fatal("validateConsoleBaud([qemu-virt], 115200) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "qemu-virt") {
		t.Errorf("validateConsoleBaud error = %q, want it to mention qemu-virt", err.Error())
	}
}

func TestValidateConsoleBaudAcceptsCapableBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}
	if err := validateConsoleBaud(selected, 115200); err != nil {
		t.Errorf("validateConsoleBaud([pi-zero-2w], 115200) = %v, want nil: pi-zero-2w supports --console-baud", err)
	}
}

func TestValidateConsoleBaudMixedBoardsNamesOnlyTheIncapableOne(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w"), mustFindBoard(t, "qemu-virt")}

	err := validateConsoleBaud(selected, 115200)
	if err == nil {
		t.Fatal("validateConsoleBaud([pi-zero-2w, qemu-virt], 115200) succeeded, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "qemu-virt") {
		t.Errorf("validateConsoleBaud error = %q, want it to name qemu-virt", msg)
	}
	if !strings.Contains(msg, "--board") {
		t.Errorf("validateConsoleBaud error = %q, want it to suggest restricting with --board since pi-zero-2w does support --console-baud", msg)
	}
	if !strings.Contains(msg, "pi-zero-2w") {
		t.Errorf("validateConsoleBaud error = %q, want it to name the capable board pi-zero-2w as the suggested restriction", msg)
	}
}

func TestDeriveAppNameFromDotUsesWorkingDirectoryName(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "widget-3")
	if err := os.Mkdir(appDir, 0o755); err != nil {
		t.Fatalf("creating fixture app directory: %v", err)
	}
	t.Chdir(appDir)

	got, err := deriveAppName(".")
	if err != nil {
		t.Fatalf(`deriveAppName("."): %v`, err)
	}
	if got != "widget-3" {
		t.Errorf(`deriveAppName(".") = %q, want "widget-3"`, got)
	}
}

func TestDeriveAppNameFromRelativePath(t *testing.T) {
	got, err := deriveAppName("./examples/hello")
	if err != nil {
		t.Fatalf("deriveAppName: %v", err)
	}
	if got != "hello" {
		t.Errorf(`deriveAppName("./examples/hello") = %q, want "hello"`, got)
	}
}

func TestParseDataSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1GiB", 1024 * 1024 * 1024},
		{"1gib", 1024 * 1024 * 1024},
		{"512MiB", 512 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
		{"64K", 64 * 1024},
		{"4096", 4096},
		{" 8 MiB ", 8 * 1024 * 1024},
	}
	for _, c := range cases {
		got, expand, err := parseDataSize(c.in, diskfmt.FAT32)
		if err != nil {
			t.Errorf("parseDataSize(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want || expand {
			t.Errorf("parseDataSize(%q) = (%d, expand=%v), want (%d, expand=false)", c.in, got, expand, c.want)
		}
	}
}

func TestParseDataSizeExpandKeyword(t *testing.T) {
	for _, in := range []string{"expand", "Expand", "EXPAND", " expand "} {
		bytes, expand, err := parseDataSize(in, diskfmt.FAT32)
		if err != nil {
			t.Errorf("parseDataSize(%q) error: %v", in, err)
			continue
		}
		if !expand || bytes != 0 {
			t.Errorf("parseDataSize(%q) = (%d, expand=%v), want (0, expand=true)", in, bytes, expand)
		}
	}
}

func TestParseDataSizeRejectsInvalidValues(t *testing.T) {
	for _, in := range []string{"", "-1", "-1GiB", "1GB", "lots", "1.5GiB", "expanded"} {
		if _, _, err := parseDataSize(in, diskfmt.FAT32); err == nil {
			t.Errorf("parseDataSize(%q) succeeded, want an error", in)
		}
	}
}

// TestParseDataSizeRefusesMoreThanFAT32CanHold is bean gosd-mt53's guard: a
// --data-size past the largest FAT32 volume GoSD can lay out used to build an
// image whose GOSD-DATA partition was silently corrupt (bean gosd-8kdm).
func TestParseDataSizeRefusesMoreThanFAT32CanHold(t *testing.T) {
	maxBytes := diskfmt.MaxFAT32Bytes()

	for _, tc := range []struct {
		name    string
		in      string
		refused bool
	}{
		{"a 64GiB card's worth", "64GiB", false},
		{"the round 256GiB --data-size=expand caps at", "256GiB", false},
		{"the exact maximum, in bytes", strconv.FormatInt(maxBytes, 10), false},
		{"one sector past the maximum", strconv.FormatInt(maxBytes+512, 10), true},
		{"a 400GiB partition", "400GiB", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := parseDataSize(tc.in, diskfmt.FAT32)
			if (err != nil) != tc.refused {
				t.Fatalf("parseDataSize(%q) = (%d, %v), want refused = %v", tc.in, got, err, tc.refused)
			}
			if err == nil {
				return
			}
			// The refusal has to say how big is too big, and where the
			// whole story is written down.
			for _, want := range []string{diskfmt.GibibytesString(maxBytes), strconv.FormatInt(maxBytes, 10), "exFAT", dataSizeLimitDocsURL} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not mention %q", err, want)
				}
			}
		})
	}
}

// TestParseDataSizeEXT4RequiresANonZeroSize confirms --data-filesystem=ext4
// refuses --data-size=0 (the default): there is no partition for it to
// format at all.
func TestParseDataSizeEXT4RequiresANonZeroSize(t *testing.T) {
	_, _, err := parseDataSize("0", diskfmt.EXT4)
	if err == nil {
		t.Fatal("parseDataSize(\"0\", ext4) succeeded, want a refusal")
	}
	for _, want := range []string{"--data-filesystem=ext4", "--data-size"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err, want)
		}
	}
}

// TestParseDataSizeEXT4RefusesBelowTheGoldenMinimum confirms a --data-size
// smaller than diskfmt.MinEXT4Bytes() is refused for ext4, naming the
// minimum and the remedy - bean gosd-95yu's floor, mirroring
// TestParseDataSizeRefusesMoreThanFAT32CanHold's ceiling check for FAT32.
func TestParseDataSizeEXT4RefusesBelowTheGoldenMinimum(t *testing.T) {
	minBytes := diskfmt.MinEXT4Bytes()

	for _, tc := range []struct {
		name    string
		in      string
		refused bool
	}{
		{"one byte below the minimum", strconv.FormatInt(minBytes-1, 10), true},
		{"the exact minimum", strconv.FormatInt(minBytes, 10), false},
		{"comfortably above the minimum", "1GiB", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseDataSize(tc.in, diskfmt.EXT4)
			if (err != nil) != tc.refused {
				t.Fatalf("parseDataSize(%q, ext4) = %v, want refused = %v", tc.in, err, tc.refused)
			}
			if err == nil {
				return
			}
			for _, want := range []string{strconv.FormatInt(minBytes, 10), diskfmt.EXT4SizeLimitReason, "--data-filesystem=fat32"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not mention %q", err, want)
				}
			}
		})
	}
}

// TestParseDataSizeEXT4AcceptsExpandWithNoFloorCheck confirms
// --data-size=expand is valid for ext4 with no minimum-size refusal: it
// carries no --data-size number to compare against diskfmt.MinEXT4Bytes(),
// and gosd-init always fills the whole remaining card, which comfortably
// clears the golden image's floor.
func TestParseDataSizeEXT4AcceptsExpandWithNoFloorCheck(t *testing.T) {
	bytes, expand, err := parseDataSize("expand", diskfmt.EXT4)
	if err != nil {
		t.Fatalf("parseDataSize(\"expand\", ext4) = %v, want nil", err)
	}
	if !expand || bytes != 0 {
		t.Errorf("parseDataSize(\"expand\", ext4) = (%d, expand=%v), want (0, expand=true)", bytes, expand)
	}
}

// TestParseDataSizeEXT4HasNoFAT32Ceiling confirms the 256GiB FAT32-formatter
// ceiling (TestParseDataSizeRefusesMoreThanFAT32CanHold) does not apply to
// ext4, which grows via EXT4_IOC_RESIZE_FS rather than GoSD's own pure-Go
// FAT32 writer.
func TestParseDataSizeEXT4HasNoFAT32Ceiling(t *testing.T) {
	overFAT32Ceiling := strconv.FormatInt(diskfmt.MaxFAT32Bytes()+512, 10)
	if _, _, err := parseDataSize(overFAT32Ceiling, diskfmt.EXT4); err != nil {
		t.Errorf("parseDataSize(%q, ext4) = %v, want nil: the FAT32 ceiling must not apply to ext4", overFAT32Ceiling, err)
	}
}

func TestParseDataFilesystemAcceptsKnownValues(t *testing.T) {
	for in, want := range map[string]diskfmt.FS{
		"fat32":  diskfmt.FAT32,
		"FAT32":  diskfmt.FAT32,
		"ext4":   diskfmt.EXT4,
		"EXT4":   diskfmt.EXT4,
		" ext4 ": diskfmt.EXT4,
	} {
		got, err := parseDataFilesystem(in)
		if err != nil {
			t.Errorf("parseDataFilesystem(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseDataFilesystem(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseDataFilesystemRejectsUnknownValue(t *testing.T) {
	_, err := parseDataFilesystem("exfat")
	if err == nil {
		t.Fatal("parseDataFilesystem(\"exfat\") succeeded, want an error")
	}
	for _, want := range []string{"--data-filesystem", "fat32", "ext4"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestValidateDataFlushExt4ConflictSkippedWithoutBoth(t *testing.T) {
	if err := validateDataFlushExt4Conflict(diskfmt.FAT32, true); err != nil {
		t.Errorf("validateDataFlushExt4Conflict(fat32, flush=true) = %v, want nil: only ext4+flush conflicts", err)
	}
	if err := validateDataFlushExt4Conflict(diskfmt.EXT4, false); err != nil {
		t.Errorf("validateDataFlushExt4Conflict(ext4, flush=false) = %v, want nil: --data-flush wasn't passed", err)
	}
}

func TestValidateDataFlushExt4ConflictRejectsBoth(t *testing.T) {
	err := validateDataFlushExt4Conflict(diskfmt.EXT4, true)
	if err == nil {
		t.Fatal("validateDataFlushExt4Conflict(ext4, flush=true) succeeded, want an error")
	}
	for _, want := range []string{"--data-filesystem=ext4", "--data-flush"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestValidateDataFilesystemSupportSkippedForFAT32(t *testing.T) {
	incapable := []boards.Board{mustFindBoard(t, "pi-zero-2w")}
	if err := validateDataFilesystemSupport(incapable, diskfmt.FAT32); err != nil {
		t.Errorf("validateDataFilesystemSupport(pi-zero-2w, fat32) = %v, want nil: fat32 is always supported", err)
	}
}

func TestValidateDataFilesystemSupportRejectsIncapableBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	err := validateDataFilesystemSupport(selected, diskfmt.EXT4)
	if err == nil {
		t.Fatal("validateDataFilesystemSupport([pi-zero-2w], ext4) succeeded, want an error")
	}
	for _, want := range []string{"pi-zero-2w", "COMPATIBILITY.md", "--data-filesystem=ext4"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestValidateDataFilesystemSupportAcceptsCapableBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "radxa-zero-3e")}
	if err := validateDataFilesystemSupport(selected, diskfmt.EXT4); err != nil {
		t.Errorf("validateDataFilesystemSupport([radxa-zero-3e], ext4) = %v, want nil: radxa-zero-3e supports ext4", err)
	}
}

func TestValidateDataFilesystemSupportMixedBoardsNamesOnlyTheIncapableOneAndSuggestsBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "radxa-zero-3e"), mustFindBoard(t, "pi-zero-2w")}

	err := validateDataFilesystemSupport(selected, diskfmt.EXT4)
	if err == nil {
		t.Fatal("validateDataFilesystemSupport([radxa-zero-3e, pi-zero-2w], ext4) succeeded, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "pi-zero-2w") {
		t.Errorf("error = %q, want it to name the incapable board pi-zero-2w", msg)
	}
	if !strings.Contains(msg, "--board") {
		t.Errorf("error = %q, want it to suggest restricting with --board since radxa-zero-3e does support ext4", msg)
	}
	if !strings.Contains(msg, "radxa-zero-3e") {
		t.Errorf("error = %q, want it to name the capable board radxa-zero-3e as the suggested restriction", msg)
	}
}

// TestDefaultBootSizeMatchesImagePackage pins defaultBootSize's parsed value
// against image.DefaultBootPartitionSizeBytes: the flag's default and the
// image package's zero-means-default fallback must agree, or `gosd build`
// with no --boot-size would report a different size than it actually wrote.
func TestDefaultBootSizeMatchesImagePackage(t *testing.T) {
	got, err := parseBootSize(defaultBootSize)
	if err != nil {
		t.Fatalf("parseBootSize(defaultBootSize) failed: %v", err)
	}
	if got != image.DefaultBootPartitionSizeBytes {
		t.Errorf("parseBootSize(%q) = %d, want image.DefaultBootPartitionSizeBytes (%d)", defaultBootSize, got, int64(image.DefaultBootPartitionSizeBytes))
	}
}

func TestParseBootSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"256MiB", 256 * 1024 * 1024},
		{"1GiB", 1024 * 1024 * 1024},
		{"1gib", 1024 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
		{"512MiB", 512 * 1024 * 1024},
		{" 8 MiB ", 8 * 1024 * 1024},
	}
	for _, c := range cases {
		got, err := parseBootSize(c.in)
		if err != nil {
			t.Errorf("parseBootSize(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseBootSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestParseBootSizeRejectsInvalidValues covers non-numeric/negative input,
// the same shape TestParseDataSizeRejectsInvalidValues checks for
// --data-size, plus --boot-size's own bounds: too small to plausibly hold
// anything (a likely missing unit suffix), too large for the FAT32
// formatter, and not a whole number of MiB.
func TestParseBootSizeRejectsInvalidValues(t *testing.T) {
	for _, in := range []string{"", "-1", "-1GiB", "1GB", "lots", "1.5GiB", "expand", "0"} {
		if _, err := parseBootSize(in); err == nil {
			t.Errorf("parseBootSize(%q) succeeded, want an error", in)
		}
	}
}

// TestParseBootSizeRejectsTooSmallToBePlausible confirms a --boot-size well
// under any real kernel+initramfs (most likely a missing unit suffix, e.g.
// "256" meaning 256 bytes) is refused at flag-parse time with an actionable
// error, rather than silently attempted and failing deep inside go-diskfs.
func TestParseBootSizeRejectsTooSmallToBePlausible(t *testing.T) {
	for _, in := range []string{"1", "256", "1023KiB"} {
		_, err := parseBootSize(in)
		if err == nil {
			t.Fatalf("parseBootSize(%q) succeeded, want a too-small refusal", in)
		}
		if !strings.Contains(err.Error(), "--boot-size") {
			t.Errorf("parseBootSize(%q) error %q does not mention --boot-size", in, err)
		}
	}
}

// TestParseBootSizeRejectsMisalignedValues confirms a --boot-size that isn't
// a whole number of MiB is refused with a rounded suggestion, rather than
// silently truncated to the nearest sector (image.computeLayout's own
// sector-rounding would otherwise make the flag's value and the partition's
// real size quietly diverge).
func TestParseBootSizeRejectsMisalignedValues(t *testing.T) {
	for _, in := range []string{"10485761", "2049KiB"} { // 1 byte, and 1KiB, past a whole MiB
		_, err := parseBootSize(in)
		if err == nil {
			t.Fatalf("parseBootSize(%q) succeeded, want a misalignment refusal", in)
		}
		if !strings.Contains(err.Error(), "whole number of MiB") {
			t.Errorf("parseBootSize(%q) error %q does not mention MiB alignment", in, err)
		}
	}
}

// TestParseBootSizeRefusesMoreThanFAT32CanHold mirrors
// TestParseDataSizeRefusesMoreThanFAT32CanHold: --boot-size is bounded by the
// same FAT32 formatter ceiling as --data-size.
func TestParseBootSizeRefusesMoreThanFAT32CanHold(t *testing.T) {
	maxBytes := diskfmt.MaxFAT32Bytes()
	// --boot-size also requires MiB alignment (TestParseBootSizeRejects
	// MisalignedValues), and maxBytes itself isn't a whole MiB, so the
	// largest value --boot-size actually accepts is one MiB below it.
	maxAlignedBytes := (maxBytes / bootSizeAlignmentBytes) * bootSizeAlignmentBytes

	for _, tc := range []struct {
		name    string
		in      string
		refused bool
	}{
		{"a 1GiB boot volume", "1GiB", false},
		{"the largest MiB-aligned value at or under the maximum", strconv.FormatInt(maxAlignedBytes, 10), false},
		{"one sector past the maximum", strconv.FormatInt(maxBytes+512, 10), true},
		{"a 400GiB boot volume", "400GiB", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBootSize(tc.in)
			if (err != nil) != tc.refused {
				t.Fatalf("parseBootSize(%q) = (%d, %v), want refused = %v", tc.in, got, err, tc.refused)
			}
			if err == nil {
				return
			}
			for _, want := range []string{diskfmt.GibibytesString(maxBytes), strconv.FormatInt(maxBytes, 10), "--boot-size"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not mention %q", err, want)
				}
			}
		})
	}
}

func mustFindBoard(t *testing.T, id string) boards.Board {
	t.Helper()
	b, ok := boards.Find(id)
	if !ok {
		t.Fatalf("boards.Find(%q) = not found; registered boards: %v", id, boards.IDs())
	}
	return b
}

func TestResolveOutputsSingleBoardUsesOutputAsFile(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	got, err := resolveOutputs(selected, "myapp", "/tmp/x.img")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	if got["pi-zero-2w"] != "/tmp/x.img" {
		t.Errorf("outputs[pi-zero-2w] = %q, want /tmp/x.img", got["pi-zero-2w"])
	}
}

func TestResolveOutputsSingleBoardDefaultsToAppNameBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}

	got, err := resolveOutputs(selected, "myapp", "")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	if got["pi-zero-2w"] != "myapp-pi-zero-2w.img" {
		t.Errorf("outputs[pi-zero-2w] = %q, want myapp-pi-zero-2w.img", got["pi-zero-2w"])
	}
}

func TestResolveOutputsMultiBoardTreatsOutputAsDirectory(t *testing.T) {
	selected := boards.All()

	got, err := resolveOutputs(selected, "myapp", "/tmp/out")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	for _, b := range selected {
		want := "/tmp/out/myapp-" + b.Name() + ".img"
		if got[b.Name()] != want {
			t.Errorf("outputs[%s] = %q, want %q", b.Name(), got[b.Name()], want)
		}
	}
}

func TestEnsureOutputDirCreatesMissingMultiBoardDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")

	if err := ensureOutputDir(dir, true); err != nil {
		t.Fatalf("ensureOutputDir(%q, true): %v", dir, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("ensureOutputDir(%q, true) did not create a directory there", dir)
	}
}

func TestEnsureOutputDirCreatesMissingSingleBoardParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "app-board.img")

	if err := ensureOutputDir(path, false); err != nil {
		t.Fatalf("ensureOutputDir(%q, false): %v", path, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Errorf("ensureOutputDir(%q, false) did not create the parent directory", path)
	}
}

func TestEnsureOutputDirMultiBoardRejectsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}

	err := ensureOutputDir(path, true)
	if err == nil {
		t.Fatalf("ensureOutputDir(%q, true) succeeded, want an error", path)
	}
	if got, want := err.Error(), "-o must be a directory when building multiple boards"; !strings.Contains(got, want) {
		t.Errorf("ensureOutputDir error = %q, want it to contain %q", got, want)
	}
}

func TestEnsureOutputDirEmptySingleBoardOutputIsNoop(t *testing.T) {
	if err := ensureOutputDir("", false); err != nil {
		t.Errorf("ensureOutputDir(\"\", false) = %v, want nil", err)
	}
}

func TestParseEnvFlagsNil(t *testing.T) {
	got, err := parseEnvFlags(nil)
	if err != nil {
		t.Fatalf("parseEnvFlags(nil): %v", err)
	}
	if got != nil {
		t.Errorf("parseEnvFlags(nil) = %v, want nil", got)
	}
}

func TestParseEnvFlagsValid(t *testing.T) {
	got, err := parseEnvFlags([]string{"API_URL=https://example.com", "LOG_LEVEL=debug"})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	want := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseEnvFlags = %v, want %v", got, want)
	}
}

func TestParseEnvFlagsEmptyValueIsOK(t *testing.T) {
	got, err := parseEnvFlags([]string{"FOO="})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	if v, ok := got["FOO"]; !ok || v != "" {
		t.Errorf(`parseEnvFlags(["FOO="]) = %v, want {"FOO": ""}`, got)
	}
}

func TestParseEnvFlagsValueContainingEqualsSplitsOnFirst(t *testing.T) {
	got, err := parseEnvFlags([]string{"CONN=user=admin;pass=secret"})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	if want := "user=admin;pass=secret"; got["CONN"] != want {
		t.Errorf("parseEnvFlags CONN = %q, want %q", got["CONN"], want)
	}
}

func TestParseEnvFlagsRejectsMissingEquals(t *testing.T) {
	_, err := parseEnvFlags([]string{"NOEQUALS"})
	if err == nil {
		t.Fatal("parseEnvFlags([NOEQUALS]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--env needs KEY=VALUE") {
		t.Errorf("error = %q, want it to mention KEY=VALUE", err.Error())
	}
}

func TestParseEnvFlagsRejectsInvalidKeyChars(t *testing.T) {
	for _, in := range []string{"1STARTSWITHDIGIT=x", "HAS-HYPHEN=x", "HAS SPACE=x", "=emptykey"} {
		if _, err := parseEnvFlags([]string{in}); err == nil {
			t.Errorf("parseEnvFlags([%q]) succeeded, want an error", in)
		}
	}
}

func TestParseEnvFlagsRejectsReservedGosdNamespace(t *testing.T) {
	_, err := parseEnvFlags([]string{"GOSD_FOO=bar"})
	if err == nil {
		t.Fatal("parseEnvFlags([GOSD_FOO=bar]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "GOSD_") {
		t.Errorf("error = %q, want it to mention the GOSD_ namespace", err.Error())
	}
}

func TestParseEnvFlagsRejectsDuplicateKey(t *testing.T) {
	_, err := parseEnvFlags([]string{"FOO=one", "FOO=two"})
	if err == nil {
		t.Fatal("parseEnvFlags with a duplicate key succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "FOO") {
		t.Errorf("error = %q, want it to mention the duplicate key FOO", err.Error())
	}
}

func TestParsePlaceholderFlagsNil(t *testing.T) {
	got, err := parsePlaceholderFlags(nil)
	if err != nil {
		t.Fatalf("parsePlaceholderFlags(nil): %v", err)
	}
	if got != nil {
		t.Errorf("parsePlaceholderFlags(nil) = %v, want nil", got)
	}
}

func TestParsePlaceholderFlagsValid(t *testing.T) {
	got, err := parsePlaceholderFlags([]string{"backupist.yaml=32KiB", "network-config=4096"})
	if err != nil {
		t.Fatalf("parsePlaceholderFlags: %v", err)
	}
	want := []inject.Placeholder{
		{Path: "backupist.yaml", SizeBytes: 32 * 1024},
		{Path: "network-config", SizeBytes: 4096},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePlaceholderFlags = %v, want %v", got, want)
	}
}

func TestParsePlaceholderFlagsRejectsMissingEquals(t *testing.T) {
	_, err := parsePlaceholderFlags([]string{"backupist.yaml"})
	if err == nil {
		t.Fatal("parsePlaceholderFlags([backupist.yaml]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--placeholder <path>=<size>") {
		t.Errorf("error = %q, want it to show the expected shape", err.Error())
	}
}

func TestParsePlaceholderFlagsRejectsBadSize(t *testing.T) {
	_, err := parsePlaceholderFlags([]string{"backupist.yaml=not-a-size"})
	if err == nil {
		t.Fatal("parsePlaceholderFlags with an unparseable size succeeded, want an error")
	}
}

func TestParsePlaceholderFlagsRejectsSizeBelowMinimum(t *testing.T) {
	_, err := parsePlaceholderFlags([]string{"backupist.yaml=1"})
	if err == nil {
		t.Fatal("parsePlaceholderFlags with a 1-byte size succeeded, want an error (too small for the rendered header)")
	}
}

func TestParsePlaceholderFlagsRejectsDuplicatePathDifferingOnlyByCase(t *testing.T) {
	_, err := parsePlaceholderFlags([]string{"backupist.yaml=32KiB", "Backupist.Yaml=32KiB"})
	if err == nil {
		t.Fatal("parsePlaceholderFlags with paths differing only by case succeeded, want an error (FAT is case-insensitive)")
	}
	if !strings.Contains(err.Error(), "backupist.yaml") || !strings.Contains(err.Error(), "Backupist.Yaml") {
		t.Errorf("error = %q, want it to name both colliding paths", err.Error())
	}
}

func TestResolveOutputsMultiBoardDefaultsToCurrentDirectory(t *testing.T) {
	selected := boards.All()

	got, err := resolveOutputs(selected, "myapp", "")
	if err != nil {
		t.Fatalf("resolveOutputs: %v", err)
	}
	for _, b := range selected {
		want := "myapp-" + b.Name() + ".img"
		if got[b.Name()] != want {
			t.Errorf("outputs[%s] = %q, want %q", b.Name(), got[b.Name()], want)
		}
	}
}

func TestGosdInitSrcFlagDefaultsToEnv(t *testing.T) {
	t.Setenv("GOSD_INIT_SRC", "/nix/store/example-gosd-src")

	flag := newBuildCmd().Flags().Lookup("gosd-init-src")
	if flag == nil {
		t.Fatal("build command has no --gosd-init-src flag")
	}
	if flag.DefValue != "/nix/store/example-gosd-src" {
		t.Errorf("--gosd-init-src default = %q, want the GOSD_INIT_SRC env value (the package-manager hook)", flag.DefValue)
	}
}

func TestGosdInitSrcFlagDefaultsEmptyWithoutEnv(t *testing.T) {
	t.Setenv("GOSD_INIT_SRC", "")

	flag := newBuildCmd().Flags().Lookup("gosd-init-src")
	if flag == nil {
		t.Fatal("build command has no --gosd-init-src flag")
	}
	if flag.DefValue != "" {
		t.Errorf("--gosd-init-src default = %q, want empty when GOSD_INIT_SRC is unset", flag.DefValue)
	}
}
