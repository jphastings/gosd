package provsnapshot

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/provision"
)

// store is an in-memory stand-in for the data partition's snapshot
// directory and the boot partition's root, recording what was written so
// tests can assert on writes that did — and didn't — happen.
type store struct {
	files    map[string][]byte
	writes   []string
	writeErr error

	boot    map[string][]byte
	bootErr error

	logs []string
}

func newStore() *store {
	return &store{files: map[string][]byte{}, boot: map[string][]byte{}}
}

func (s *store) deps() Deps {
	return Deps{
		ReadFile: func(name string) ([]byte, error) {
			data, ok := s.files[name]
			if !ok {
				return nil, fmt.Errorf("open %s: %w", name, os.ErrNotExist)
			}
			return data, nil
		},
		WriteFile: func(name string, data []byte) error {
			if s.writeErr != nil {
				return s.writeErr
			}
			s.writes = append(s.writes, name)
			s.files[name] = data
			return nil
		},
		WriteBootFile: func(name string, data []byte) error {
			if s.bootErr != nil {
				return s.bootErr
			}
			s.boot[name] = data
			return nil
		},
		Log: func(format string, args ...any) { s.logs = append(s.logs, fmt.Sprintf(format, args...)) },
	}
}

func (s *store) logged(substr string) bool {
	for _, line := range s.logs {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// snapshot decodes whatever is currently stored, failing the test if it
// isn't a complete, valid snapshot.
func (s *store) snapshot(t *testing.T) Snapshot {
	t.Helper()
	snap, err := decode(s.files[tomlFile], s.files[metaFile])
	if err != nil {
		t.Fatalf("stored snapshot is unreadable: %v", err)
	}
	return snap
}

func (s *store) seed(t *testing.T, snap Snapshot) {
	t.Helper()
	tomlData, metaData, err := snap.encode()
	if err != nil {
		t.Fatalf("encoding the seed snapshot: %v", err)
	}
	s.files[tomlFile] = tomlData
	s.files[metaFile] = metaData
}

func TestSnapshotRecordsWhatTheBootSettledOn(t *testing.T) {
	s := newStore()
	in := Input{
		Identity: "aaaa1111",
		Baked: Provisioning{
			Hostname: "hello",
			Env:      map[string]string{"LOG_LEVEL": "info"},
		},
		// The wizard's WiFi never appears in gosd.toml, so only an
		// effective-value snapshot can carry it across a reflash.
		CloudInit: CloudInit{Wifi: []provision.WifiNetwork{{SSID: "Home", Password: "hunter2"}}},
		GosdToml:  gosdtoml.Config{Hostname: "hello", Env: map[string]string{"LOG_LEVEL": "debug"}},
	}

	Run(s.deps(), in)

	if !s.logged("provisioning snapshot saved") {
		t.Errorf("no save was logged; logs: %v", s.logs)
	}
	snap := s.snapshot(t)
	if snap.Identity != "aaaa1111" {
		t.Errorf("snapshot identity = %q, want the running image's", snap.Identity)
	}
	want := Provisioning{
		Hostname: "hello",
		Wifi:     gosdtoml.Wifi{SSID: "Home", Passphrase: "hunter2"},
		Env:      map[string]string{"LOG_LEVEL": "debug"},
	}
	if !snap.Effective.equal(want) {
		t.Errorf("snapshot effective = %+v, want %+v", snap.Effective, want)
	}
	if !snap.Baked.equal(in.Baked) {
		t.Errorf("snapshot baked defaults = %+v, want %+v", snap.Baked, in.Baked)
	}
}

func TestSnapshotIsNotRewrittenWhenNothingChanged(t *testing.T) {
	s := newStore()
	in := Input{
		Identity: "aaaa1111",
		Baked:    Provisioning{Hostname: "hello"},
		GosdToml: gosdtoml.Config{Hostname: "hello"},
	}

	Run(s.deps(), in)
	writesAfterFirstBoot := len(s.writes)
	Run(s.deps(), in)

	if len(s.writes) != writesAfterFirstBoot {
		t.Errorf("second boot rewrote %v; an unchanged snapshot must not be written again", s.writes[writesAfterFirstBoot:])
	}
	if !s.logged("provisioning snapshot unchanged") {
		t.Errorf("the skipped rewrite wasn't logged; logs: %v", s.logs)
	}
}

func TestReflashRestoresHandEditedEnvIntoTheCardsGosdToml(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity:  "old",
		Effective: Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://mine", "LOG_LEVEL": "info"}},
		Baked:     Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "info"}},
	})

	res := Run(s.deps(), Input{
		Identity: "new",
		Baked:    Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "info"}},
		GosdToml: gosdtoml.Config{Hostname: "hello", Env: map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "info"}},
	})

	if got := res.GosdToml.Env["API_URL"]; got != "https://mine" {
		t.Errorf("API_URL = %q for this boot, want the hand-edited value restored", got)
	}
	written, ok := s.boot[BootConfigFile]
	if !ok {
		t.Fatalf("nothing was written back to %s on the boot partition", BootConfigFile)
	}
	back, _, err := gosdtoml.Parse(written)
	if err != nil {
		t.Fatalf("the gosd.toml written back doesn't parse: %v", err)
	}
	if back.Env["API_URL"] != "https://mine" {
		t.Errorf("written gosd.toml API_URL = %q, want the restored value visible to the operator", back.Env["API_URL"])
	}
	if snap := s.snapshot(t); snap.Identity != "new" {
		t.Errorf("snapshot identity = %q after a successful heal, want the running image's", snap.Identity)
	}
}

