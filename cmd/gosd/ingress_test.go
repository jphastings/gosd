package main

import (
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pi3b"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
)

func TestParseIngressFlagsRecognizesTailscaleFunnel(t *testing.T) {
	sel, err := parseIngressFlags([]string{"tailscale-funnel"})
	if err != nil {
		t.Fatalf("parseIngressFlags: %v", err)
	}
	if !sel.TailscaleFunnel {
		t.Error("parseIngressFlags([tailscale-funnel]).TailscaleFunnel = false, want true")
	}
	if sel.Cloudflared {
		t.Error("parseIngressFlags([tailscale-funnel]).Cloudflared = true, want false")
	}
}

func TestParseIngressFlagsAcceptsBothAgentsTogether(t *testing.T) {
	sel, err := parseIngressFlags([]string{"cloudflared", "tailscale-funnel"})
	if err != nil {
		t.Fatalf("parseIngressFlags: %v", err)
	}
	if !sel.Cloudflared || !sel.TailscaleFunnel {
		t.Errorf("parseIngressFlags([cloudflared, tailscale-funnel]) = %+v, want both true", sel)
	}
}

// TestValidateIngressAcceptsTailscaleFunnelOnEveryBoard confirms the epic's
// "ALL boards" board rule: unlike cloudflared, tailscale-funnel must not
// refuse pi-zero-w (armv6/GOARM=6) - gosd compiles the shim itself, so
// there's no upstream GOARM=7-only asset to be incompatible with.
func TestValidateIngressAcceptsTailscaleFunnelOnEveryBoard(t *testing.T) {
	selected := []boards.Board{pizero2w.New(), pizerow.New(), pi3b.New()}
	sel := ingressSelection{TailscaleFunnel: true}
	if err := validateIngress(selected, sel); err != nil {
		t.Errorf("validateIngress(tailscale-funnel, every board incl. pi-zero-w) = %v, want nil", err)
	}
}

func TestValidateIngressDataPartitionNoOpWhenAgentNotSelected(t *testing.T) {
	if err := validateIngressDataPartition(ingressSelection{Cloudflared: true}, 0, false); err != nil {
		t.Errorf("validateIngressDataPartition(cloudflared only, no data) = %v, want nil (cloudflared has no data-partition requirement)", err)
	}
}

func TestValidateIngressDataPartitionRefusesTailscaleFunnelWithNoDataPartition(t *testing.T) {
	err := validateIngressDataPartition(ingressSelection{TailscaleFunnel: true}, 0, false)
	if err == nil {
		t.Fatal("validateIngressDataPartition(tailscale-funnel, no data partition) succeeded, want an error")
	}
	for _, want := range []string{"tailscale-funnel", "data partition", "--data-size", "--data-size=expand"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestValidateIngressDataPartitionAcceptsExplicitDataSize(t *testing.T) {
	if err := validateIngressDataPartition(ingressSelection{TailscaleFunnel: true}, 64*1024*1024, false); err != nil {
		t.Errorf("validateIngressDataPartition(tailscale-funnel, --data-size=64MiB) = %v, want nil", err)
	}
}

func TestValidateIngressDataPartitionAcceptsExpand(t *testing.T) {
	if err := validateIngressDataPartition(ingressSelection{TailscaleFunnel: true}, 0, true); err != nil {
		t.Errorf("validateIngressDataPartition(tailscale-funnel, --data-size=expand) = %v, want nil", err)
	}
}
