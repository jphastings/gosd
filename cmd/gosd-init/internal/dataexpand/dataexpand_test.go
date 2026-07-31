package dataexpand

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/diskfmt"
)

// fakeCard scripts one block device for Run: an MBR, a size, what partition 2
// holds, and whether its node appears when the kernel is told about it. Every
// mutating call lands in actions, in order, so tests can assert both what
// happened and the crash-safety ordering (MBR on disk before the kernel add,
// both before the format).
type fakeCard struct {
	mbr        []byte
	sizeBytes  int64
	contents   diskfmt.Contents
	inspectErr error
	// marked is whether the partition already carries the completed-format
	// marker; markerErr makes reading it fail, as a half-written filesystem
	// with an unreadable root directory would.
	marked     bool
	markerErr  error
	nodeExists bool
	// nodeAppearsOnAdd simulates devtmpfs: the partition node shows up when
	// AddKernelPartition succeeds. Defaults true via newFakeCard.
	nodeAppearsOnAdd bool

	actions    []string
	wroteMBR   []byte
	addedStart int64
	addedSize  int64
	logs       []string
}

func newFakeCard(mbr []byte, sizeBytes int64) *fakeCard {
	return &fakeCard{mbr: mbr, sizeBytes: sizeBytes, nodeAppearsOnAdd: true}
}

func (c *fakeCard) deps() Deps {
	clock := struct {
		mu  sync.Mutex
		now time.Time
	}{now: time.Unix(0, 0)}

	return Deps{
		ReadMBR: func(string) ([]byte, error) {
			mbr := make([]byte, len(c.mbr))
			copy(mbr, c.mbr)
			return mbr, nil
		},
		WriteMBR: func(_ string, sector []byte) error {
			c.actions = append(c.actions, "write-mbr")
			c.wroteMBR = append([]byte(nil), sector...)
			return nil
		},
		DeviceSizeBytes: func(string) (int64, error) { return c.sizeBytes, nil },
		AddKernelPartition: func(_ string, partNo int, startBytes, sizeBytes int64) error {
			c.actions = append(c.actions, fmt.Sprintf("add-partition-%d", partNo))
			c.addedStart, c.addedSize = startBytes, sizeBytes
			if c.nodeAppearsOnAdd {
				c.nodeExists = true
			}
			return nil
		},
		Inspect:     func(string) (diskfmt.Contents, error) { return c.contents, c.inspectErr },
		FormatFAT32: func(_, label string) error { c.actions = append(c.actions, "format-"+label); return nil },
		CreateMarker: func(string) error {
			c.actions = append(c.actions, "write-marker")
			c.marked = true
			return nil
		},
		MarkerExists: func(string) (bool, error) { return c.marked, c.markerErr },
		SyncDevice:   func(string) error { c.actions = append(c.actions, "sync-partition"); return nil },
		PathExists:   func(string) bool { return c.nodeExists },
		Sleep: func(d time.Duration) {
			clock.mu.Lock()
			clock.now = clock.now.Add(d)
			clock.mu.Unlock()
		},
		Now: func() time.Time {
			clock.mu.Lock()
			defer clock.mu.Unlock()
			return clock.now
		},
		Log: func(format string, args ...any) { c.logs = append(c.logs, fmt.Sprintf(format, args...)) },
	}
}

