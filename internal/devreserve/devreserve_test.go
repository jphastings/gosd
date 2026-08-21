package devreserve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoversIsOneDirectional pins the rule the whole package turns on: a
// reservation refuses the disk that contains it, and never the siblings
// beside it. Reserving partition 1 must therefore refuse the whole card
// while leaving the data partition — the app's own storage — shareable.
func TestCoversIsOneDirectional(t *testing.T) {
	tests := []struct {
		name       string
		whole, dev string
		want       bool
	}{
		{"the device itself", "/dev/mmcblk0p1", "/dev/mmcblk0p1", true},
		{"the whole card contains its boot partition", "/dev/mmcblk0", "/dev/mmcblk0p1", true},
		{"the data partition contains no part of the boot partition", "/dev/mmcblk0p2", "/dev/mmcblk0p1", false},
		{"a sibling partition is not the disk", "/dev/mmcblk0p1", "/dev/mmcblk0", false},
		{"sd naming: whole device contains its partition", "/dev/sda", "/dev/sda1", true},
		{"nvme naming: whole device contains its partition", "/dev/nvme0n1", "/dev/nvme0n1p1", true},
		{"virtio naming: whole device contains its partition", "/dev/vda", "/dev/vda1", true},
		{"unrelated cards never match", "/dev/mmcblk1", "/dev/mmcblk0p1", false},
		{"an eMMC hardware partition is not a numbered partition", "/dev/mmcblk0", "/dev/mmcblk0boot0", false},
		{"a redundant path still matches the device it names", "/dev/../dev/mmcblk0", "/dev/mmcblk0p1", true},
		{"image files match only themselves", "/data/site.img", "/data/site.img", true},
		{"image files have no partitions", "/data/site.img", "/data/site.img2", false},
		{"an empty candidate matches nothing", "", "/dev/mmcblk0p1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Covers(tt.whole, tt.dev); got != tt.want {
				t.Errorf("Covers(%q, %q) = %v, want %v", tt.whole, tt.dev, got, tt.want)
			}
		})
	}
}

func TestExposesNamesTheReservationItFound(t *testing.T) {
	reserved := Reservations{
		{Path: "/dev/mmcblk0p1", Role: "the boot partition"},
		{Path: "/dev/mmcblk0p3", Role: "the settings partition"},
	}

	for _, candidate := range []string{"/dev/mmcblk0p3", "/dev/mmcblk0"} {
		entry, blocked := reserved.Exposes(candidate)
		if !blocked {
			t.Fatalf("Exposes(%q) = not blocked, want blocked", candidate)
		}
		if entry.Path == "" || entry.Role == "" {
			t.Errorf("Exposes(%q) returned %+v, want the entry it matched so a refusal can name it", candidate, entry)
		}
	}

	if _, blocked := reserved.Exposes("/dev/mmcblk0p2"); blocked {
		t.Error("Exposes(\"/dev/mmcblk0p2\") blocked the data partition, which no entry reserves")
	}
}

func TestEncodeParseRoundTrip(t *testing.T) {
	want := []Entry{{Path: "/dev/vda1", Role: "the boot partition this device started from"}}

	data, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Parse(Encode(%+v)) = %+v", want, got)
	}
}

func TestEncodeRefusesAPathlessEntry(t *testing.T) {
	if _, err := Encode([]Entry{{Role: "the boot partition"}}); err == nil {
		t.Fatal("Encode() = nil error for an entry that reserves nothing, want a refusal")
	}
}

// A publisher this reader is too old to understand must still get its
// devices refused: the role is quoted, never interpreted, and an unknown
// extra field is ignored rather than failing the whole file.
func TestParseAcceptsAnUnknownFieldFromANewerPublisher(t *testing.T) {
	got, err := Parse([]byte(`[{"path":"/dev/mmcblk0p3","role":"the settings partition","since":"v9"}]`))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Path != "/dev/mmcblk0p3" || got[0].Role != "the settings partition" {
		t.Errorf("Parse() = %+v, want the entry with its role intact", got)
	}
}

func TestParseRejectsWhatItCannotTrust(t *testing.T) {
	tests := map[string][]byte{
		"empty":         nil,
		"not JSON":      []byte("boot partition\n"),
		"not the shape": []byte(`{"path":"/dev/mmcblk0p1"}`),
		"oversized":     []byte(strings.Repeat("x", MaxBytes+1)),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(data); err == nil {
				t.Error("Parse() = nil error, want a refusal to trust the file")
			}
		})
	}
}

// A role is rendered straight into a refusal on a serial console, so a
// publisher that got one wrong loses the prose, never the reservation.
func TestParseKeepsThePathWhenTheRoleIsUnusable(t *testing.T) {
	long := strings.Repeat("é", MaxRoleBytes)
	data, err := Encode([]Entry{
		{Path: "/dev/mmcblk0p1", Role: "clears\rthe\nline"},
		{Path: "/dev/mmcblk0p2", Role: "the boot partition\u202e"},
		{Path: "/dev/mmcblk0p3", Role: long},
	})
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}

	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Fatalf("Parse() kept %d entries, want 3", len(got))
	}
	if got[0].Role != "" || got[0].Describe() == "" {
		t.Errorf("entry with a control-character role = %+v, want the role dropped but still described", got[0])
	}
	// U+202E carries no control character, and would reorder the text of a
	// refusal printed to a console.
	if got[1].Role != "" {
		t.Errorf("entry with a bidi-override role = %+v, want the role dropped", got[1])
	}
	if len(got[2].Role) > MaxRoleBytes || !strings.HasPrefix(long, got[2].Role) {
		t.Errorf("over-long role = %q, want it trimmed on a rune boundary within %d bytes", got[2].Role, MaxRoleBytes)
	}
}

// A gosd-init that predates the file publishes nothing, and a caller must
// carry on with whatever protection it already had — see the package doc.
func TestReadTreatsAMissingFileAsNoReservations(t *testing.T) {
	got, err := Read(filepath.Join(t.TempDir(), "reserved-devices.json"))
	if err != nil {
		t.Fatalf("Read() = %v, want nil for a file that isn't there", err)
	}
	if len(got) != 0 {
		t.Errorf("Read() = %+v, want no reservations", got)
	}
}

// The opposite case: a file that IS there but says something we can't read
// means we don't know what is reserved, which must be reported rather than
// read as "nothing is".
func TestReadReportsAFilePresentButUntrustworthy(t *testing.T) {
	name := filepath.Join(t.TempDir(), "reserved-devices.json")
	if err := os.WriteFile(name, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Read(name); err == nil {
		t.Fatal("Read() = nil error for an unparseable file, want a refusal to guess")
	}
}

func TestWriteIsReadableAndLeavesNoTemporary(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "gosd", "reserved-devices.json")
	want := []Entry{{Path: "/dev/mmcblk0p1", Role: "the boot partition this device started from"}}

	if err := Write(name, want); err != nil {
		t.Fatalf("Write() = %v, want nil", err)
	}

	got, err := Read(name)
	if err != nil {
		t.Fatalf("Read() = %v, want nil", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Read(Write(%+v)) = %+v", want, got)
	}
	if _, err := os.Stat(name + ".tmp"); err == nil {
		t.Error("Write() left its temporary file behind")
	}
}
