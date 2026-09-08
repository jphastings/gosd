package ready

import "testing"

// TestSignalIsANoOpOffDevice pins the off-device half of the package's
// contract: go test builds without the `gosd` tag, so runDir is empty and
// Signal must return nil without touching any filesystem.
func TestSignalIsANoOpOffDevice(t *testing.T) {
	if err := Signal(); err != nil {
		t.Errorf("Signal() = %v, want nil (a no-op off a GoSD device)", err)
	}
}
