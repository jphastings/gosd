package netup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	uiorand "github.com/u-root/uio/rand"
)

// TestDHCPXIDSourceIgnoresContextDeadline proves dhcpXIDSource never depends
// on a signal that would track kernel CRNG readiness (gosd-yx94: the real
// bug's error was literally "context deadline exceeded" from a context
// whose only job was bounding a wait for the CRNG to seed). An
// already-expired context is the sharpest way to show that: any source that
// blocked on something ctx-cancellable would fail immediately here, exactly
// as the real, unseeded-CRNG-backed source did on hardware.
func TestDHCPXIDSourceIgnoresContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done() // guarantee the deadline has already passed

	var s dhcpXIDSource
	b := make([]byte, 4) // dhcpv4.TransactionID is 4 bytes
	n, err := s.ReadContext(ctx, b)
	if err != nil {
		t.Fatalf("ReadContext() with an expired context = %v, want nil error", err)
	}
	if n != len(b) {
		t.Fatalf("ReadContext() returned n=%d, want %d", n, len(b))
	}
}

// TestDHCPXIDSourceCompletesImmediately guards against a regression that
// reintroduces any blocking path: the whole point of dhcpXIDSource is that
// it can never be the reason gosd-init waits on randomness.
func TestDHCPXIDSourceCompletesImmediately(t *testing.T) {
	var s dhcpXIDSource
	b := make([]byte, 4)

	start := time.Now()
	if _, err := s.Read(b); err != nil {
		t.Fatalf("Read() = %v, want nil error", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Read() took %s, want effectively instant", elapsed)
	}
}

func TestDHCPXIDSourceFillsRequestedLength(t *testing.T) {
	var s dhcpXIDSource
	for _, size := range []int{0, 1, 4, 7, 8, 9, 16, 33} {
		b := make([]byte, size)
		n, err := s.ReadContext(context.Background(), b)
		if err != nil {
			t.Errorf("ReadContext(%d bytes) = %v, want nil error", size, err)
		}
		if n != size {
			t.Errorf("ReadContext(%d bytes) returned n=%d, want %d", size, n, size)
		}
	}
}

// TestDHCPXIDSourceVariesAcrossCalls is a light sanity check that this
// isn't accidentally always-zero or always-the-same value — it doesn't need
// cryptographic quality (see dhcpXIDSource's doc), only enough variation
// that concurrent DHCP transactions on the same link don't collide.
func TestDHCPXIDSourceVariesAcrossCalls(t *testing.T) {
	var s dhcpXIDSource
	seen := map[[4]byte]bool{}
	for i := 0; i < 100; i++ {
		var b [4]byte
		if _, err := s.Read(b[:]); err != nil {
			t.Fatalf("Read() = %v, want nil error", err)
		}
		seen[b] = true
	}
	if len(seen) < 90 {
		t.Fatalf("got only %d distinct values out of 100 reads, want mostly-unique output", len(seen))
	}
}

// blockingRandSource models an unseeded kernel CRNG's exact observed
// behavior (gosd-yx94): every read blocks until the caller's context is
// cancelled and then fails — it never produces a byte no matter how long
// it's given, exactly like a board whose interrupt-timing entropy never
// accumulates enough within the DHCP library's own give-up window.
type blockingRandSource struct{}

func (blockingRandSource) Read(b []byte) (int, error) {
	return blockingRandSource{}.ReadContext(context.Background(), b)
}

func (blockingRandSource) ReadContext(ctx context.Context, _ []byte) (int, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

// withRandReader installs r as u-root/uio/rand's process-wide Reader for
// the duration of the calling test and restores the previous value
// afterwards.
func withRandReader(t *testing.T, r uiorand.ContextReader) {
	t.Helper()
	prev := uiorand.Reader
	uiorand.Reader = r
	t.Cleanup(func() { uiorand.Reader = prev })
}

// TestUnseededCRNGReproducesTheObservedDHCPFailure reproduces gosd-yx94's
// exact real-world symptom deterministically, against the real
// github.com/insomniacslk/dhcp code path (dhcpv4.GenerateTransactionIDWithContext
// is what dhcpv4.New — and so nclient4's Discover/Request — calls to build
// every DHCP packet's transaction ID): installing a random source that
// behaves exactly like the unseeded getrandom(2) path observed on the Cubie
// A5E reproduces the bean's precise error, "could not get random number:
// context deadline exceeded".
func TestUnseededCRNGReproducesTheObservedDHCPFailure(t *testing.T) {
	withRandReader(t, blockingRandSource{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := dhcpv4.GenerateTransactionIDWithContext(ctx)
	if err == nil {
		t.Fatal("GenerateTransactionIDWithContext() succeeded against a source that never produces randomness, want an error")
	}
	// dhcpv4 formats (rather than wraps) the underlying error, so this
	// checks the message text — the same text the bean's console log
	// showed — not errors.Is.
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf(`GenerateTransactionIDWithContext() error = %q, want it to contain "context deadline exceeded" (matching the bean's observed failure)`, err)
	}
}

// TestDHCPXIDSourceFixesTheUnseededCRNGRace proves the fix against that same
// real dhcpv4 call path: once dhcpXIDSource is installed (as
// platform_linux.go's init does in production), the identical scenario — a
// context with no time left, which the reproduction above shows the old
// source fails against — succeeds immediately, because generation no longer
// touches anything ctx-cancellable at all.
func TestDHCPXIDSourceFixesTheUnseededCRNGRace(t *testing.T) {
	withRandReader(t, dhcpXIDSource{})

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	if _, err := dhcpv4.GenerateTransactionIDWithContext(ctx); err != nil {
		t.Fatalf("GenerateTransactionIDWithContext() = %v, want nil error even with no time left on ctx", err)
	}
}
