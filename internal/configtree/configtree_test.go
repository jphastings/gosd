package configtree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeOverlay creates a --config-dir with the given path->content files.
func writeOverlay(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func valueByPath(t *testing.T, tree Tree, p string) Value {
	t.Helper()
	for _, v := range tree.Values {
		if v.Path == p {
			return v
		}
	}
	t.Fatalf("tree has no value %q; it has %v", p, valuePaths(tree))
	return Value{}
}

func valuePaths(tree Tree) []string {
	paths := make([]string, len(tree.Values))
	for i, v := range tree.Values {
		paths[i] = v.Path
	}
	return paths
}

func hasPath(tree Tree, p string) bool {
	for _, v := range tree.Values {
		if v.Path == p {
			return true
		}
	}
	for _, d := range tree.Docs {
		if d.Path == p {
			return true
		}
	}
	return false
}

func TestBuildDefaultsAreDocumentedAndPadded(t *testing.T) {
	tree, err := Build("", Features{})
	if err != nil {
		t.Fatalf("building gosd's own defaults failed: %v", err)
	}

	for _, want := range []string{"hostname", "wifi/ssid", "wifi/passphrase", "data_flush"} {
		v := valueByPath(t, tree, want)
		if len(v.Content) < MinValueBytes {
			t.Errorf("%s reserves %d bytes, want at least %d", want, len(v.Content), MinValueBytes)
		}
		if v.Value != "" {
			t.Errorf("%s ships set to %q; gosd's own defaults are all unset", want, v.Value)
		}
		if !hasPath(tree, want+DocSuffix) {
			t.Errorf("%s ships with no %s sidecar", want, DocSuffix)
		}
	}

	if !hasPath(tree, "env/"+GroupDoc) {
		t.Error("env/ ships without its group explain.md, so nothing tells a customer they can add app settings")
	}
}

func TestBuildPrunesFeaturesTheImageDoesNotCarry(t *testing.T) {
	off, err := Build("", Features{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"ingress/" + GroupDoc, "ingress/cloudflared/token", "ingress/tailscale-funnel/authkey"} {
		if hasPath(off, p) {
			t.Errorf("a build with no --ingress still wrote %s", p)
		}
	}

	on, err := Build("", Features{IngressCloudflared: true})
	if err != nil {
		t.Fatal(err)
	}
	if !hasPath(on, "ingress/cloudflared/token") || !hasPath(on, "ingress/"+GroupDoc) {
		t.Errorf("--ingress cloudflared didn't write its settings; got %v", valuePaths(on))
	}
	if hasPath(on, "ingress/tailscale-funnel/authkey") {
		t.Error("--ingress cloudflared also wrote tailscale-funnel's settings")
	}
}

func TestBuildOverlayWinsPerFileAndInheritsExplanations(t *testing.T) {
	overlay := writeOverlay(t, map[string]string{
		"hostname":                  "kitchen-clock\n",
		"env/API_TOKEN":             "",
		"env/API_TOKEN" + DocSuffix: "# API token\n\nThe token the app talks to its server with.\n",
	})

	tree, err := Build(overlay, Features{})
	if err != nil {
		t.Fatalf("building with an app overlay failed: %v", err)
	}

	hostname := valueByPath(t, tree, "hostname")
	if hostname.Value != "kitchen-clock" {
		t.Errorf("hostname reads as %q, want the app's own baked value", hostname.Value)
	}
	if !hasPath(tree, "hostname"+DocSuffix) {
		t.Error("an overridden value lost gosd's inherited explanation")
	}
	if got := valueByPath(t, tree, "env/API_TOKEN"); len(got.Content) != MinValueBytes {
		t.Errorf("env/API_TOKEN reserves %d bytes, want the %d-byte minimum", len(got.Content), MinValueBytes)
	}
}

func TestBuildReservationIsTheShippedSize(t *testing.T) {
	overlay := writeOverlay(t, map[string]string{
		"blob":             strings.Repeat("\n", 4096),
		"blob" + DocSuffix: "# blob\n\nHolds a big injected value.\n",
	})

	tree, err := Build(overlay, Features{})
	if err != nil {
		t.Fatal(err)
	}

	blob := valueByPath(t, tree, "blob")
	if len(blob.Content) != 4096 {
		t.Errorf("a 4096-byte value file reserved %d bytes, want its own size", len(blob.Content))
	}
	if blob.Value != "" {
		t.Errorf("a padding-only value reads as %q, want unset", blob.Value)
	}
}

func TestBuildRefusals(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name:    "an undocumented setting",
			files:   map[string]string{"secret": ""},
			wantErr: "no documentation",
		},
		{
			name:    "documentation for a setting that doesn't exist",
			files:   map[string]string{"secrt" + DocSuffix: "# typo\n"},
			wantErr: "doesn't exist",
		},
		{
			name:    "a reserved .new suffix",
			files:   map[string]string{"hostname.new": "", "hostname.new" + DocSuffix: "# no\n"},
			wantErr: "reserved suffix",
		},
		{
			name:    "a macOS AppleDouble file",
			files:   map[string]string{"._hostname": ""},
			wantErr: "AppleDouble",
		},
		{
			name:    "a dot-file",
			files:   map[string]string{".DS_Store": ""},
			wantErr: "starts with a period",
		},
		{
			name:    "Windows folder metadata",
			files:   map[string]string{"Thumbs.db": ""},
			wantErr: "operating-system metadata",
		},
		{
			name:    "a GOSD_* environment variable",
			files:   map[string]string{"env/GOSD_BOARD": "", "env/GOSD_BOARD" + DocSuffix: "# no\n"},
			wantErr: "GOSD_* namespace",
		},
		{
			name:    "an environment variable that isn't a usable name",
			files:   map[string]string{"env/2fast": "", "env/2fast" + DocSuffix: "# no\n"},
			wantErr: "valid environment variable name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(writeOverlay(t, tc.files), Features{})
			if err == nil {
				t.Fatalf("building with %s succeeded; want a refusal mentioning %q", tc.name, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("refusal was %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// A FAT card can't hold two names differing only in case, and neither can
// the macOS filesystem a developer most often authors a --config-dir on -
// so this collision is only reachable from a case-sensitive host, and the
// gate is exercised against the merged file set directly rather than
// through a directory this test can't portably create.
func TestValidateRefusesCaseInsensitiveCollisions(t *testing.T) {
	files := map[string]entry{
		"env/TOKEN":             {source: "a"},
		"env/TOKEN" + DocSuffix: {source: "a"},
		"env/token":             {source: "b"},
		"env/token" + DocSuffix: {source: "b"},
	}

	err := validate(files)
	if err == nil || !strings.Contains(err.Error(), "differ only in capitalization") {
		t.Fatalf("validating two case-colliding settings gave %v, want a refusal naming the collision", err)
	}
}

func TestValidateRefusesASettingThatIsAlsoADirectory(t *testing.T) {
	files := map[string]entry{
		"wifi":                  {source: "a"},
		"wifi" + DocSuffix:      {source: "a"},
		"wifi/ssid":             {source: "b"},
		"wifi/ssid" + DocSuffix: {source: "b"},
	}

	err := validate(files)
	if err == nil || !strings.Contains(err.Error(), "both a setting and a directory") {
		t.Fatalf("validating a setting that is also a directory gave %v, want a refusal", err)
	}
}

func TestBuildRefusesAMissingConfigDir(t *testing.T) {
	_, err := Build(filepath.Join(t.TempDir(), "nope"), Features{})
	if err == nil || !strings.Contains(err.Error(), "--config-dir") {
		t.Fatalf("a missing --config-dir gave %v, want a refusal naming the flag", err)
	}
}

func TestTreeBootFilesAndDigests(t *testing.T) {
	tree, err := Build("", Features{})
	if err != nil {
		t.Fatal(err)
	}

	files := tree.BootFiles()
	if _, ok := files[Dir+"/wifi/ssid"]; !ok {
		t.Errorf("boot files are not keyed by their path on the card; got %d files, none at %s/wifi/ssid", len(files), Dir)
	}

	digests := tree.Digests()
	if len(digests) != len(tree.Values) {
		t.Errorf("config.json would carry %d digests for %d values", len(digests), len(tree.Values))
	}
	if digests["wifi/ssid"] != valueByPath(t, tree, "wifi/ssid").SHA256() {
		t.Error("a digest doesn't match the bytes written to the card")
	}
}

// TestIgnoredNameCoversEveryNameTheBuildRefusesAsJunk pins the two halves
// of the same rule against each other: `gosd build` refuses these names so
// an app can never ship one, and the device ignores them because a card is
// edited by hand on machines that write them unbidden. A name that only one
// half knows about is a hole - a setting that ships but never takes effect,
// or a stray file read as one.
func TestIgnoredNameCoversEveryNameTheBuildRefusesAsJunk(t *testing.T) {
	refused := []string{
		"hostname" + NewSuffix,
		"hostname" + UnusedSuffix,
		"._hostname",
		".DS_Store",
		"Thumbs.db",
		"thumbs.db",
		"desktop.ini",
	}
	for _, name := range refused {
		if !IgnoredName(name) {
			t.Errorf("IgnoredName(%q) = false; the build refuses that name, so the device must read past it", name)
		}
		if err := checkName(name, name, "dir"); err == nil {
			t.Errorf("checkName(%q) = nil; the device ignores that name, so the build must refuse it", name)
		}
	}

	// Documentation is the exception: the build writes it and the device
	// reads past it, since a sidecar explains a setting rather than being
	// one.
	for _, name := range []string{GroupDoc, "hostname" + DocSuffix} {
		if !IgnoredName(name) {
			t.Errorf("IgnoredName(%q) = false, want documentation read past rather than read as a setting", name)
		}
	}

	for _, name := range []string{"hostname", "ssid", "API_TOKEN", "google-service-account.json"} {
		if IgnoredName(name) {
			t.Errorf("IgnoredName(%q) = true, want a setting to be read as one", name)
		}
	}
}

// restorableCredentialShaped names the settings whose names read like a
// credential but which gosd-init still restores from the copy kept on the
// data partition, each with the reason a person gave for it. It exists so
// that the test below can insist every credential-shaped setting has been
// classified deliberately, in one direction or the other, rather than
// silently defaulting to "restorable" the day it is added.
var restorableCredentialShaped = map[string]string{
	"wifi/passphrase": "meaningless without the SSID beside it, which is not credential-shaped and has to be restored for a device to rejoin its network unattended after a re-flash; refusing only the passphrase would leave a device trying to join its own network with no key, or joining an attacker's open one",
}

func TestEveryCredentialShapedDefaultIsClassifiedByHand(t *testing.T) {
	// A new ingress agent brings a new token file with it. Restoring one
	// from unauthenticated storage is what bean gosd-7m9y is about, so the
	// question has to be answered when the setting is added rather than
	// noticed afterwards.
	credentialWords := []string{"token", "authkey", "auth_key", "secret", "password", "passphrase", "apikey", "api_key", "credential"}

	tree, err := Build("", Features{IngressCloudflared: true, IngressTailscaleFunnel: true})
	if err != nil {
		t.Fatalf("building gosd's own defaults failed: %v", err)
	}

	for _, v := range tree.Values {
		name := strings.ToLower(filepath.Base(v.Path))
		shaped := false
		for _, word := range credentialWords {
			if strings.Contains(name, word) {
				shaped = true
				break
			}
		}
		if !shaped {
			continue
		}
		if IsCredential(v.Path) {
			continue
		}
		if _, exempted := restorableCredentialShaped[v.Path]; !exempted {
			t.Errorf("%s reads like a bearer credential but gosd-init would restore it from the unauthenticated copy on /data.\n"+
				"Decide which it is: add it to credentialPaths in configtree.go so it is never kept, or to restorableCredentialShaped in this test with the reason it is safe to put back.", v.Path)
		}
	}
}

func TestCredentialPathsNameSettingsThatActuallyShip(t *testing.T) {
	// A credential path that matches nothing is a rename nobody noticed:
	// the refusal quietly stops applying while still reading as though it
	// does.
	tree, err := Build("", Features{IngressCloudflared: true, IngressTailscaleFunnel: true})
	if err != nil {
		t.Fatalf("building gosd's own defaults failed: %v", err)
	}

	shipped := make(map[string]bool, len(tree.Values))
	for _, v := range tree.Values {
		shipped[v.Path] = true
	}
	for p := range credentialPaths {
		if !shipped[p] {
			t.Errorf("credentialPaths names %q, which no image ships; it protects nothing", p)
		}
	}
}

func TestPlausibleValueRefusesOnlyANul(t *testing.T) {
	// A multi-line value is a real thing somebody pastes into config/env/,
	// so the gate has to let one through; a NUL cannot survive execve(2)
	// and so is never a value anybody meant.
	for _, ok := range []string{"", "plain", "line one\nline two\n", "  spaced  "} {
		if !PlausibleValue([]byte(ok)) {
			t.Errorf("PlausibleValue(%q) = false, want true", ok)
		}
	}
	if PlausibleValue([]byte("before\x00after")) {
		t.Error("PlausibleValue accepted a NUL byte")
	}
}

func TestIsCredentialIgnoresCapitalizationTheWayAFatCardDoes(t *testing.T) {
	// The store may live on a FAT data partition and the tree always lives
	// on a FAT boot partition, where "Token" and "token" are one file. A
	// case-sensitive refusal would be one an attacker walks around by
	// changing a letter.
	for _, spelling := range []string{
		"ingress/cloudflared/token",
		"ingress/cloudflared/Token",
		"ingress/cloudflared/TOKEN",
		"Ingress/Cloudflared/token",
		"ingress/tailscale-funnel/AuthKey",
	} {
		if !IsCredential(spelling) {
			t.Errorf("IsCredential(%q) = false; on a FAT card that is the same file as the credential", spelling)
		}
	}
	if IsCredential("ingress/cloudflared/hostname") {
		t.Error("IsCredential refused the tunnel's hostname, which is not a credential and has to be restorable")
	}
}
