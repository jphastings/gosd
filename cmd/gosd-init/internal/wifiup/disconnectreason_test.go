package wifiup

import (
	"testing"

	"github.com/mdlayher/netlink"
)

// deauthFrame fabricates a management frame: a 24-byte MAC header
// followed by the little-endian reason code that ends the body.
func deauthFrame(reason uint16) []byte {
	return append(make([]byte, 24), byte(reason), byte(reason>>8))
}

func TestParseDisconnectEvent(t *testing.T) {
	const ifindex = 3

	tests := []struct {
		name   string
		cmd    uint8
		attrs  func(*netlink.AttributeEncoder)
		want   DisconnectReason
		wantOK bool
	}{
		{
			name: "disconnect event with reason attribute and by-AP flag",
			cmd:  nl80211CmdDisconnect,
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex)
				ae.Uint16(nl80211AttrReasonCode, 15)
				ae.Flag(nl80211AttrDisconnectedByAP, true)
			},
			want:   DisconnectReason{Code: 15, FromAP: true},
			wantOK: true,
		},
		{
			name: "deauthenticate event carries reason in the frame's last two bytes",
			cmd:  nl80211CmdDeauthenticate,
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex)
				ae.Bytes(nl80211AttrFrame, deauthFrame(2))
			},
			want:   DisconnectReason{Code: 2},
			wantOK: true,
		},
		{
			name: "disassociate event parses the same way",
			cmd:  nl80211CmdDisassociate,
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex)
				ae.Bytes(nl80211AttrFrame, deauthFrame(4))
			},
			want:   DisconnectReason{Code: 4},
			wantOK: true,
		},
		{
			name: "event for another interface is filtered out",
			cmd:  nl80211CmdDisconnect,
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex+1)
				ae.Uint16(nl80211AttrReasonCode, 15)
			},
		},
		{
			name: "unrelated command is ignored",
			cmd:  46, // NL80211_CMD_CONNECT
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex)
				ae.Uint16(nl80211AttrReasonCode, 15)
			},
		},
		{
			name: "disconnect with no reason code reports nothing",
			cmd:  nl80211CmdDisconnect,
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex)
			},
		},
		{
			name: "frame too short for a reason code reports nothing",
			cmd:  nl80211CmdDeauthenticate,
			attrs: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(nl80211AttrIfindex, ifindex)
				ae.Bytes(nl80211AttrFrame, []byte{0x0f})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ae := netlink.NewAttributeEncoder()
			tt.attrs(ae)
			data, err := ae.Encode()
			if err != nil {
				t.Fatalf("encoding attributes: %v", err)
			}

			got, ok := parseDisconnectEvent(tt.cmd, data, ifindex)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("parseDisconnectEvent() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestDisconnectReasonString(t *testing.T) {
	tests := []struct {
		reason DisconnectReason
		want   string
	}{
		{DisconnectReason{Code: 15, FromAP: true}, "reason 15 (4-way handshake timeout), reported by AP"},
		{DisconnectReason{Code: 2}, "reason 2 (previous authentication no longer valid)"},
		{DisconnectReason{Code: 99}, "reason 99"},
	}
	for _, tt := range tests {
		if got := tt.reason.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.reason, got, tt.want)
		}
	}
}
