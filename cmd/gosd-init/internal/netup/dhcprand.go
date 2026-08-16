package netup

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
)

// dhcpXIDSource supplies the randomness DHCPv4 transaction IDs are drawn
// from. It exists to work around gosd-yx94: on a board with no hardware RNG
// (proven on the Radxa Cubie A5E), the kernel CRNG can take far longer than
// the DHCP library's own 2-minute give-up timeout to seed itself from
// interrupt timing alone, and the library's default random source
// (github.com/u-root/uio/rand, wired in on platform_linux.go) blocks on
// exactly that — so gosd-init could spend up to two minutes silently
// spinning before it could even send its first DHCP packet (the transaction
// ID has to exist before the packet that carries it does).
//
// A DHCP transaction ID only needs to be probably-unique among concurrent
// exchanges on the same link (RFC 2131 §4.1) so a client can match replies
// to its own request; it is not cryptographic material; DHCP has no
// authentication for anything else in the exchange either, so guessing one
// gains an attacker nothing they didn't already have. This deliberately
// never touches the kernel CRNG (crypto/rand, /dev/urandom, getrandom(2)):
// math/rand/v2's top-level generator is seeded once, non-blockingly, by the
// Go runtime at process start (from ELF auxv AT_RANDOM, not a CRNG read),
// so it's available immediately and never depends on interrupt-timing
// entropy having accumulated.
type dhcpXIDSource struct{}

// Read implements the plain (non-context) half of
// github.com/u-root/uio/rand.ContextReader.
func (s dhcpXIDSource) Read(b []byte) (int, error) {
	return s.ReadContext(context.Background(), b)
}

// ReadContext implements github.com/u-root/uio/rand.ContextReader. ctx is
// accepted only to satisfy that interface: generation never blocks, so
// there is nothing for ctx to cancel.
func (dhcpXIDSource) ReadContext(_ context.Context, b []byte) (int, error) {
	n := len(b)
	for len(b) >= 8 {
		binary.LittleEndian.PutUint64(b, rand.Uint64())
		b = b[8:]
	}
	if len(b) > 0 {
		var tail [8]byte
		binary.LittleEndian.PutUint64(tail[:], rand.Uint64())
		copy(b, tail[:])
	}
	return n, nil
}
