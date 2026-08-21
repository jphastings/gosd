package boot

import (
	"testing"

	"github.com/jphastings/gosd/internal/devreserve"
)

// TestReservedBootPartitionRefusesTheWholeCard checks the two halves of
// bean gosd-ix0r agree, since nothing but a shared file couples them: what
// gosd-init publishes here has to be the thing that makes an app-side
// refusal come out right. Publishing the mountpoint, or the whole disk
// instead of the partition, would each pass every other test in this
// package and quietly get the app-side answer wrong.
func TestReservedBootPartitionRefusesTheWholeCard(t *testing.T) {
	var published []devreserve.Entry
	deps := Deps{ReserveDevices: func(devices []devreserve.Entry) error {
		published = devices
		return nil
	}}

	reserveDevices(deps, func(string, ...any) {}, "/dev/mmcblk0p1")

	data, err := devreserve.Encode(published)
	if err != nil {
		t.Fatalf("Encode(%+v) = %v; gosd-init must publish a set a reader will accept", published, err)
	}
	reserved, err := devreserve.Parse(data)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	for _, refused := range []string{"/dev/mmcblk0p1", "/dev/mmcblk0"} {
		entry, blocked := reserved.Exposes(refused)
		if !blocked {
			t.Errorf("%s is not refused; the card's kernel and config tree would reach a USB host", refused)
			continue
		}
		if entry.Role != bootPartitionRole {
			t.Errorf("refusing %s quotes role %q, want %q", refused, entry.Role, bootPartitionRole)
		}
	}
	if _, blocked := reserved.Exposes("/dev/mmcblk0p2"); blocked {
		t.Error("the data partition is refused; it is the app's own storage to share or not")
	}
}