func TestReflashDoesNotRestoreAValueTheNewImagesTemplateChanged(t *testing.T) {
	s := newStore()
	// LOG_LEVEL was never hand-edited: it matched the old image's baked
	// default exactly. API_URL was.
	s.seed(t, Snapshot{
		Identity:  "old",
		Effective: Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://mine", "LOG_LEVEL": "info"}},
		Baked:     Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "info"}},
	})

	newBaked := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "warn"}
	res := Run(s.deps(), Input{
		Identity: "new",
		Baked:    Provisioning{Hostname: "hello", Env: newBaked},
		GosdToml: gosdtoml.Config{Hostname: "hello", Env: maps.Clone(newBaked)},
	})

	if got := res.GosdToml.Env["LOG_LEVEL"]; got != "warn" {
		t.Errorf("LOG_LEVEL = %q, want the new image's changed default to stand", got)
	}
	if got := res.GosdToml.Env["API_URL"]; got != "https://mine" {
		t.Errorf("API_URL = %q, want the hand-edit restored alongside it", got)
	}
}

func TestReflashKeepsAHandEditMadeOnTheNewCard(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity:  "old",
		Effective: Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://mine"}},
		Baked:     Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://example.com"}},
	})

	res := Run(s.deps(), Input{
		Identity: "new",
		Baked:    Provisioning{Hostname: "hello", Env: map[string]string{"API_URL": "https://example.com"}},
		// Edited on the freshly flashed card, before this boot.
		GosdToml: gosdtoml.Config{Hostname: "hello", Env: map[string]string{"API_URL": "https://newer"}},
	})

	if got := res.GosdToml.Env["API_URL"]; got != "https://newer" {
		t.Errorf("API_URL = %q, want the edit on the new card to win over the snapshot", got)
	}
}