func (c *fakeCard) logged(substr string) bool {
	for _, l := range c.logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func testOptions() Options {
	return Options{
		Device:          "/dev/mmcblk0",
		PartitionDevice: "/dev/mmcblk0p2",
		NodeTimeout:     5 * time.Second,
	}
}

// defaultDataStartLBA is where partition 2 belongs on an image built with the
// default 256MiB boot volume: 272MiB in. Nothing in the package knows this
// number any more — it is derived from each card's own table — so the tests
// state it independently.
const defaultDataStartLBA = (16 + 256) * 1024 * 1024 / sectorSize

// gosdMBR builds the MBR a freshly-flashed expand image carries: boot
// signature, partition 1 (FAT32-LBA at 16MiB, bootSizeBytes long), no
// partition 2.
func gosdMBR(bootSizeBytes int64) []byte {
	mbr := make([]byte, mbrSize)
	mbr[signatureOffset], mbr[signatureOffset+1] = 0x55, 0xAA
	entry := mbr[partitionEntriesOffset:]
	entry[4] = fatPartitionType
	binary.LittleEndian.PutUint32(entry[8:12], bootPartitionStartLBA)
	binary.LittleEndian.PutUint32(entry[12:16], uint32(bootSizeBytes/sectorSize))
	return mbr
}

// defaultMBR is the flashed table of an image built with the default boot
// volume size.
func defaultMBR() []byte { return gosdMBR(256 * 1024 * 1024) }

// withDataEntry returns defaultMBR plus a partition-2 entry, as a card looks
// after a completed (or interrupted-after-the-MBR-write) first boot.
func withDataEntry(sizeLBA uint32) []byte {
	mbr := defaultMBR()
	writeDataEntry(mbr, defaultDataStartLBA, sizeLBA)
	return mbr
}

func TestRunCreatesTheDataPartitionOnFirstBoot(t *testing.T) {
	const cardSize = 8 << 30 // an ordinary 8GiB card
	card := newFakeCard(defaultMBR(), cardSize)

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	// Crash safety hangs on this exact order: the MBR entry is the commit
	// record, written only after the formatted filesystem is durable, so
	// power loss at any earlier point leaves no entry and the next boot
	// redoes everything.
	wantActions := []string{"add-partition-2", "format-" + Label, "sync-partition", "write-marker", "sync-partition", "write-mbr"}
	if got := strings.Join(card.actions, ","); got != strings.Join(wantActions, ",") {
		t.Fatalf("actions = %v, want %v", card.actions, wantActions)
	}

	partType, start, size := readEntry(card.wroteMBR, dataPartitionNumber)
	wantSize := uint32(cardSize/sectorSize - defaultDataStartLBA) // 8GiB is already 4MiB-aligned
	if partType != fatPartitionType || start != defaultDataStartLBA || size != wantSize {
		t.Errorf("partition 2 entry = type %#02x start %d size %d, want type %#02x start %d size %d",
			partType, start, size, fatPartitionType, defaultDataStartLBA, wantSize)
	}
	if bootType, bootStart, _ := readEntry(card.wroteMBR, bootPartitionNumber); bootType != fatPartitionType || bootStart != bootPartitionStartLBA {
		t.Error("partition 1's entry was disturbed")
	}
	if card.addedStart != defaultDataStartLBA*sectorSize || card.addedSize != int64(wantSize)*sectorSize {
		t.Errorf("kernel partition registered as [%d, +%d), want [%d, +%d)",
			card.addedStart, card.addedSize, defaultDataStartLBA*sectorSize, int64(wantSize)*sectorSize)
	}
}

func TestRunPutsTheDataPartitionAfterAnyBootVolumeSize(t *testing.T) {
	// The boot volume's size is chosen per app at build time, so the only
	// thing that knows where partition 2 goes is the table the flash left on
	// the card.
	const cardSize = 8 << 30
	const bootSize = 1024 * 1024 * 1024 // an app that needs a 1GiB boot volume
	card := newFakeCard(gosdMBR(bootSize), cardSize)

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	wantStart := uint32((16*1024*1024 + bootSize) / sectorSize)
	wantSize := uint32(cardSize/sectorSize - int64(wantStart))
	_, start, size := readEntry(card.wroteMBR, dataPartitionNumber)
	if start != wantStart || size != wantSize {
		t.Errorf("partition 2 entry = start %d size %d, want start %d size %d", start, size, wantStart, wantSize)
	}
	if card.addedStart != int64(wantStart)*sectorSize {
		t.Errorf("kernel partition registered at byte %d, want %d", card.addedStart, int64(wantStart)*sectorSize)
	}
}

func TestRunAdoptsASurvivingDataPartitionAfterAReflash(t *testing.T) {
	// Reflashing rewrites the MBR (no partition 2) but never touches the
	// bytes beyond the boot partition, so the app's data is still there.
	card := newFakeCard(defaultMBR(), 8<<30)
	card.contents = diskfmt.Contents{FS: diskfmt.FAT32, Label: Label}
	card.marked = true

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	wantActions := []string{"add-partition-2", "write-mbr"}
	if got := strings.Join(card.actions, ","); got != strings.Join(wantActions, ",") {
		t.Fatalf("actions = %v, want %v (the survivor must not be formatted)", card.actions, wantActions)
	}
	partType, start, _ := readEntry(card.wroteMBR, dataPartitionNumber)
	if partType != fatPartitionType || start != defaultDataStartLBA {
		t.Errorf("partition 2 entry = type %#02x start %d, want type %#02x start %d",
			partType, start, fatPartitionType, defaultDataStartLBA)
	}
	if !card.logged("re-adopted") {
		t.Errorf("logs = %q, want a mention that the partition was re-adopted", card.logs)
	}
}

func TestRunFormatsWhateverIsNotASurvivingDataPartition(t *testing.T) {
	// The last two cases are the debris of an interrupted format: go-diskfs
	// writes the volume label last and syncs nothing along the way, so a
	// power cut can leave a volume that inspects as GOSD-DATA over
	// incomplete FAT tables. Only the marker — written after this package's
	// own sync barrier — separates that from a real survivor, and adopting
	// it would commit an MBR entry over a broken filesystem forever.
	cases := []struct {
		name      string
		contents  diskfmt.Contents
		marked    bool
		markerErr error
	}{
		{name: "a blank card", contents: diskfmt.Contents{Blank: true}},
		{name: "a foreign volume", contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "HOLIDAY"}},
		{name: "an unreadable filesystem", contents: diskfmt.Contents{OtherFS: "exFAT"}},
		{name: "mid-partition rubble", contents: diskfmt.Contents{}},
		{
			name:     "a labelled volume with no completion marker",
			contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: Label},
		},
		{
			name:      "a labelled volume whose root directory will not read",
			contents:  diskfmt.Contents{FS: diskfmt.FAT32, Label: Label},
			marked:    true,
			markerErr: errors.New("invalid argument"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			card := newFakeCard(defaultMBR(), 8<<30)
			card.contents, card.marked, card.markerErr = c.contents, c.marked, c.markerErr

			if err := Run(card.deps(), testOptions()); err != nil {
				t.Fatalf("Run() = %v, want nil", err)
			}
			wantActions := []string{"add-partition-2", "format-" + Label, "sync-partition", "write-marker", "sync-partition", "write-mbr"}
			if got := strings.Join(card.actions, ","); got != strings.Join(wantActions, ",") {
				t.Errorf("actions = %v, want %v", card.actions, wantActions)
			}
		})
	}
}

