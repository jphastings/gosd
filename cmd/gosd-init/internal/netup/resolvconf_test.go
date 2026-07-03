package netup

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteResolvConfListsEachNameserver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")

	err := WriteResolvConf(path, []net.IP{net.IPv4(8, 8, 8, 8), net.IPv4(1, 1, 1, 1)})
	if err != nil {
		t.Fatalf("WriteResolvConf() = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	got := string(data)
	if !strings.Contains(got, "nameserver 8.8.8.8\n") {
		t.Errorf("resolv.conf missing 8.8.8.8: %q", got)
	}
	if !strings.Contains(got, "nameserver 1.1.1.1\n") {
		t.Errorf("resolv.conf missing 1.1.1.1: %q", got)
	}
}

func TestWriteResolvConfOverwritesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 192.0.2.1\n"), 0o644); err != nil {
		t.Fatalf("seeding %s: %v", path, err)
	}

	if err := WriteResolvConf(path, []net.IP{net.IPv4(8, 8, 8, 8)}); err != nil {
		t.Fatalf("WriteResolvConf() = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if strings.Contains(string(data), "192.0.2.1") {
		t.Errorf("resolv.conf still contains the stale nameserver: %q", data)
	}
}

func TestMarkAndClearNetworkUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "gosd", "network-up")

	if err := MarkNetworkUp(path); err != nil {
		t.Fatalf("MarkNetworkUp() = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("marker file missing after MarkNetworkUp(): %v", err)
	}

	if err := ClearNetworkUp(path); err != nil {
		t.Fatalf("ClearNetworkUp() = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("marker file still present after ClearNetworkUp(): err=%v", err)
	}
}

func TestClearNetworkUpOnMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-up")

	if err := ClearNetworkUp(path); err != nil {
		t.Errorf("ClearNetworkUp() on a never-created marker = %v, want nil", err)
	}
}
