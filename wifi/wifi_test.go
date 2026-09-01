package wifi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/wifictl"
)

func newTestJoiner(t *testing.T) (*joiner, string) {
	t.Helper()
	dir := t.TempDir()
	return &joiner{dir: dir, pollInterval: time.Millisecond}, dir
}

// startFakeReconciler stands in for gosd-init's wifiup reconciler: it
// watches dir for a request it hasn't answered yet and writes back
// whatever respond decides, so Join's polling/matching logic is exercised
// as behaviour without any real gosd-init involved.
func startFakeReconciler(t *testing.T, dir string, respond func(wifictl.Request) wifictl.Status) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })

	go func() {
		answered := ""
		for {
			select {
			case <-stop:
				return
			default:
			}
			if req, ok, err := wifictl.ReadRequest(dir); ok && err == nil && req.ID != answered {
				answered = req.ID
				_ = wifictl.WriteStatus(dir, respond(req))
			}
			time.Sleep(time.Millisecond)
		}
	}()
}

func TestJoinSucceedsWhenGosdInitReportsJoined(t *testing.T) {
	j, dir := newTestJoiner(t)
	startFakeReconciler(t, dir, func(req wifictl.Request) wifictl.Status {
		return wifictl.Status{ID: req.ID, State: wifictl.Joined}
	})

	if err := j.join(context.Background(), Credentials{SSID: "home-network", Passphrase: "correct-horse-battery"}, Options{}); err != nil {
		t.Fatalf("join() = %v, want nil", err)
	}
}

func TestJoinSurfacesTheFailureReasonVerbatim(t *testing.T) {
	j, dir := newTestJoiner(t)
	startFakeReconciler(t, dir, func(req wifictl.Request) wifictl.Status {
		return wifictl.Status{ID: req.ID, State: wifictl.Failed, Error: "4-way handshake timed out"}
	})

	err := j.join(context.Background(), Credentials{SSID: "home-network", Passphrase: "wrong-passphrase"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "4-way handshake timed out") {
		t.Fatalf("join() = %v, want it to include the reconciler's failure reason verbatim", err)
	}
}

func TestJoinIgnoresAStaleStatusFromAnEarlierRequest(t *testing.T) {
	j, dir := newTestJoiner(t)
	// A status left behind by a previous call, naming an id this call
	// never made.
	if err := wifictl.WriteStatus(dir, wifictl.Status{ID: "stale-id", State: wifictl.Joined}); err != nil {
		t.Fatal(err)
	}
	startFakeReconciler(t, dir, func(req wifictl.Request) wifictl.Status {
		return wifictl.Status{ID: req.ID, State: wifictl.Failed, Error: "network not found"}
	})

	err := j.join(context.Background(), Credentials{SSID: "home-network"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "network not found") {
		t.Fatalf("join() = %v, want it to wait for its own request's outcome rather than trust the stale status", err)
	}
}

func TestJoinReturnsWhenCtxIsCancelled(t *testing.T) {
	j, _ := newTestJoiner(t)
	// No reconciler runs: nothing will ever answer this request.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := j.join(ctx, Credentials{SSID: "home-network"}, Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("join() = %v, want context.DeadlineExceeded", err)
	}
}

func TestJoinRequiresAnSSID(t *testing.T) {
	j, _ := newTestJoiner(t)
	if err := j.join(context.Background(), Credentials{}, Options{}); err == nil {
		t.Fatal("join() with no SSID succeeded, want an error")
	}
}

func TestJoinOffADeviceFailsImmediately(t *testing.T) {
	j := &joiner{dir: ""}

	err := j.join(context.Background(), Credentials{SSID: "home-network"}, Options{})
	if err == nil {
		t.Fatal("join() off a device succeeded, want an actionable error")
	}
	if !strings.Contains(err.Error(), "gosd build") {
		t.Errorf("join() = %v, want it to say this binary wasn't built by gosd build", err)
	}
}

// TestJoinOffADeviceViaThePublicAPI exercises the exported Join, whose std
// joiner picks up runDir from the build tag this test binary was actually
// compiled with — the `gosd` tag is never set for `go test`, so this pins
// the same off-device behaviour production code gets when it isn't built
// by `gosd build`.
func TestJoinOffADeviceViaThePublicAPI(t *testing.T) {
	if err := Join(context.Background(), Credentials{SSID: "home-network"}, Options{}); err == nil {
		t.Fatal("Join() succeeded off a device, want an actionable error")
	}
}