func TestRunRefusesToFormatContentsItCouldNotRead(t *testing.T) {
	card := newFakeCard(defaultMBR(), 8<<30)
	card.inspectErr = errors.New("I/O error")

	if err := Run(card.deps(), testOptions()); err == nil {
		t.Fatal("Run() = nil, want the read failure reported")
	}
	if got := strings.Join(card.actions, ","); got != "add-partition-2" {
		t.Errorf("actions = %v, want only the kernel registration — data that could not be seen must not be formatted over", card.actions)
	}
}

func TestRunResumesCleanlyAfterAnInterruptedFirstBoot(t *testing.T) {
	// Power loss between the marker's flush and the MBR write leaves the card
	// with no partition-2 entry over a finished, marked filesystem; the next
	// boot must reach the same committed table without reformatting it.
	card := newFakeCard(defaultMBR(), 8<<30)
	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("first boot: Run() = %v, want nil", err)
	}
	committed := card.wroteMBR

	resumed := newFakeCard(defaultMBR(), 8<<30) // the MBR write never landed
	resumed.contents = diskfmt.Contents{FS: diskfmt.FAT32, Label: Label}
	resumed.marked = card.marked // exactly what the first boot left behind
	if err := Run(resumed.deps(), testOptions()); err != nil {
		t.Fatalf("second boot: Run() = %v, want nil", err)
	}
	if strings.Contains(strings.Join(resumed.actions, ","), "format") {
		t.Errorf("second boot performed %v, want no reformat of the completed filesystem", resumed.actions)
	}
	if !bytes.Equal(resumed.wroteMBR, committed) {
		t.Error("the resumed boot committed a different partition table than the interrupted one would have")
	}
}

