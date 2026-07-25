//go:build linux

package wifiup

import (
	"testing"

	"golang.org/x/sys/unix"
)

// The nl80211 values in disconnectreason.go are mirrored locally so event
// parsing stays pure and testable on macOS; this pins them to the real
// constants on the platform that actually speaks nl80211.
func TestNL80211ConstantsMatchUnix(t *testing.T) {
	pairs := map[string][2]int{
		"CmdDeauthenticate":  {nl80211CmdDeauthenticate, unix.NL80211_CMD_DEAUTHENTICATE},
		"CmdDisassociate":    {nl80211CmdDisassociate, unix.NL80211_CMD_DISASSOCIATE},
		"CmdDisconnect":      {nl80211CmdDisconnect, unix.NL80211_CMD_DISCONNECT},
		"AttrIfindex":        {nl80211AttrIfindex, unix.NL80211_ATTR_IFINDEX},
		"AttrFrame":          {nl80211AttrFrame, unix.NL80211_ATTR_FRAME},
		"AttrReasonCode":     {nl80211AttrReasonCode, unix.NL80211_ATTR_REASON_CODE},
		"AttrDisconnectedBy": {nl80211AttrDisconnectedByAP, unix.NL80211_ATTR_DISCONNECTED_BY_AP},
	}
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("%s = %d, want unix's %d", name, pair[0], pair[1])
		}
	}
}