func TestReflashRestoresHostnameAndWifiOnlyWhenTheFreshBootHasNone(t *testing.T) {
	seed := Snapshot{
		Identity: "old",
		Effective: Provisioning{
			Hostname: "kitchen-pi",
			Wifi:     gosdtoml.Wifi{SSID: "Home", Passphrase: "hunter2"},
		},
		Baked: Provisioning{Hostname: "hello"},
	}
	// GosdToml.Hostname is empty, not "hello": a non-explicit `gosd build`
	// ships the hostname line commented out (bean gosd-4hz1), so a real
	// freshly flashed card's gosd.toml carries no hostname value at all,
	// same as if the line were absent entirely.
	newImage := Input{
		Identity: "new",
		Baked:    Provisioning{Hostname: "hello"},
		GosdToml: gosdtoml.Config{Hostname: ""},
	}

	t.Run("wizard skipped", func(t *testing.T) {
		s := newStore()
		s.seed(t, seed)

		res := Run(s.deps(), newImage)

		if res.GosdToml.Hostname != "kitchen-pi" {
			t.Errorf("hostname = %q, want it restored from the snapshot", res.GosdToml.Hostname)
		}
		if !res.HostnameRestored {
			t.Error("HostnameRestored = false, want the caller told to re-apply it")
		}
		if res.GosdToml.Wifi != seed.Effective.Wifi {
			t.Errorf("wifi = %+v, want it restored from the snapshot", res.GosdToml.Wifi)
		}
	})

	t.Run("wizard used", func(t *testing.T) {
		s := newStore()
		s.seed(t, seed)

		fresh := newImage
		fresh.CloudInit = CloudInit{
			Hostname: "living-room",
			Wifi:     []provision.WifiNetwork{{SSID: "Office", Password: "swordfish"}},
		}
		res := Run(s.deps(), fresh)

		if res.HostnameRestored || res.GosdToml.Hostname != "" {
			t.Errorf("hostname = %q (restored=%v), want gosd.toml left untouched (still commented out)", res.GosdToml.Hostname, res.HostnameRestored)
		}
		if res.GosdToml.Wifi != (gosdtoml.Wifi{}) {
			t.Errorf("wifi = %+v, want the wizard's network left to win", res.GosdToml.Wifi)
		}
		if _, ok := s.boot[BootConfigFile]; ok {
			t.Error("gosd.toml was rewritten even though the wizard provided everything")
		}
		// Bean gosd-4hz1: a non-explicit build's gosd.toml carries no
		// hostname (it ships commented out), so it no longer shadows the
		// wizard's hostname the way it used to. Both the WiFi network and
		// the hostname the wizard supplied are what actually took effect,
		// so both must be what the refreshed snapshot records.
		snap := s.snapshot(t)
		if snap.Effective.Wifi.SSID != "Office" {
			t.Errorf("snapshot effective = %+v, want it refreshed with the wizard's network", snap.Effective)
		}
		if snap.Effective.Hostname != "living-room" {
			t.Errorf("snapshot effective hostname = %q, want the wizard's hostname — it's no longer shadowed by a commented-out gosd.toml default", snap.Effective.Hostname)
		}
	})

	t.Run("hand-edited on the new card", func(t *testing.T) {
		s := newStore()
		s.seed(t, seed)

		fresh := newImage
		fresh.GosdToml = gosdtoml.Config{Hostname: "bedroom", Wifi: gosdtoml.Wifi{SSID: "Cafe"}}
		res := Run(s.deps(), fresh)

		if res.GosdToml.Hostname != "bedroom" || res.GosdToml.Wifi.SSID != "Cafe" {
			t.Errorf("got hostname %q / ssid %q, want the new card's own edits kept", res.GosdToml.Hostname, res.GosdToml.Wifi.SSID)
		}
	})
}

func TestSnapshotRecordsAHandSetIngressSection(t *testing.T) {
	s := newStore()
	ingress := gosdtoml.IngressCloudflared{Token: "example-tunnel-token", Hostname: "app.example.com", Port: 8080}

	Run(s.deps(), Input{
		Identity: "aaaa1111",
		GosdToml: gosdtoml.Config{Ingress: gosdtoml.Ingress{Cloudflared: ingress}},
	})

	if snap := s.snapshot(t); snap.Effective.Ingress.Cloudflared != ingress {
		t.Errorf("snapshot effective ingress = %+v, want %+v", snap.Effective.Ingress.Cloudflared, ingress)
	}
}

