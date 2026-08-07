package gosdtoml

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderWithValuesRoundTripsThroughParse(t *testing.T) {
	env := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}
	out := Render("my-device", true, "home-network", "hunter2", env, IngressCloudflared{})

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	want := Config{
		Hostname: "my-device",
		Wifi:     Wifi{SSID: "home-network", Passphrase: "hunter2"},
		Env:      env,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(Render(...)) = %+v, want %+v", got, want)
	}

	// Checked against the specific commented line, not just "# hostname":
	// the ingress example below also legitimately contains "# hostname =
	// ..." (its own, unrelated field), which a bare substring check would
	// mistake for the top-level hostname line staying commented out.
	if strings.Contains(string(out), `# hostname = "my-device"`) {
		t.Errorf("Render() commented out hostname despite a value being set:\n%s", out)
	}
	if strings.Contains(string(out), "# ssid") {
		t.Errorf("Render() commented out wifi despite a value being set:\n%s", out)
	}
	if strings.Contains(string(out), "# [env]") {
		t.Errorf("Render() commented out [env] despite values being set:\n%s", out)
	}
}

func TestRenderWithoutValuesProducesCommentedExamplesThatParseAsEmpty(t *testing.T) {
	out := Render("", false, "", "", nil, IngressCloudflared{})

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if !reflect.DeepEqual(got, Config{}) {
		t.Errorf(`Parse(Render("", false, "", "", nil, IngressCloudflared{})) = %+v, want zero Config (all commented out)`, got)
	}

	for _, want := range []string{
		`# hostname = "my-device"`,
		`# ssid = "MyHomeNetwork"`,
		`# passphrase = "MyWiFiPassword"`,
		`# NAME = "value"`,
		`# [ingress.cloudflared]`,
		`# token = "paste-your-tunnel-token-here"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf(`Render("", false, "", "", nil, IngressCloudflared{}) missing example line %q:`+"\n%s", want, out)
		}
	}
}

// TestRenderWithUnbakedHostnameShowsItAsACommentedExample covers the common
// build-time case: a computed default hostname (the sanitized package name)
// that wasn't explicitly chosen via --hostname. It must render commented,
// like the fully-unset case above, but show the actual default as the
// example rather than the generic placeholder - and Parse must still see no
// hostname at all, so a wizard-provided cloud-init hostname is free to take
// effect (bean gosd-4hz1).
func TestRenderWithUnbakedHostnameShowsItAsACommentedExample(t *testing.T) {
	out := Render("default-app-name", false, "", "", nil, IngressCloudflared{})

	got, _, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if got.Hostname != "" {
		t.Errorf("Parse(Render(\"default-app-name\", false, ...)).Hostname = %q, want empty (commented out)", got.Hostname)
	}
	if !strings.Contains(string(out), `# hostname = "default-app-name"`) {
		t.Errorf(`Render("default-app-name", false, "", "", nil, IngressCloudflared{}) missing commented example line for the default hostname:`+"\n%s", out)
	}
	if strings.Contains(string(out), "\nhostname = ") {
		t.Errorf("Render() baked an uncommented hostname line despite bakeHostname=false:\n%s", out)
	}
}

func TestRenderIncludesPlainLanguageHeader(t *testing.T) {
	out := string(Render("", false, "", "", nil, IngressCloudflared{}))

	for _, want := range []string{"text editor", "Notepad", "restart it"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() header missing expected plain-language phrase %q:\n%s", want, out)
		}
	}
}

func TestRenderEnvExactOutputWithoutBakedValues(t *testing.T) {
	out := string(Render("", false, "", "", nil, IngressCloudflared{}))

	const want = `
# Extra settings your app reads when it starts, sometimes called
# "environment variables" — most apps don't need any. To add one, remove
# the "#" from the two lines below and change NAME and "value"; add more
# lines the same way for further settings. Names are case-sensitive, and
# values always need double quotes.
# [env]
# NAME = "value"
`
	if !strings.Contains(out, want) {
		t.Errorf("Render(\"\", \"\", \"\", nil, IngressCloudflared{}) missing expected [env] section:\ngot:\n%s\nwant substring:\n%s", out, want)
	}
}