func TestRunAlignsThePartitionDownTo4MiB(t *testing.T) {
	const cardSize = 8<<30 + 1000*sectorSize // an untidy tail past the last 4MiB boundary
	card := newFakeCard(defaultMBR(), cardSize)

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	_, _, size := readEntry(card.wroteMBR, dataPartitionNumber)
	if size%(alignBytes/sectorSize) != 0 {
		t.Errorf("partition size %d sectors is not 4MiB-aligned", size)
	}
	if want := uint32(8<<30/sectorSize - defaultDataStartLBA); size != want {
		t.Errorf("partition size = %d sectors, want %d (tail dropped)", size, want)
	}
}

func TestRunLeavesAHealthyDataPartitionAlone(t *testing.T) {
	card := newFakeCard(withDataEntry(1<<21), 8<<30)
	card.nodeExists = true
	card.contents = diskfmt.Contents{FS: diskfmt.FAT32, Label: Label}

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if len(card.actions) != 0 {
		t.Errorf("a healthy later boot performed %v, want nothing", card.actions)
	}
	if !card.logged("already present") {
		t.Errorf("logs = %q, want a mention that the partition is already present", card.logs)
	}
}

func TestRunReportsCorruptionInsteadOfTouchingAnEstablishedPartition(t *testing.T) {
	// The entry is only ever written after a completed, synced format, so an
	// entry over anything but the GOSD-DATA filesystem means an established
	// partition — possibly holding app data — has been damaged. Nothing may
	// repair, reformat, or otherwise touch it.
	cases := []struct {
		name     string
		contents diskfmt.Contents
	}{
		{"blank space", diskfmt.Contents{Blank: true}},
		{"a foreign volume", diskfmt.Contents{FS: diskfmt.FAT32, Label: "HOLIDAY"}},
		{"unreadable content", diskfmt.Contents{OtherFS: "exFAT"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			card := newFakeCard(withDataEntry(1<<21), 8<<30)
			card.nodeExists = true
			card.contents = c.contents

			err := Run(card.deps(), testOptions())
			if !errors.Is(err, ErrDataCorrupt) {
				t.Fatalf("Run() = %v, want ErrDataCorrupt", err)
			}
			if len(card.actions) != 0 {
				t.Errorf("a corrupt partition saw %v, want nothing", card.actions)
			}
		})
	}
}

func TestRunReportsCorruptionWhenTheEstablishedNodeIsMissing(t *testing.T) {
	card := newFakeCard(withDataEntry(1<<21), 8<<30)
	card.nodeExists = false

	err := Run(card.deps(), testOptions())
	if !errors.Is(err, ErrDataCorrupt) {
		t.Fatalf("Run() = %v, want ErrDataCorrupt", err)
	}
	if len(card.actions) != 0 {
		t.Errorf("actions = %v, want nothing", card.actions)
	}
}