// TestIngressSurvivesAPlainReflashWithNoCredentialsFile is the bean gosd-tgzo
// property: since the tunnel token lives nowhere but gosd.toml (epic
// gosd-virc decision 3 - no separate credentials file exists anywhere on
// the boot partition), restoring the whole [ingress.cloudflared] section
// from the snapshot is the entire mechanism by which a hand-configured tunnel
// survives an ordinary Raspberry Pi Imager reflash, exactly like a
// hand-edited WiFi passphrase already does.
func TestIngressSurvivesAPlainReflashWithNoCredentialsFile(t *testing.T) {
	s := newStore()
	ingress := gosdtoml.IngressCloudflared{Token: "super-secret-tunnel-token", Hostname: "app.example.com", Port: 8080}

	// First boot: the operator hand-edited [ingress.cloudflared] into
	// gosd.toml before ever powering the board on.
	Run(s.deps(), Input{
		Identity: "old",
		GosdToml: gosdtoml.Config{Ingress: gosdtoml.Ingress{Cloudflared: ingress}},
	})

	// Reflash: a new image identity, and a gosd.toml exactly as "gosd build"
	// renders it on every image - no [ingress.cloudflared] at all, since
	// config.json never bakes a token. Deps exposes no credentials-file API
	// of any kind, and the fake boot filesystem here starts empty: nothing
	// but the /data snapshot is available to recover the tunnel from.
	res := Run(s.deps(), Input{
		Identity: "new",
		GosdToml: gosdtoml.Config{},
	})

	if res.GosdToml.Ingress.Cloudflared != ingress {
		t.Errorf("ingress = %+v after reflash, want the hand-set tunnel restored exactly (token included)", res.GosdToml.Ingress.Cloudflared)
	}
	written, ok := s.boot[BootConfigFile]
	if !ok {
		t.Fatalf("nothing was written back to %s on the boot partition", BootConfigFile)
	}
	back, _, err := gosdtoml.Parse(written)
	if err != nil {
		t.Fatalf("the gosd.toml written back doesn't parse: %v", err)
	}
	if back.Ingress.Cloudflared != ingress {
		t.Errorf("written gosd.toml ingress = %+v, want the restored tunnel visible to the operator", back.Ingress.Cloudflared)
	}
}

func TestReflashKeepsAHandSetIngressMadeOnTheNewCard(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity:  "old",
		Effective: Provisioning{Ingress: gosdtoml.Ingress{Cloudflared: gosdtoml.IngressCloudflared{Token: "old-token", Hostname: "old.example.com", Port: 8080}}},
	})

	fresh := gosdtoml.IngressCloudflared{Token: "fresh-token", Hostname: "fresh.example.com", Port: 9090}
	res := Run(s.deps(), Input{
		Identity: "new",
		// Hand-edited on the freshly flashed card, before this boot.
		GosdToml: gosdtoml.Config{Ingress: gosdtoml.Ingress{Cloudflared: fresh}},
	})

	if res.GosdToml.Ingress.Cloudflared != fresh {
		t.Errorf("ingress = %+v, want the tunnel declared on the new card to win over the snapshot", res.GosdToml.Ingress.Cloudflared)
	}
	if _, ok := s.boot[BootConfigFile]; ok {
		t.Error("gosd.toml was rewritten even though the new card already declared its own tunnel")
	}
}

func TestSnapshotRecordsAHandSetTailscaleFunnelSection(t *testing.T) {
	s := newStore()
	ingress := gosdtoml.IngressTailscaleFunnel{Authkey: "tskey-auth-example", Hostname: "my-device", Port: 8080, FunnelPort: 443}

	Run(s.deps(), Input{
		Identity: "aaaa1111",
		GosdToml: gosdtoml.Config{Ingress: gosdtoml.Ingress{TailscaleFunnel: ingress}},
	})

	if snap := s.snapshot(t); snap.Effective.Ingress.TailscaleFunnel != ingress {
		t.Errorf("snapshot effective ingress = %+v, want %+v", snap.Effective.Ingress.TailscaleFunnel, ingress)
	}
}

