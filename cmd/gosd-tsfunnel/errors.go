package main

import (
	"fmt"
	"time"
)

// registerTimeoutError wraps a failure from tsnet.Server.Up: the node
// didn't reach the Running state within timeout. The two most common causes
// are both outside gosd-tsfunnel's control — an expired TS_AUTHKEY (auth
// keys expire within 90 days and are only needed for first registration;
// tsnet ignores the key once state already exists) and a device clock that's
// wrong enough to fail the tailnet's TLS handshake (GoSD boards have no
// battery-backed RTC and start every boot at the Unix epoch until SNTP
// syncs) — so the message names both rather than passing the bare tsnet
// error through.
func registerTimeoutError(timeout time.Duration, err error) error {
	return fmt.Errorf(
		"tsnet node did not finish registering within %s: %w — check that TS_AUTHKEY hasn't expired (auth keys expire within 90 days and are only needed for first registration; tsnet ignores the key once state already exists) and that the device clock is roughly correct (GoSD boards have no RTC and start every boot at the Unix epoch until SNTP syncs)",
		timeout, err,
	)
}

// funnelUnavailableError wraps a failure from tsnet.Server.ListenFunnel.
// Funnel needs three things set on the tailnet's own policy, none of which
// gosd-tsfunnel (or the device at all) can set for itself: the "funnel"
// nodeAttr granted to this node in the ACL policy file, HTTPS certificates
// enabled for the tailnet, and MagicDNS enabled (a Funnel hostname is a
// MagicDNS name). The message names all three, plus docs/ingress.md, rather
// than passing the bare tsnet error through, since none of them are
// diagnosable from the error text alone.
func funnelUnavailableError(err error) error {
	return fmt.Errorf(
		"tsnet could not start a Funnel listener: %w — check that this tailnet's ACL policy grants this device the \"funnel\" nodeAttr, that HTTPS certificates are enabled for the tailnet, and that MagicDNS is enabled; see docs/ingress.md",
		err,
	)
}
