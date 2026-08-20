package netup

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestWriteResolvConfWithEmptyDNSLeavesExistingFileIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	const existing = "nameserver 192.0.2.1\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seeding %s: %v", path, err)
	}

	err := WriteResolvConf(path, nil)
	if !errors.Is(err, ErrNoDNSServers) {
		t.Fatalf("WriteResolvConf() with no DNS servers = %v, want ErrNoDNSServers", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(data) != existing {
		t.Errorf("resolv.conf changed despite an empty DNS list: got %q, want %q", data, existing)
	}

	assertOnlyFile(t, path)
}

// assertOnlyFile reports any file beside path in its directory: the scratch
// file WriteResolvConf renames into place must never outlive the call, on
// any path through it.
func assertOnlyFile(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.Dir(path), err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("stray file %q left beside %s", e.Name(), path)
		}
	}
}

// Both of a dual-interface board's DHCP loops (netup's and wifiup's) can
// renew at once, so two WriteResolvConf calls can overlap. Each must publish
// its own complete file, never a blend of the two, and neither may leave its
// scratch file behind.
func TestWriteResolvConfConcurrentWritersEachPublishAWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := net.IPv4(8, 8, 8, byte(i))
			for n := 0; n < 50; n++ {
				if err := WriteResolvConf(path, []net.IP{ip}); err != nil {
					t.Errorf("WriteResolvConf() = %v", err)
					return
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("reading %s: %v", path, err)
					return
				}
				if strings.Count(string(data), "nameserver ") != 1 {
					t.Errorf("resolv.conf is not one writer's whole file: %q", data)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	assertOnlyFile(t, path)
}

func TestWriteResolvConfConcurrentReaderNeverSeesEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := WriteResolvConf(path, []net.IP{net.IPv4(8, 8, 8, 8)}); err != nil {
		t.Fatalf("seeding %s: %v", path, err)
	}

	stop := make(chan struct{})
	readErrs := make(chan error, 1)
	var reads int
	go func() {
		defer close(readErrs)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				readErrs <- err
				return
			}
			if len(data) == 0 {
				readErrs <- errors.New("read an empty resolv.conf mid-write")
				return
			}
			reads++
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			ip := net.IPv4(8, 8, 8, byte(i%256))
			if err := WriteResolvConf(path, []net.IP{ip}); err != nil {
				t.Errorf("WriteResolvConf() = %v", err)
				return
			}
		}
	}()
	wg.Wait()
	close(stop)

	select {
	case err, ok := <-readErrs:
		if ok && err != nil {
			t.Fatalf("reader observed a bad state: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reader goroutine never finished")
	}
	if reads == 0 {
		t.Fatal("reader never observed a successful read")
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