func TestRenderEnvExactOutputWithBakedValuesIsSortedAndDeterministic(t *testing.T) {
	env := map[string]string{"ZEBRA": "z", "API_URL": "https://example.com", "DEBUG": "true"}

	const want = `
# Extra settings your app reads when it starts, sometimes called
# "environment variables". To change one, edit the value between the
# quotes below; to add another, add a line like NAME = "value". Names are
# case-sensitive, and values always need double quotes.
[env]
API_URL = "https://example.com"
DEBUG = "true"
ZEBRA = "z"
`

	for i := 0; i < 5; i++ {
		out := string(Render("", false, "", "", env, IngressCloudflared{}))
		if !strings.Contains(out, want) {
			t.Fatalf("Render() [env] section not sorted/deterministic on iteration %d:\ngot:\n%s\nwant substring:\n%s", i, out, want)
		}
	}
}

// TestRenderIngressExactOutputWithoutValues is the ingress schema's golden
// test for the commented-example form: present on every image (there's no
// build-time signal to omit it on), placed right after [env], and stating
// the `gosd build --ingress cloudflared` requirement up front so a hand-
// editing user on any other image knows why filling this in does nothing.
func TestRenderIngressExactOutputWithoutValues(t *testing.T) {
	out := string(Render("", false, "", "", nil, IngressCloudflared{}))

	const want = `
# Makes an app on this device reachable from the internet through a free
# Cloudflare Tunnel — no port forwarding or public IP address needed. This
# only works on a device built with "gosd build --ingress cloudflared"; on
# any other device, filling this in does nothing.
#
# To turn this on, remove the "#" from the start of all three lines below,
# then fill in your own values:
#   token: run "cloudflared tunnel token <tunnel-name>" (or copy it from
#   the Cloudflare dashboard) and paste the long piece of text it prints
#   hostname: the public web address you want to use, for example
#   "app.example.com"
#   port: the port number the app on this device listens on, for example
#   8080
# [ingress.cloudflared]
# token = "paste-your-tunnel-token-here"
# hostname = "app.example.com"
# port = 8080
`
	if !strings.HasSuffix(out, want) {
		t.Errorf("Render(..., IngressCloudflared{}) does not end with the expected commented ingress example:\ngot:\n%s\nwant suffix:\n%s", out, want)
	}
}

// TestRenderIngressExactOutputWithValues is the ingress schema's golden
// test for the configured form: real values render uncommented, and the
// build-requirement prose is dropped since it plainly took effect.
func TestRenderIngressExactOutputWithValues(t *testing.T) {
	ingress := IngressCloudflared{Token: "example-tunnel-token", Hostname: "app.example.com", Port: 8080}
	out := string(Render("", false, "", "", nil, ingress))

	const want = `
# Makes this device's app reachable from the internet through Cloudflare
# Tunnel. To change these, edit the values below.
[ingress.cloudflared]
token = "example-tunnel-token"
hostname = "app.example.com"
port = 8080
`
	if !strings.HasSuffix(out, want) {
		t.Errorf("Render(..., %+v) does not end with the expected ingress block:\ngot:\n%s\nwant suffix:\n%s", ingress, out, want)
	}
}

// TestRenderWithIngressRoundTripsThroughParse mirrors
// TestRenderWithValuesRoundTripsThroughParse for the ingress case: a
// Configured() value round-trips through Parse with no warnings, and the
// commented example text is absent (it's the real block instead).
func TestRenderWithIngressRoundTripsThroughParse(t *testing.T) {
	ingress := IngressCloudflared{Token: "example-tunnel-token", Hostname: "app.example.com", Port: 8080}
	out := Render("my-device", true, "", "", nil, ingress)

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if got.Ingress.Cloudflared != ingress {
		t.Errorf("Parse(Render(...)).Ingress.Cloudflared = %+v, want %+v", got.Ingress.Cloudflared, ingress)
	}
	if strings.Contains(string(out), "# [ingress.cloudflared]") {
		t.Errorf("Render() commented out ingress despite a Configured() value being set:\n%s", out)
	}
}