// TestTailscaleFunnelSurvivesAPlainReflashWithNoCredentialsFile is bean
// gosd-u2gz's counterpart to gosd-tgzo's cloudflared test: since the auth
// key lives nowhere but gosd.toml (same as a Cloudflare Tunnel's token, epic
// gosd-65uy decision 6), restoring the whole [ingress.tailscale-funnel]
// section from the snapshot is what lets a hand-configured Funnel survive an
// ordinary Raspberry Pi Imager reflash, exactly like a hand-edited WiFi
// passphrase or Cloudflare Tunnel already does.
func TestTailscaleFunnelSurvivesAPlainReflashWithNoCredentialsFile(t *testing.T) {
	s := newStore()
	ingress := gosdtoml.IngressTailscaleFunnel{Authkey: "tskey-auth-super-secret", Hostname: "my-device", Port: 8080, FunnelPort: 443}

	// First boot: the operator hand-edited [ingress.tailscale-funnel] into
	// gosd.toml before ever powering the board on.
	Run(s.deps(), Input{
		Identity: "old",
		GosdToml: gosdtoml.Config{Ingress: gosdtoml.Ingress{TailscaleFunnel: ingress}},
	})

	// Reflash: a new image identity, and a gosd.toml exactly as "gosd build"
	// renders it on every image - no [ingress.tailscale-funnel] at all, since
	// config.json never bakes an auth key. Deps exposes no credentials-file
	// API of any kind, and the fake boot filesystem here starts empty:
	// nothing but the /data snapshot is available to recover the Funnel
	// settings from.
	res := Run(s.deps(), Input{
		Identity: "new",
		GosdToml: gosdtoml.Config{},
	})

	if res.GosdToml.Ingress.TailscaleFunnel != ingress {
		t.Errorf("ingress = %+v after reflash, want the hand-set Funnel restored exactly (authkey included)", res.GosdToml.Ingress.TailscaleFunnel)
	}
	written, ok := s.boot[BootConfigFile]
	if !ok {
		t.Fatalf("nothing was written back to %s on the boot partition", BootConfigFile)
	}
	back, _, err := gosdtoml.Parse(written)
	if err != nil {
		t.Fatalf("the gosd.toml written back doesn't parse: %v", err)
	}
	if back.Ingress.TailscaleFunnel != ingress {
		t.Errorf("written gosd.toml ingress = %+v, want the restored Funnel visible to the operator", back.Ingress.TailscaleFunnel)
	}
}

func TestReflashKeepsAHandSetTailscaleFunnelSectionMadeOnTheNewCard(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity:  "old",
		Effective: Provisioning{Ingress: gosdtoml.Ingress{TailscaleFunnel: gosdtoml.IngressTailscaleFunnel{Authkey: "tskey-auth-old", Hostname: "old-device", Port: 8080, FunnelPort: 443}}},
	})

	fresh := gosdtoml.IngressTailscaleFunnel{Authkey: "tskey-auth-fresh", Hostname: "fresh-device", Port: 9090, FunnelPort: 8443}
	res := Run(s.deps(), Input{
		Identity: "new",
		// Hand-edited on the freshly flashed card, before this boot.
		GosdToml: gosdtoml.Config{Ingress: gosdtoml.Ingress{TailscaleFunnel: fresh}},
	})

	if res.GosdToml.Ingress.TailscaleFunnel != fresh {
		t.Errorf("ingress = %+v, want the Funnel declared on the new card to win over the snapshot", res.GosdToml.Ingress.TailscaleFunnel)
	}
	if _, ok := s.boot[BootConfigFile]; ok {
		t.Error("gosd.toml was rewritten even though the new card already declared its own Funnel")
	}
}

