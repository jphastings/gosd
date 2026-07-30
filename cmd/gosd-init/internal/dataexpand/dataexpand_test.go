package dataexpand

import (
	"encoding/binary"
	"fmt"
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
		Inspect:     func(string) (diskfmt.Contents, error) { return c.contents, nil },
		FormatFAT32: func(_, label string) error { c.actions = append(c.actions, "format-"+label); return nil },
		PathExists:  func(string) bool { return c.nodeExists },
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

// gosdMBR builds the MBR a freshly-flashed expand image carries: boot
// signature, partition 1 (FAT32-LBA at 16MiB, 256MiB long), no partition 2.
func gosdMBR() []byte {
	mbr := make([]byte, mbrSize)
	mbr[signatureOffset], mbr[signatureOffset+1] = 0x55, 0xAA
	entry := mbr[partitionEntriesOffset:]
	entry[4] = fatPartitionType
	binary.LittleEndian.PutUint32(entry[8:12], bootPartitionStartLBA)
	binary.LittleEndian.PutUint32(entry[12:16], 256*1024*1024/sectorSize)
	return mbr
}

// withDataEntry returns gosdMBR plus a partition-2 entry, as a card looks
// after a completed (or interrupted-after-the-MBR-write) first boot.
func withDataEntry(sizeLBA uint32) []byte {
	mbr := gosdMBR()
	writeDataEntry(mbr, dataPartitionStartLBA, sizeLBA)
	return mbr
}

func TestRunCreatesTheDataPartitionOnFirstBoot(t *testing.T) {
	const cardSize = 8 << 30 // an ordinary 8GiB card
	card := newFakeCard(gosdMBR(), cardSize)

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	// Crash safety hangs on this exact order: the MBR is durable before the
	// kernel learns of the partition, and both precede the format.
	wantActions := []string{"write-mbr", "add-partition-2", "format-" + Label}
	if got := strings.Join(card.actions, ","); got != strings.Join(wantActions, ",") {
		t.Fatalf("actions = %v, want %v", card.actions, wantActions)
	}

	partType, start, size := readEntry(card.wroteMBR, dataPartitionNumber)
	wantSize := uint32(cardSize/sectorSize - dataPartitionStartLBA) // 8GiB is already 4MiB-aligned
	if partType != fatPartitionType || start != dataPartitionStartLBA || size != wantSize {
		t.Errorf("partition 2 entry = type %#02x start %d size %d, want type %#02x start %d size %d",
			partType, start, size, fatPartitionType, dataPartitionStartLBA, wantSize)
	}
	if bootType, bootStart, _ := readEntry(card.wroteMBR, bootPartitionNumber); bootType != fatPartitionType || bootStart != bootPartitionStartLBA {
		t.Error("partition 1's entry was disturbed")
	}
	if card.addedStart != dataPartitionStartLBA*sectorSize || card.addedSize != int64(wantSize)*sectorSize {
		t.Errorf("kernel partition registered as [%d, +%d), want [%d, +%d)",
			card.addedStart, card.addedSize, dataPartitionStartLBA*sectorSize, int64(wantSize)*sectorSize)
	}
}

func TestRunAlignsThePartitionDownTo4MiB(t *testing.T) {
	const cardSize = 8<<30 + 1000*sectorSize // an untidy tail past the last 4MiB boundary
	card := newFakeCard(gosdMBR(), cardSize)

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	_, _, size := readEntry(card.wroteMBR, dataPartitionNumber)
	if size%(alignBytes/sectorSize) != 0 {
		t.Errorf("partition size %d sectors is not 4MiB-aligned", size)
	}
	if want := uint32(8<<30/sectorSize - dataPartitionStartLBA); size != want {
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

func TestRunFormatsAfterAnInterruptedFirstBoot(t *testing.T) {
	// Power was lost between the MBR write and the format: the entry exists,
	// the partition holds nothing. Only the format is redone — the MBR and
	// the kernel's view (from its own boot-time scan) are already right.
	card := newFakeCard(withDataEntry(1<<21), 8<<30)
	card.nodeExists = true
	card.contents = diskfmt.Contents{Blank: true}

	if err := Run(card.deps(), testOptions()); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if got := strings.Join(card.actions, ","); got != "format-"+Label {
		t.Errorf("actions = %v, want only the format", card.actions)
	}
}

func TestRunSkipsACardWithNoRoom(t *testing.T) {
	// A card barely bigger than the 272MiB image: no partition is worth
	// creating, and the card must not be written to at all.
	card := newFakeCard(gosdMBR(), 300*1024*1024)

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
	card := newFakeCard(gosdMBR(), 1<<40) // 1TiB

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
	foreign := gosdMBR()
	foreign[signatureOffset] = 0x00 // no boot signature: not a GoSD card

	card := newFakeCard(foreign, 8<<30)
	err := Run(card.deps(), testOptions())
	if err == nil || !strings.Contains(err.Error(), "leaving it untouched") {
		t.Fatalf("Run() = %v, want a refusal naming the foreign table", err)
	}
	if len(card.actions) != 0 {
		t.Errorf("a foreign card saw %v, want nothing", card.actions)
	}
}

func TestRunReportsANodeThatNeverAppears(t *testing.T) {
	card := newFakeCard(gosdMBR(), 8<<30)
	card.nodeAppearsOnAdd = false

	err := Run(card.deps(), testOptions())
	if err == nil || !strings.Contains(err.Error(), "did not appear") {
		t.Fatalf("Run() = %v, want a node-timeout error", err)
	}
	for _, a := range card.actions {
		if strings.HasPrefix(a, "format") {
			t.Error("format ran though the partition node never appeared")
		}
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