func TestRunSkipsACardWithNoRoom(t *testing.T) {
	// A card barely bigger than the 272MiB image: no partition is worth
	// creating, and the card must not be written to at all.
	card := newFakeCard(defaultMBR(), 300*1024*1024)

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil (no room is not an error)", err)
	}
	if len(card.actions) != 0 {
		t.Errorf("a card with no room saw %v, want nothing", card.actions)
	}
	if !card.logged("not creating a data partition") {
		t.Errorf("logs = %q, want an explanation that no partition was created", card.logs)
	}
}

func TestRunCapsThePartitionForTheFAT32Formatter(t *testing.T) {
	card := newFakeCard(defaultMBR(), 1<<40) // 1TiB

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	_, _, size := readEntry(card.wroteMBR, dataPartitionNumber)
	if want := uint32(maxPartitionBytes / sectorSize); size != want {
		t.Errorf("partition size = %d sectors, want the %d-sector cap", size, want)
	}
	if !card.logged("capping the data partition") {
		t.Errorf("logs = %q, want a mention of the cap", card.logs)
	}
}

func TestRunRefusesAForeignPartitionTable(t *testing.T) {
	noSignature := defaultMBR()
	noSignature[signatureOffset] = 0x00 // not a GoSD card

	// A partition 1 ending past the MBR's 32-bit sector range has no
	// expressible successor: the derived start would wrap.
	overflowing := defaultMBR()
	binary.LittleEndian.PutUint32(overflowing[partitionEntriesOffset+12:], math.MaxUint32)

	cases := []struct {
		name string
		mbr  []byte
	}{
		{"no boot signature", noSignature},
		// A partition 1 of no length would put the data partition on top of
		// the boot partition, so the derivation refuses it.
		{"a zero-length partition 1", gosdMBR(0)},
		{"a partition 1 ending past the MBR's addressing limit", overflowing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			card := newFakeCard(c.mbr, 8<<30)
			err := Run(card.deps(), testOptions())
			if err == nil || !strings.Contains(err.Error(), "leaving it untouched") {
				t.Fatalf("Run() = %v, want a refusal naming the foreign table", err)
			}
			if len(card.actions) != 0 {
				t.Errorf("a foreign card saw %v, want nothing", card.actions)
			}
		})
	}
}

func TestRunReportsANodeThatNeverAppears(t *testing.T) {
	card := newFakeCard(defaultMBR(), 8<<30)
	card.nodeAppearsOnAdd = false

	err := Run(card.deps(), testOptions())
	if err == nil || !strings.Contains(err.Error(), "did not appear") {
		t.Fatalf("Run() = %v, want a node-timeout error", err)
	}
	if errors.Is(err, ErrDataCorrupt) {
		t.Error("a node timeout during creation is a transient failure, not corruption")
	}
	// Nothing further may happen — in particular no MBR entry, which would
	// falsely commit a format that never ran.
	if got := strings.Join(card.actions, ","); got != "add-partition-2" {
		t.Errorf("actions = %v, want only the kernel registration", card.actions)
	}
}

func TestDataPartitionFor(t *testing.T) {
	cases := []struct {
		bootPartition, device, partition2 string
		ok                                bool
	}{
		{"/dev/mmcblk0p1", "/dev/mmcblk0", "/dev/mmcblk0p2", true},
		{"/dev/mmcblk1p1", "/dev/mmcblk1", "/dev/mmcblk1p2", true},
		{"/dev/vda1", "/dev/vda", "/dev/vda2", true},
		{"/dev/nvme0n1p1", "/dev/nvme0n1", "/dev/nvme0n1p2", true},
		{"/dev/sda1", "/dev/sda", "/dev/sda2", true},
		{"/dev/mmcblk0p2", "", "", false}, // not a first partition
		{"/dev/sda", "", "", false},
		{"1", "", "", false},
	}
	for _, c := range cases {
		device, partition2, ok := DataPartitionFor(c.bootPartition)
		if device != c.device || partition2 != c.partition2 || ok != c.ok {
			t.Errorf("DataPartitionFor(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.bootPartition, device, partition2, ok, c.device, c.partition2, c.ok)
		}
	}
}
