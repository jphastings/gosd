//go:build linux

package wifiup

import (
	"testing"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// The kernel silently skips netlink messages that lack NLM_F_REQUEST —
// and still returns a success ack when NLM_F_ACK is set — so a CONNECT
// sent without Request is an invisible no-op (bean gosd-anyp: two bench
// days of "WiFi loop" were exactly this). mdlayher/netlink does not add
// the flag automatically.
func TestConnectRequestFlagsIncludeRequest(t *testing.T) {
	if connectRequestFlags&netlink.Request == 0 {
		t.Fatal("connectRequestFlags is missing netlink.Request: the kernel will silently discard (and ack!) the CONNECT")
	}
	if connectRequestFlags&netlink.Acknowledge == 0 {
		t.Fatal("connectRequestFlags is missing netlink.Acknowledge: CONNECT errors would go unreported")
	}
}

// The CONNECT attribute set must stay in lockstep with mdlayher/wifi's
// ConnectWPAPSK (v0.8.0 client_linux.go:146-163) — the only set
// bench-proven against real BCM43430/1 firmware (gosd-anyp, 2026-07-25).
func TestConnectPSKAttributesMirrorMdlayherWifi(t *testing.T) {
	b, err := connectPSKAttributes(Interface{Index: 7}, "ssid", [32]byte{1})
	if err != nil {
		t.Fatalf("connectPSKAttributes: %v", err)
	}
	attrs, err := netlink.UnmarshalAttributes(b)
	if err != nil {
		t.Fatalf("unmarshalling attributes: %v", err)
	}

	want := []uint16{
		unix.NL80211_ATTR_IFINDEX,
		unix.NL80211_ATTR_SSID,
		unix.NL80211_ATTR_WPA_VERSIONS,
		unix.NL80211_ATTR_CIPHER_SUITE_GROUP,
		unix.NL80211_ATTR_CIPHER_SUITES_PAIRWISE,
		unix.NL80211_ATTR_AKM_SUITES,
		unix.NL80211_ATTR_WANT_1X_4WAY_HS,
		unix.NL80211_ATTR_PMK,
		unix.NL80211_ATTR_AUTH_TYPE,
	}
	if len(attrs) != len(want) {
		t.Fatalf("got %d attributes, want %d", len(attrs), len(want))
	}
	for i, a := range attrs {
		if a.Type != want[i] {
			t.Errorf("attribute %d: got type %d, want %d", i, a.Type, want[i])
		}
	}
}
