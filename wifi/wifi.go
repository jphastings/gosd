// Package wifi lets a GoSD app join a WiFi network at runtime, using
// credentials the app obtained by its own means — an NFC tag, a
// provisioning screen, anything other than the config tree gosd-init
// already reads at boot.
//
// [Join] hands the request to gosd-init over the same file-drop idiom
// fault uses (see internal/wifictl): there is no socket and gosd-init adds
// no listener, matching the fleet-wide rule that mDNS is the only network
// listener it runs. gosd-init reconciles the request against the WiFi
// interface, tears down any current association, attempts the new network,
// and — on success — restarts the ingress tunnel (cloudflared or
// tailscale-funnel, whichever the image was built with) so it comes back on
// the new network without a reboot. Join itself does not touch ingress; the
// reconnect is automatic and has no app-facing API.
//
// # Off a device
//
// Like fault, this package behaves differently depending on whether the
// binary was produced by `gosd build` — the `gosd` build tag, not
// `GOOS=linux` — rather than probing for /run at runtime, so a `go test` on
// a Linux CI runner never mistakes itself for a device. Off a device, Join
// returns an immediate, actionable error and touches no filesystem: there
// is no gosd-init to hand a request to.
//
// # Scope
//
// WPA2-PSK and open networks only, matching the fleet's WiFi scope. A board
// with no WiFi interface fails the join honestly rather than refusing to
// build. WPA3, EAP, and orchestrating against WiFi AP mode are out of scope
// for v1.
package wifi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jphastings/gosd/fault"
	"github.com/jphastings/gosd/internal/wifictl"
)

// Credentials identifies the network to join.
type Credentials struct {
	// SSID is the network name. Required.
	SSID string
	// Passphrase authenticates to SSID under WPA2-PSK. Leave it empty to
	// join an open network — the fleet's WiFi scope allows both, nothing
	// else (no WPA3, no EAP).
	Passphrase string
}

// Options controls how a runtime join is applied.
type Options struct {
	// Persist asks gosd-init to write these credentials into the card's
	// config tree once the join succeeds, so the device rejoins this
	// network on its next boot too. Left false, a successful join is
	// in-memory only: the next boot uses whatever the config tree
	// already had. A failed join is never persisted either way.
	Persist bool
}

// wifiPassphraseLabel is what a redacted crash report shows in place of a
// passphrase Join registered — see [fault.RegisterSecretString].
const wifiPassphraseLabel = "wifi-passphrase"

// defaultPollInterval is how often Join checks status.json for this
// request's outcome while it waits.
const defaultPollInterval = 500 * time.Millisecond

// Join asks gosd-init to join creds now, and blocks until gosd-init reports
// the attempt joined or failed.
//
// ctx cancels the WAIT, not the join attempt: gosd-init keeps trying (or
// keeps the connection it already made) regardless of ctx, exactly as if
// nothing had called Join to watch it. Cancelling ctx makes Join return
// ctx.Err() without changing what gosd-init is doing.
//
// A returned error names the failure as precisely as nl80211 reported it —
// a wrong WPA2 passphrase usually surfaces as a handshake timeout, and the
// error says so rather than claiming to know the passphrase was wrong. A
// board with no WiFi interface fails with an honest "no WiFi interface"
// error rather than hanging.
//
// creds.Passphrase, if any, is registered with [fault.RegisterSecretString]
// before anything is written, so a crash between here and a terminal status
// still redacts it from any report the device writes about itself.
//
// Off a device — anything not built by `gosd build` — Join returns an
// immediate, actionable error and does nothing else: see the package doc.
func Join(ctx context.Context, creds Credentials, opts Options) error {
	return std.join(ctx, creds, opts)
}

// std is the joiner behind [Join]. Tests build their own against a temp
// directory, the same pattern fault uses for its reporter, which is what
// lets the polling/matching logic be tested as behaviour on any OS.
var std = &joiner{dir: runDir}

// joiner is this package's state: where a request is dropped and a status
// is read back from (empty off a device) and how often to poll for it.
type joiner struct {
	dir          string
	pollInterval time.Duration
}

func (j *joiner) join(ctx context.Context, creds Credentials, opts Options) error {
	if creds.SSID == "" {
		return errors.New("wifi.Join: SSID is required")
	}
	if j.dir == "" {
		return fmt.Errorf("wifi.Join only works in an app built by `gosd build`; this binary wasn't, so it would have joined %q now rather than doing anything", creds.SSID)
	}

	if creds.Passphrase != "" {
		fault.RegisterSecretString(creds.Passphrase, wifiPassphraseLabel)
	}

	id, err := newRequestID()
	if err != nil {
		return fmt.Errorf("wifi.Join: generating a request id: %w", err)
	}

	req := wifictl.Request{
		ID:         id,
		SSID:       creds.SSID,
		Passphrase: creds.Passphrase,
		Persist:    opts.Persist,
	}
	if err := wifictl.WriteRequest(j.dir, req); err != nil {
		return fmt.Errorf("wifi.Join: asking gosd-init to join %q: %w", creds.SSID, err)
	}

	return j.awaitOutcome(ctx, id, creds.SSID)
}

// awaitOutcome polls status.json until it carries id's terminal outcome, or
// ctx is done. A status for a different id — this request's predecessor,
// still being written out — or one that doesn't parse is treated as "not
// yet" rather than an error: the next poll is expected to see something
// trustworthy, and the file drop protocol gives no other way to tell "not
// yet" from "briefly unreadable".
func (j *joiner) awaitOutcome(ctx context.Context, id, ssid string) error {
	interval := j.pollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if status, ok, err := wifictl.ReadStatus(j.dir); err == nil && ok && status.ID == id {
			switch status.State {
			case wifictl.Joined:
				return nil
			case wifictl.Failed:
				return fmt.Errorf("joining %q failed: %s", ssid, status.Error)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// newRequestID names one Join call's request, so its eventual status can be
// matched back to it even after a later call has replaced request.json.
func newRequestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
