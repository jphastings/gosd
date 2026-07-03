package netup

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

func testDeps(clock *fakeClock, dhcp *fakeDHCP, log *testLog) Deps {
	return Deps{
		DHCP:       dhcp,
		Clock:      clock,
		NewBackoff: func() *Backoff { return noJitterBackoff(time.Second, 10*time.Second) },
		Log:        log.Printf,
	}
}

func leaseAt(clock *fakeClock, renewAfter, rebindAfter, expireAfter time.Duration) *Lease {
	return &Lease{
		ObtainedAt:  clock.Now(),
		RenewAfter:  renewAfter,
		RebindAfter: rebindAfter,
		ExpireAfter: expireAfter,
	}
}

func TestRunDHCPRetriesDiscoveryWithBackoffUntilSuccess(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	success := leaseAt(clock, time.Hour, 2*time.Hour, 3*time.Hour)
	dhcp := &fakeDHCP{requestResults: []requestResult{
		{err: errBoom},
		{err: errBoom},
		{lease: success},
	}}
	deps := testDeps(clock, dhcp, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := &leaseLog{}
	done := make(chan struct{})
	go func() {
		RunDHCP(ctx, deps, "eth0", got.add)
		close(done)
	}()

	// Two failed attempts, each followed by a 1s/2s backoff wait.
	waitForPending(t, clock, 1)
	clock.Advance(time.Second)
	waitForPending(t, clock, 1)
	clock.Advance(2 * time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for got.len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if dhcp.requestCallCount() != 3 {
		t.Fatalf("Request called %d times, want 3", dhcp.requestCallCount())
	}
	if leases := got.snapshot(); len(leases) != 1 || leases[0] != success {
		t.Fatalf("onLease calls = %v, want exactly the successful lease once", leases)
	}
	if !log.contains("retrying in 1s") {
		t.Errorf("log missing first retry delay: %v", log.snapshot())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunDHCP did not return after ctx was cancelled")
	}
}

func TestRunDHCPRenewsAtT1(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	first := leaseAt(clock, 10*time.Second, 20*time.Second, 30*time.Second)
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: first}}}
	renewed := &leaseBox{}
	dhcp.renewFn = func(iface string, lease *Lease, call int) (*Lease, error) {
		l := leaseAt(clock, time.Hour, 2*time.Hour, 3*time.Hour)
		renewed.set(l)
		return l, nil
	}
	deps := testDeps(clock, dhcp, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := &leaseLog{}
	done := make(chan struct{})
	go func() {
		RunDHCP(ctx, deps, "eth0", got.add)
		close(done)
	}()

	waitForPending(t, clock, 1)
	clock.Advance(10 * time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for dhcp.renewCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if dhcp.renewCallCount() != 1 {
		t.Fatalf("Renew called %d times, want 1", dhcp.renewCallCount())
	}
	if leases := got.snapshot(); len(leases) != 2 || leases[1] != renewed.get() {
		t.Fatalf("onLease calls = %v, want [initial, renewed]", leases)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunDHCP did not return after ctx was cancelled")
	}
}

func TestRunDHCPFallsBackToRebindWhenRenewFails(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	first := leaseAt(clock, 10*time.Second, 20*time.Second, 30*time.Second)
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: first}}}
	rebound := &leaseBox{}
	dhcp.renewFn = func(iface string, lease *Lease, call int) (*Lease, error) {
		if call == 0 {
			return nil, errBoom
		}
		l := leaseAt(clock, time.Hour, 2*time.Hour, 3*time.Hour)
		rebound.set(l)
		return l, nil
	}
	deps := testDeps(clock, dhcp, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := &leaseLog{}
	done := make(chan struct{})
	go func() {
		RunDHCP(ctx, deps, "eth0", got.add)
		close(done)
	}()

	waitForPending(t, clock, 1) // wait until T1
	clock.Advance(10 * time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for dhcp.renewCallCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	waitForPending(t, clock, 1) // wait until T2 after the failed renew
	clock.Advance(10 * time.Second)

	deadline = time.Now().Add(2 * time.Second)
	for dhcp.renewCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if dhcp.renewCallCount() != 2 {
		t.Fatalf("Renew called %d times, want 2 (renew at T1, rebind at T2)", dhcp.renewCallCount())
	}
	if leases := got.snapshot(); len(leases) != 2 || leases[1] != rebound.get() {
		t.Fatalf("onLease calls = %v, want [initial, rebound]", leases)
	}
	if !log.contains("will retry at rebind") {
		t.Errorf("log missing rebind-fallback message: %v", log.snapshot())
	}

	cancel()
	<-done
}

func TestRunDHCPRestartsDiscoveryWhenRebindFails(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	first := leaseAt(clock, 10*time.Second, 20*time.Second, 30*time.Second)
	second := leaseAt(clock, time.Hour, 2*time.Hour, 3*time.Hour)
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: first}, {lease: second}}}
	dhcp.renewFn = func(iface string, lease *Lease, call int) (*Lease, error) {
		return nil, errBoom // both renew and rebind fail
	}
	deps := testDeps(clock, dhcp, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := &leaseLog{}
	done := make(chan struct{})
	go func() {
		RunDHCP(ctx, deps, "eth0", got.add)
		close(done)
	}()

	waitForPending(t, clock, 1) // T1
	clock.Advance(10 * time.Second)
	deadline := time.Now().Add(2 * time.Second)
	for dhcp.renewCallCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	waitForPending(t, clock, 1) // T2
	clock.Advance(10 * time.Second)
	deadline = time.Now().Add(2 * time.Second)
	for dhcp.requestCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if dhcp.requestCallCount() != 2 {
		t.Fatalf("Request called %d times, want 2 (initial + restart after rebind failure)", dhcp.requestCallCount())
	}
	if leases := got.snapshot(); len(leases) != 2 || leases[1] != second {
		t.Fatalf("onLease calls = %v, want [first, second]", leases)
	}
	if !log.contains("restarting discovery") {
		t.Errorf("log missing discovery-restart message: %v", log.snapshot())
	}

	cancel()
	<-done
}

func TestRunDHCPStopsWhenContextCancelledDuringDiscovery(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}}
	deps := testDeps(clock, dhcp, log)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunDHCP(ctx, deps, "eth0", func(*Lease) {})
	}()

	waitForPending(t, clock, 1)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunDHCP() = %v, want nil on graceful ctx cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunDHCP did not return promptly after ctx cancellation")
	}
}
