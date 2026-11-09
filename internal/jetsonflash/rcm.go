package jetsonflash

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

const (
	// rcmVersion is T210's boot ROM answer to a QUERY_RCM_VERSION message.
	rcmVersion = 0x00210001
	// rcmChunk matches the boot ROM's 4 KiB receive buffers.
	rcmChunk   = 4096
	uidTimeout = time.Second
	rcmTimeout = 2 * time.Second
)

// bootROM reads the boot ROM's 16-byte UID, then sends two signed RCM
// messages verbatim: rcm0 queries the RCM version and rcm1 carries the
// recovery applet, which the ROM verifies and jumps to.
func bootROM(c conn, rcm0, rcm1 []byte) ([]byte, error) {
	buf := make([]byte, maxTransfer)
	n, err := c.Read(buf, uidTimeout)
	if errors.Is(err, errTimeout) {
		return nil, errors.New("the boot ROM sent no UID; it sends it once per power-on, so power-cycle the module into recovery mode and retry")
	}
	if err != nil {
		return nil, fmt.Errorf("reading the boot ROM's UID: %w", err)
	}
	if n != 16 {
		return nil, fmt.Errorf("the boot ROM's UID was %d bytes, want 16: % x", n, buf[:n])
	}
	uid := slices.Clone(buf[:16])

	for _, step := range []struct {
		what string
		msg  []byte
		want uint32
	}{
		{"RCM version query", rcm0, rcmVersion},
		{"recovery applet download", rcm1, 0},
	} {
		for chunk := range slices.Chunk(step.msg, rcmChunk) {
			if err := c.Write(chunk, rcmTimeout); err != nil {
				return nil, fmt.Errorf("sending the %s: %w", step.what, err)
			}
		}
		n, err := c.Read(buf, rcmTimeout)
		if err != nil {
			return nil, fmt.Errorf("reading the boot ROM's answer to the %s: %w", step.what, err)
		}
		if n != 4 {
			return nil, fmt.Errorf("the boot ROM answered the %s with %d bytes, want 4: % x", step.what, n, buf[:n])
		}
		if got := le32(buf); got != step.want {
			return nil, fmt.Errorf("the boot ROM answered the %s with %#08x, want %#08x", step.what, got, step.want)
		}
	}
	return uid, nil
}

// brCID renders a UID the way NVIDIA's tools print it, as one
// little-endian 128-bit number, so it can be compared with their logs.
func brCID(uid []byte) string {
	r := slices.Clone(uid)
	slices.Reverse(r)
	return fmt.Sprintf("0x%x", r)
}
