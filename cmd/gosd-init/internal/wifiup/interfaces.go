package wifiup

// Interface is the subset of a WiFi station interface's identity wifiup
// needs, deliberately decoupled from github.com/mdlayher/wifi's own type
// (mirroring netup.Lease's rationale) so the state machine in wifiup.go
// can be constructed and asserted on by tests without ever importing
// github.com/mdlayher/wifi.
type Interface struct {
	Name  string
	Index int
}

// WifiClient performs the nl80211 operations wifiup needs: discovering
// wireless interfaces, associating with an access point (open or
// WPA2-PSK), tearing a connection down, and checking whether an
// association is still live. Implementations must be safe to use from a
// single goroutine at a time per interface (wifiup never calls into the
// same interface concurrently).
type WifiClient interface {
	// Interfaces lists the system's WiFi station interfaces. Called
	// repeatedly (with backoff) by wifiup.Run's wait loop, since the
	// wlan interface may not exist yet at boot — firmware for the WiFi
	// chipset loads asynchronously at driver probe.
	Interfaces() ([]Interface, error)

	// Connect associates ifi with an open (no security) network named
	// ssid.
	Connect(ifi Interface, ssid string) error

	// ConnectPSK associates ifi with a WPA2-PSK network named ssid,
	// using psk as the already-resolved 256-bit pairwise master key
	// (see DerivePSK and ParsePSKHex) — never a plaintext passphrase.
	// gosd-init's own PBKDF2 derivation is the single source of truth
	// for the key regardless of whether config.json supplied a
	// plaintext passphrase or a pre-hashed PSK, so this call site never
	// needs to know or care which form the credential started as.
	ConnectPSK(ifi Interface, ssid string, psk [32]byte) error

	// Disconnect tears down any existing association on ifi.
	Disconnect(ifi Interface) error

	// Associated reports whether ifi currently has a live association.
	// wifiup polls this (there is no nl80211 deauth/disconnect event
	// stream exposed by mdlayher/wifi) to detect a lost connection and
	// trigger a reconnect.
	Associated(ifi Interface) (bool, error)

	// WatchDisconnects subscribes to ifi's nl80211 MLME events
	// (disconnect / deauthenticate / disassociate) so the reconnect loop
	// can report WHY an association dropped — Associated's polling only
	// reveals THAT it dropped. The returned watcher must be Closed when
	// the loop ends. Failing to subscribe loses diagnostics, nothing
	// more: callers degrade to logging association losses without a
	// reason code.
	WatchDisconnects(ifi Interface) (DisconnectWatcher, error)
}

// DisconnectWatcher accumulates nl80211 disconnect reason codes for one
// interface in the background, never blocking its consumer: TakeReason
// is an instant read of the most recently observed event, not a wait for
// one, so a slow or silent event socket can never stall the reconnect
// loop.
type DisconnectWatcher interface {
	// TakeReason returns the most recent disconnect reason observed
	// since the previous call, clearing it so one event can never be
	// attributed to two separate association losses. ok=false means no
	// disconnect / deauthenticate / disassociate event has been seen
	// since the last call.
	TakeReason() (reason DisconnectReason, ok bool)

	// Close ends the subscription and its background receiver. Safe to
	// call while events are still arriving.
	Close() error
}

// CredentialSource supplies the network wifiup should join. v0.1 wires
// this to config.json (see ConfigCredentials); v0.2's Imager provisioning
// is expected to implement this interface against its own source (e.g. a
// provisioning file written at flash time) without any change to this
// package.
type CredentialSource interface {
	// Credentials returns the network to join and whether one is
	// configured at all. ok=false means "no WiFi credentials
	// configured" (e.g. an Ethernet-only board) — wifiup.Run must not
	// spin a retry loop in that case.
	Credentials() (creds Credentials, ok bool, err error)
}
