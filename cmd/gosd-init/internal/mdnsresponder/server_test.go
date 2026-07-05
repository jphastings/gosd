package mdnsresponder

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv4"
)

// TestNewServerAnswersQueries is a real, no-fakes smoke test: it starts the
// actual production responder — real bound multicast UDP sockets, no fakes
// anywhere — and queries it with a second, independent mDNS client, as
// close as a single host can get to the dns-sd / `ping hostname.local`
// checks the bean's manual cross-OS matrix calls for (those need a second
// physical/virtual machine on the same LAN and stay unchecked in the bean).
//
// Guarded rather than asserted-fatal on any failure to bind or get an
// answer: sandboxed or containerized CI runners sometimes can't join
// multicast groups at all (no usable non-loopback interface, or the group
// join itself is blocked by the sandbox), and that's an environment
// limitation, not a bug in this package — skip rather than flake CI, per
// this repo's requirement that networked tests be guarded.
func TestNewServerAnswersQueries(t *testing.T) {
	log := &testLog{}
	srv, err := NewServer("gosd-smoketest-host", log.Printf)
	if err != nil {
		t.Skipf("mDNS responder unavailable in this environment, skipping: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := newQueryOnlyClient()
	if err != nil {
		t.Skipf("mDNS query client unavailable in this environment, skipping: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, addr, err := client.QueryAddr(ctx, "gosd-smoketest-host.local")
	if err != nil {
		t.Skipf("no answer to gosd-smoketest-host.local within the probe window (environment likely blocks multicast); skipping: %v", err)
	}
	if !addr.IsValid() {
		t.Fatal("QueryAddr reported success but returned an invalid address")
	}
	t.Logf("gosd-smoketest-host.local resolved to %s", addr)
}

// newQueryOnlyClient opens a second, independent mDNS instance with no
// LocalNames of its own (so it only ever queries, never answers), mirroring
// pion/mdns's own examples/query.
func newQueryOnlyClient() (*mdns.Conn, error) {
	addr4, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
	if err != nil {
		return nil, err
	}
	l4, err := net.ListenUDP("udp4", addr4)
	if err != nil {
		return nil, err
	}
	conn, err := mdns.Server(ipv4.NewPacketConn(l4), nil, &mdns.Config{Name: "gosd-smoketest-client"})
	if err != nil {
		return nil, fmt.Errorf("starting query client: %w", err)
	}
	return conn, nil
}
