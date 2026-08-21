package diskfmt

import "testing"

// reformatMatrixDeviceBytes must hold the data golden's fixed 512MiB image
// with headroom left over for a FAT32 or exFAT layout written on top of it
// afterwards — the same size the bean's own reproduction used.
const reformatMatrixDeviceBytes = 768 << 20

// reformatFormatters lets TestReformatOverwritesEveryPriorSignature drive
// every ordered pair of {ext4, fat32, exfat} without hand-listing them.
var reformatFormatters = map[FS]func(devicePath, label string) error{
	EXT4:  func(devicePath, label string) error { return FormatEXT4(EXT4GoldenData, devicePath, label) },
	FAT32: FormatFAT32,
	ExFAT: FormatExFAT,
}

// TestReformatOverwritesEveryPriorSignature is the regression test for
// gosd-o34r: formatting a device as A and then as B must leave Inspect
// reporting exactly B and B's label, whatever A was and whatever label A
// carried — never a stale signature A left behind.
//
// Before the fix, ext4 -> fat32 failed this: go-diskfs's FAT32 writer never
// touches offset 1024, where ext4 puts its superblock, so a fully
// successful FAT32 format left the old ext4 superblock intact and Inspect
// (which probes ext4 before FAT) reported the dead ext4 volume instead. The
// other five ordered pairs already passed — exFAT is probed before ext4,
// and both FAT32 and ext4's writers happen to overwrite whatever a prior
// exFAT boot sector left at their own signature offsets — and this test
// proves the fix does not disturb any of them.
func TestReformatOverwritesEveryPriorSignature(t *testing.T) {
	for from := range reformatFormatters {
		for to := range reformatFormatters {
			if from == to {
				continue
			}
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				path := backingFile(t, reformatMatrixDeviceBytes)

				if err := reformatFormatters[from](path, "OLD-DATA"); err != nil {
					t.Fatalf("formatting as %s: %v", from, err)
				}
				if err := reformatFormatters[to](path, "NEW-DATA"); err != nil {
					t.Fatalf("reformatting as %s: %v", to, err)
				}

				got, err := Inspect(path)
				if err != nil {
					t.Fatalf("Inspect: %v", err)
				}
				if got.FS != to || got.Label != "NEW-DATA" {
					t.Fatalf("Inspect after %s -> %s = %+v, want {FS:%s Label:NEW-DATA}", from, to, got, to)
				}
			})
		}
	}
}