// TestTailscaleFunnelReflashRestoresSettingsEvenAfterTheAuthkeyWasRemoved is
// bean gosd-u2gz's "layered reflash property" test. Epic gosd-65uy decision
// 4 lets an operator delete the auth key from gosd.toml once the device has
// registered, since tsnet ignores it once local state exists; decision 3
// puts that state on /data (/data/.gosd/tailscale), so it is untouched by,
// and invisible to, this package. The two layers stack to give the full
// property described in the epic and this bean:
//
//   - Layer 1 (exercised here): this package restores whatever the operator
//     last set for hostname/port/funnel_port from the snapshot, even with no
//     authkey in it at all — the snapshotted section still counts as
//     configured because Hostname/Port/FunnelPort are non-zero, so restore
//     fires exactly as it would for a still-present authkey.
//   - Layer 2 (out of this package's reach, asserted only in the comment
//     above and the epic bean): the reconnecting node keeps the SAME
//     identity and the SAME public https://…ts.net URL regardless of what
//     this restore does to authkey, because that identity already lives on
//     /data and tsnet never re-derives it from gosd.toml.
//
// Together they are why a plain Imager reflash of a --data-size=expand image
// needs no re-auth at all, unlike a Cloudflare Tunnel where the token must
// keep being restored forever.
func TestTailscaleFunnelReflashRestoresSettingsEvenAfterTheAuthkeyWasRemoved(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity: "old",
		Effective: Provisioning{Ingress: gosdtoml.Ingress{TailscaleFunnel: gosdtoml.IngressTailscaleFunnel{
			// No Authkey: the operator already removed it after this
			// device's first successful registration.
			Hostname:   "my-device",
			Port:       8080,
			FunnelPort: 443,
		}}},
	})

	res := Run(s.deps(), Input{
		Identity: "new",
		GosdToml: gosdtoml.Config{},
	})

	want := gosdtoml.IngressTailscaleFunnel{Hostname: "my-device", Port: 8080, FunnelPort: 443}
	if res.GosdToml.Ingress.TailscaleFunnel != want {
		t.Errorf("ingress = %+v after reflash, want %+v restored with no authkey needed", res.GosdToml.Ingress.TailscaleFunnel, want)
	}
	written, ok := s.boot[BootConfigFile]
	if !ok {
		t.Fatalf("nothing was written back to %s on the boot partition", BootConfigFile)
	}
	back, _, err := gosdtoml.Parse(written)
	if err != nil {
		t.Fatalf("the gosd.toml written back doesn't parse: %v", err)
	}
	if back.Ingress.TailscaleFunnel != want {
		t.Errorf("written gosd.toml ingress = %+v, want the restored Funnel settings visible to the operator", back.Ingress.TailscaleFunnel)
	}
}

func TestSameImageBootIsNotTreatedAsAReflash(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity:  "same",
		Effective: Provisioning{Hostname: "kitchen-pi"},
		Baked:     Provisioning{Hostname: "hello"},
	})

	res := Run(s.deps(), Input{
		Identity: "same",
		Baked:    Provisioning{Hostname: "hello"},
		GosdToml: gosdtoml.Config{Hostname: "hello"},
	})

	if res.GosdToml.Hostname != "hello" {
		t.Errorf("hostname = %q, want no restore when the image hasn't changed", res.GosdToml.Hostname)
	}
	if _, ok := s.boot[BootConfigFile]; ok {
		t.Error("gosd.toml was rewritten on an ordinary boot")
	}
}

