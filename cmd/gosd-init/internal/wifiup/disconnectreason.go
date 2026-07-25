package wifiup

import (
	"encoding/binary"
	"fmt"

	"github.com/mdlayher/netlink"
)

// nl80211 command and attribute values needed to interpret MLME multicast
// events, mirrored from golang.org/x/sys/unix's NL80211_* constants
// (asserted equal in platform_linux_test.go). Kept local so the event
// parsing — pure byte-level logic — lives in this OS-independent file and
// stays testable on any development host.
const (
	nl80211CmdDeauthenticate = 39
	nl80211CmdDisassociate   = 40
	nl80211CmdDisconnect     = 48

	nl80211AttrIfindex          = 3
	nl80211AttrFrame            = 51
	nl80211AttrReasonCode       = 54
	nl80211AttrDisconnectedByAP = 71
)

// DisconnectReason is why an association was lost, as carried by an
// nl80211 DISCONNECT / DEAUTHENTICATE / DISASSOCIATE event: the IEEE
// 802.11 reason code, plus whether the kernel flagged the disconnect as
// AP-initiated.
type DisconnectReason struct {
	Code   uint16
	FromAP bool
}

// String renders e.g. "reason 15 (4-way handshake timeout), reported by
// AP". Codes outside the small bench-diagnosis table render numerically.
func (r DisconnectReason) String() string {
	s := fmt.Sprintf("reason %d", r.Code)
	if text, ok := reasonText[r.Code]; ok {
		s += " (" + text + ")"
	}
	if r.FromAP {
		s += ", reported by AP"
	}
	return s
}

// reasonText names the IEEE 802.11 reason codes worth recognising on a
// serial console: the handful that discriminate between key-exchange,
// AP-policy and inactivity failures. Deliberately not a full spec
// transcription — unknown codes are still logged numerically.
var reasonText = map[uint16]string{
	1:  "unspecified",
	2:  "previous authentication no longer valid",
	3:  "deauthenticated because station is leaving",
	4:  "inactivity",
	6:  "class 2 frame from nonauthenticated station",
	7:  "class 3 frame from nonassociated station",
	15: "4-way handshake timeout",
	16: "group key handshake timeout",
	23: "802.1X authentication failed",
}

// parseDisconnectEvent extracts a DisconnectReason from one nl80211
// multicast event. ok=false means the event carries nothing to report:
// an unrelated command, an event for another interface, or a
// disconnect-ish event with no usable reason code (the kernel omits
// NL80211_ATTR_REASON_CODE for reasonless local disconnects).
func parseDisconnectEvent(cmd uint8, data []byte, ifindex uint32) (DisconnectReason, bool) {
	switch cmd {
	case nl80211CmdDisconnect, nl80211CmdDeauthenticate, nl80211CmdDisassociate:
	default:
		return DisconnectReason{}, false
	}

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return DisconnectReason{}, false
	}

	var (
		sameInterface bool
		code          uint16
		haveCode      bool
		frame         []byte
		fromAP        bool
	)
	for ad.Next() {
		switch ad.Type() {
		case nl80211AttrIfindex:
			sameInterface = ad.Uint32() == ifindex
		case nl80211AttrReasonCode:
			code = ad.Uint16()
			haveCode = true
		case nl80211AttrFrame:
			frame = ad.Bytes()
		case nl80211AttrDisconnectedByAP:
			fromAP = true
		}
	}
	if ad.Err() != nil || !sameInterface {
		return DisconnectReason{}, false
	}

	// DISCONNECT events carry the code as an attribute; DEAUTHENTICATE and
	// DISASSOCIATE instead carry the raw management frame, whose body ends
	// with the reason code field.
	if !haveCode {
		code, haveCode = reasonFromMgmtFrame(frame)
	}
	if !haveCode {
		return DisconnectReason{}, false
	}
	return DisconnectReason{Code: code, FromAP: fromAP}, true
}

// reasonFromMgmtFrame pulls the reason code out of a raw deauthentication
// or disassociation management frame (NL80211_ATTR_FRAME): both frame
// bodies end with the 2-byte little-endian Reason Code field.
func reasonFromMgmtFrame(frame []byte) (uint16, bool) {
	if len(frame) < 2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(frame[len(frame)-2:]), true
}
