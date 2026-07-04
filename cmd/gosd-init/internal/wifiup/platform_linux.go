//go:build linux

package wifiup

import (
	"errors"
	"fmt"
	"os"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	mlwifi "github.com/mdlayher/wifi"
	"golang.org/x/sys/unix"
)

// NewPlatform dials the nl80211 generic netlink family and returns the
// real, nl80211-backed WifiClient implementation. It can fail if nl80211
// isn't available at all (no WiFi hardware or driver) — a normal,
// expected outcome on an Ethernet-only board, so callers should treat a
// non-nil error here the same as "no WiFi hardware present", not as
// fatal to boot.
func NewPlatform() (WifiClient, error) {
	c, err := mlwifi.New()
	if err != nil {
		return nil, fmt.Errorf("opening nl80211: %w", err)
	}
	return nlClient{c: c}, nil
}

// nlClient implements WifiClient using github.com/mdlayher/wifi for
// everything except the WPA2-PSK connect itself (see connectPMK).
type nlClient struct {
	c *mlwifi.Client
}

func (n nlClient) Interfaces() ([]Interface, error) {
	ifis, err := n.c.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]Interface, 0, len(ifis))
	for _, ifi := range ifis {
		if ifi.Type != mlwifi.InterfaceTypeStation {
			continue
		}
		out = append(out, Interface{Name: ifi.Name, Index: ifi.Index})
	}
	return out, nil
}

func (n nlClient) Connect(ifi Interface, ssid string) error {
	return n.c.Connect(&mlwifi.Interface{Index: ifi.Index}, ssid)
}

// ConnectPSK associates with ssid using psk as the already-resolved PMK.
//
// github.com/mdlayher/wifi's own ConnectWPAPSK takes a passphrase string
// and always re-derives the PMK from it internally (PBKDF2-SHA1 over
// whatever string is passed) — there is no public API to hand it an
// already-derived key. That's incompatible with accepting a pre-hashed
// PSK directly (the whole point of which is that no plaintext passphrase
// need ever exist on the image), so ConnectPSK issues the same
// NL80211_CMD_CONNECT nl80211 request the library's ConnectWPAPSK does
// (mirroring it attribute-for-attribute), over its own short-lived
// generic netlink connection, with the resolved PMK bytes substituted
// directly for the library's internal derivation step.
func (n nlClient) ConnectPSK(ifi Interface, ssid string, psk [32]byte) error {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return fmt.Errorf("dialing generic netlink: %w", err)
	}
	defer func() { _ = conn.Close() }()

	family, err := conn.GetFamily(unix.NL80211_GENL_NAME)
	if err != nil {
		return fmt.Errorf("resolving nl80211 family: %w", err)
	}

	// Cipher/AKM suite OUI values (Wi-Fi Alliance 00-0F-AC-XX), matching
	// github.com/mdlayher/wifi's ConnectWPAPSK exactly: CCMP-128 (AES,
	// the only cipher WPA2 in gosd's scope uses) and PSK authentication.
	const (
		cipherSuiteCCMP128 = 0x000FAC04
		akmSuitePSK        = 0x000FAC02
	)

	ae := netlink.NewAttributeEncoder()
	ae.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(ifi.Index))
	ae.Bytes(unix.NL80211_ATTR_SSID, []byte(ssid))
	ae.Uint32(unix.NL80211_ATTR_WPA_VERSIONS, unix.NL80211_WPA_VERSION_2)
	ae.Uint32(unix.NL80211_ATTR_CIPHER_SUITE_GROUP, cipherSuiteCCMP128)
	ae.Uint32(unix.NL80211_ATTR_CIPHER_SUITES_PAIRWISE, cipherSuiteCCMP128)
	ae.Uint32(unix.NL80211_ATTR_AKM_SUITES, akmSuitePSK)
	ae.Flag(unix.NL80211_ATTR_WANT_1X_4WAY_HS, true)
	ae.Bytes(unix.NL80211_ATTR_PMK, psk[:])
	ae.Uint32(unix.NL80211_ATTR_AUTH_TYPE, unix.NL80211_AUTHTYPE_OPEN_SYSTEM)

	b, err := ae.Encode()
	if err != nil {
		return fmt.Errorf("encoding CONNECT attributes: %w", err)
	}

	_, err = conn.Execute(genetlink.Message{
		Header: genetlink.Header{
			Command: unix.NL80211_CMD_CONNECT,
			Version: family.Version,
		},
		Data: b,
	}, family.ID, netlink.Acknowledge)
	if err != nil {
		return fmt.Errorf("nl80211 CONNECT: %w", err)
	}
	return nil
}

func (n nlClient) Disconnect(ifi Interface) error {
	return n.c.Disconnect(&mlwifi.Interface{Index: ifi.Index})
}

// Associated reports whether ifi has a live association by requesting its
// current BSS: mdlayher/wifi reports os.ErrNotExist when there's no BSS
// with a status attribute, which is the normal "not associated" outcome,
// not an error worth surfacing to the caller.
func (n nlClient) Associated(ifi Interface) (bool, error) {
	bss, err := n.c.BSS(&mlwifi.Interface{Index: ifi.Index})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return bss.Status == mlwifi.BSSStatusAssociated, nil
}