func TestTornSnapshotIsIgnoredAndBootCarriesOn(t *testing.T) {
	cases := map[string]func(s *store){
		"gosd.toml doesn't match the recorded digest": func(s *store) {
			s.files[tomlFile] = append(s.files[tomlFile], "\nhostname = \"tampered\"\n"...)
		},
		"snapshot.json is truncated mid-write": func(s *store) {
			s.files[metaFile] = s.files[metaFile][:len(s.files[metaFile])/2]
		},
		"gosd.toml never landed": func(s *store) {
			delete(s.files, tomlFile)
		},
	}

	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			s.seed(t, Snapshot{
				Identity:  "old",
				Effective: Provisioning{Hostname: "kitchen-pi"},
				Baked:     Provisioning{Hostname: "hello"},
			})
			damage(s)

			res := Run(s.deps(), Input{
				Identity: "new",
				Baked:    Provisioning{Hostname: "hello"},
				GosdToml: gosdtoml.Config{Hostname: "hello"},
			})

			if res.GosdToml.Hostname != "hello" {
				t.Errorf("hostname = %q, want the boot to proceed with the card's own value", res.GosdToml.Hostname)
			}
			if _, ok := s.boot[BootConfigFile]; ok {
				t.Error("a damaged snapshot was allowed to rewrite gosd.toml")
			}
			if snap := s.snapshot(t); snap.Identity != "new" {
				t.Errorf("snapshot identity = %q, want the damaged snapshot replaced with a good one", snap.Identity)
			}
		})
	}
}

func TestNoDataPartitionSkipsTheSnapshotWithoutFailing(t *testing.T) {
	s := newStore()
	s.writeErr = fmt.Errorf("mkdir /data/.gosd: %w", os.ErrPermission)

	res := Run(s.deps(), Input{
		Identity: "new",
		Baked:    Provisioning{Hostname: "hello"},
		GosdToml: gosdtoml.Config{Hostname: "hello"},
	})

	if res.GosdToml.Hostname != "hello" {
		t.Errorf("hostname = %q, want the boot unaffected by an unwritable /data", res.GosdToml.Hostname)
	}
	if !s.logged("provisioning snapshot not saved") {
		t.Errorf("the skipped snapshot wasn't logged; logs: %v", s.logs)
	}
}

func TestImageWithoutAnIdentityStillSnapshotsButCannotSelfHeal(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Effective: Provisioning{Hostname: "kitchen-pi"},
		Baked:     Provisioning{Hostname: "hello"},
	})

	res := Run(s.deps(), Input{
		Baked:    Provisioning{Hostname: "hello"},
		GosdToml: gosdtoml.Config{Hostname: "hello"},
	})

	if res.GosdToml.Hostname != "hello" {
		t.Errorf("hostname = %q, want no restore without identities to compare", res.GosdToml.Hostname)
	}
	if !s.logged("self-heal skipped") {
		t.Errorf("skipping the self-heal wasn't logged; logs: %v", s.logs)
	}
	if snap := s.snapshot(t); !snap.Effective.equal(Provisioning{Hostname: "hello"}) {
		t.Errorf("snapshot effective = %+v, want it kept up to date anyway", snap.Effective)
	}
}

func TestAFailedWriteBackLeavesTheSnapshotForTheNextBootToRetry(t *testing.T) {
	s := newStore()
	s.seed(t, Snapshot{
		Identity:  "old",
		Effective: Provisioning{Hostname: "kitchen-pi"},
		Baked:     Provisioning{Hostname: "hello"},
	})
	s.bootErr = errors.New("remounting /boot read-write: read-only file system")

	res := Run(s.deps(), Input{
		Identity: "new",
		Baked:    Provisioning{Hostname: "hello"},
		GosdToml: gosdtoml.Config{Hostname: "hello"},
	})

	if res.GosdToml.Hostname != "kitchen-pi" {
		t.Errorf("hostname = %q, want the restore to still apply to this boot", res.GosdToml.Hostname)
	}
	if snap := s.snapshot(t); snap.Identity != "old" {
		t.Errorf("snapshot identity = %q, want the old one kept so the next boot retries the heal", snap.Identity)
	}
}

func TestWriteFileDurablyReplacesContentsAndLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gosd.toml")

	if err := WriteFileDurably(path, []byte("first")); err != nil {
		t.Fatalf("WriteFileDurably() = %v", err)
	}
	if err := WriteFileDurably(path, []byte("second")); err != nil {
		t.Fatalf("WriteFileDurably() second call = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if string(data) != "second" {
		t.Errorf("contents = %q, want %q", data, "second")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing the directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the file itself", len(entries))
	}
}
