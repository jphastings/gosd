package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildIngressCloudflaredEmbedsBinaryAndFlagsConfig is the acceptance
// test for gosd-g4km's build rail: --ingress cloudflared, resolved entirely
// from the --artifacts-dir well-known-name override (never the network -
// see noNetworkTransport), lands the fixture binary at /bin/cloudflared
// mode 0755 and bakes config.json's ingressCloudflared bit - the entire
// build->runtime contract for this feature (see
// initcfg.Config.IngressCloudflared's doc comment).
func TestBuildIngressCloudflaredEmbedsBinaryAndFlagsConfig(t *testing.T) {
	noNetworkTransport(t)

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--ingress", "cloudflared",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --ingress cloudflared failed: %v", err)
	}

	records := readImageInitramfs(t, imgPath)
	const wantName = "bin/cloudflared"
	rec, ok := findRecord(records, wantName)
	if !ok {
		t.Fatalf("initramfs is missing %q; got entries %v", wantName, recordNames(records))
	}
	if mode := rec.Mode & 0o777; mode != 0o755 {
		t.Errorf("%s mode = %#o, want 0755", wantName, mode)
	}

	got := recordContent(t, records, wantName)
	want, err := os.ReadFile("testdata/fake-artifacts/cloudflared-linux-arm64")
	if err != nil {
		t.Fatalf("reading fixture back: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s content does not match the fixture binary's bytes", wantName)
	}

	configJSON := string(recordContent(t, records, "etc/gosd/config.json"))
	if !strings.Contains(configJSON, `"ingressCloudflared":true`) {
		t.Errorf("config.json = %s, want it to contain %q", configJSON, `"ingressCloudflared":true`)
	}

	// gosd.toml's [ingress.cloudflared] commented example (bean gosd-7upw)
	// ships on every image regardless of --ingress - it's the on-device
	// declaration surface, orthogonal to whether the binary is baked - so
	// it must still be present here too.
	gosdToml := string(readBootFile(t, imgPath, "gosd.toml"))
	if !strings.Contains(gosdToml, "[ingress.cloudflared]") {
		t.Errorf("gosd.toml = %s, want it to contain the [ingress.cloudflared] example", gosdToml)
	}
}

// TestBuildWithoutIngressOmitsCloudflaredBinaryAndConfigFlag confirms
// --ingress is genuinely opt-in: with no --ingress flag, neither
// /bin/cloudflared nor config.json's ingressCloudflared key appears, and
// (via noNetworkTransport) the cloudflared binary is never fetched.
func TestBuildWithoutIngressOmitsCloudflaredBinaryAndConfigFlag(t *testing.T) {
	noNetworkTransport(t)

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	records := readImageInitramfs(t, imgPath)
	if hasRecord(records, "bin/cloudflared") {
		t.Error("initramfs unexpectedly contains bin/cloudflared without --ingress cloudflared")
	}

	configJSON := string(recordContent(t, records, "etc/gosd/config.json"))
	if strings.Contains(configJSON, "ingressCloudflared") {
		t.Errorf("config.json = %s, should not mention ingressCloudflared without --ingress cloudflared", configJSON)
	}
}

// TestBuildIngressRejectsUnsupportedValue confirms --ingress fails fast,
// before any compilation, on any value other than "cloudflared".
func TestBuildIngressRejectsUnsupportedValue(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--ingress", "ngrok",
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --ingress ngrok succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "ngrok") {
		t.Errorf("error = %q, want it to mention the invalid value %q", err.Error(), "ngrok")
	}
}

// TestBuildIngressRejectsPiZeroW confirms --ingress cloudflared refuses
// pi-zero-w with the board-specific armv6/GOARM=7 wording (locked decision,
// epic gosd-virc), rather than either silently ignoring the flag or
// shipping a binary that faults with "illegal instruction" at boot.
func TestBuildIngressRejectsPiZeroW(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--ingress", "cloudflared",
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --ingress cloudflared --board pi-zero-w succeeded, want an error")
	}
	for _, want := range []string{"pi-zero-w", "armv6", "GOARM=7"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

// TestBuildIngressRejectsPiZeroWButNamesCapableBoards confirms a mixed
// --board selection names which selected boards DO support --ingress
// cloudflared and suggests --board= to narrow the build, mirroring
// validateUsbGadget's equivalent multi-board wording.
func TestBuildIngressRejectsPiZeroWButNamesCapableBoards(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-w",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--ingress", "cloudflared",
		"-o", filepath.Join(t.TempDir(), "hello.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --ingress cloudflared with a mixed pi-zero-w/pi-zero-2w selection succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--board=pi-zero-2w") {
		t.Errorf("error = %q, want it to suggest --board=pi-zero-2w", err.Error())
	}
}

// TestBuildIngressCollidesWithReservedDest confirms --with-external can't be
// used to smuggle a file over gosd's own --ingress cloudflared binary or the
// baked CA bundle - the collision check runs before any compilation, the
// same way TestBuildWithExternalRejectsCollisionWithReservedDest checks
// /app.
func TestBuildIngressCollidesWithReservedDest(t *testing.T) {
	for _, dest := range []string{ingressCloudflaredDest, "/etc/ssl/certs/ca-certificates.crt"} {
		cmd := newRootCmd()
		cmd.SetArgs([]string{
			"build", "../../examples/hello",
			"--board", "pi-zero-2w",
			"--artifacts-dir", "testdata/fake-artifacts",
			"--with-external", "./testdata/fake-artifacts/kernel8.img:" + dest,
			"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
		})
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("gosd build --with-external ...:%s succeeded, want an error", dest)
		}
		if !strings.Contains(err.Error(), dest) {
			t.Errorf("error = %q, want it to mention the colliding dest %q", err.Error(), dest)
		}
	}
}
