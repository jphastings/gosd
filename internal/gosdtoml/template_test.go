package gosdtoml

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderWithValuesRoundTripsThroughParse(t *testing.T) {
	env := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}
	out := Render("my-device", true, "home-network", "hunter2", EnvSection{Values: env}, Ingress{})

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
	out := Render("", false, "", "", EnvSection{}, Ingress{})

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if !reflect.DeepEqual(got, Config{}) {
		t.Errorf(`Parse(Render("", false, "", "", EnvSection{}, Ingress{})) = %+v, want zero Config (all commented out)`, got)
	}

	for _, want := range []string{
		`# hostname = "my-device"`,
		`# ssid = "MyHomeNetwork"`,
		`# passphrase = "MyWiFiPassword"`,
		`# NAME = "value"`,
		`# [ingress.cloudflared]`,
		`# token = "paste-your-tunnel-token-here"`,
		`# [ingress.tailscale-funnel]`,
		`# authkey = "tskey-auth-your-key-here"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf(`Render("", false, "", "", EnvSection{}, Ingress{}) missing example line %q:`+"\n%s", want, out)
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
	out := Render("default-app-name", false, "", "", EnvSection{}, Ingress{})

	got, _, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if got.Hostname != "" {
		t.Errorf("Parse(Render(\"default-app-name\", false, ...)).Hostname = %q, want empty (commented out)", got.Hostname)
	}
	if !strings.Contains(string(out), `# hostname = "default-app-name"`) {
		t.Errorf(`Render("default-app-name", false, "", "", EnvSection{}, Ingress{}) missing commented example line for the default hostname:`+"\n%s", out)
	}
	if strings.Contains(string(out), "\nhostname = ") {
		t.Errorf("Render() baked an uncommented hostname line despite bakeHostname=false:\n%s", out)
	}
}

func TestRenderIncludesPlainLanguageHeader(t *testing.T) {
	out := string(Render("", false, "", "", EnvSection{}, Ingress{}))

	for _, want := range []string{"text editor", "Notepad", "restart it"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() header missing expected plain-language phrase %q:\n%s", want, out)
		}
	}
}

func TestRenderEnvExactOutputWithoutBakedValues(t *testing.T) {
	out := string(Render("", false, "", "", EnvSection{}, Ingress{}))

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
		t.Errorf("Render(\"\", \"\", \"\", EnvSection{}, Ingress{}) missing expected [env] section:\ngot:\n%s\nwant substring:\n%s", out, want)
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
		out := string(Render("", false, "", "", EnvSection{Values: env}, Ingress{}))
		if !strings.Contains(out, want) {
			t.Fatalf("Render() [env] section not sorted/deterministic on iteration %d:\ngot:\n%s\nwant substring:\n%s", i, out, want)
		}
	}
}

// TestRenderEnvVerbatimSplicesBodyUnderBareEnvHeader is the gosd build
// --env-file golden: a developer-authored [env] body — comments, a
// commented-out "suggested" entry, and an active entry — is spliced verbatim
// under a bare "[env]" line, exactly as written (including the unquoted
// suggested value the developer chose), with none of gosd's generic [env]
// preamble.
func TestRenderEnvVerbatimSplicesBodyUnderBareEnvHeader(t *testing.T) {
	body := `# uncomment this if you want the demo to run
# RUN_DEMO = true

# Where telemetry is posted; leave blank to disable
API_URL = "https://example.com"`
	out := string(Render("", false, "", "", EnvSection{Verbatim: body}, Ingress{}))

	const want = `
[env]
# uncomment this if you want the demo to run
# RUN_DEMO = true

# Where telemetry is posted; leave blank to disable
API_URL = "https://example.com"
`
	if !strings.Contains(out, want) {
		t.Errorf("Render() verbatim [env]:\ngot:\n%s\nwant substring:\n%s", out, want)
	}
	// The generic "Extra settings your app reads" preamble is gosd's, and the
	// developer owns this section, so it must not appear.
	if strings.Contains(out, "Extra settings your app reads") {
		t.Errorf("Render() added its generic [env] preamble to a verbatim body:\n%s", out)
	}
	// The whole file must still parse, with the suggested entry commented out.
	got, _, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("Parse(Render(verbatim)) error: %v", err)
	}
	if got.Env["API_URL"] != "https://example.com" {
		t.Errorf("active entry API_URL = %q, want the spliced value", got.Env["API_URL"])
	}
	if _, ok := got.Env["RUN_DEMO"]; ok {
		t.Errorf("suggested (commented-out) RUN_DEMO must not be parsed as active: %+v", got.Env)
	}
}

// TestRenderEnvVerbatimTakesPrecedenceOverValues confirms a verbatim body wins
// over Values (which is still set so config.json can bake the active defaults).
func TestRenderEnvVerbatimTakesPrecedenceOverValues(t *testing.T) {
	out := string(Render("", false, "", "", EnvSection{
		Values:   map[string]string{"API_URL": "https://example.com"},
		Verbatim: "# hand-written\nAPI_URL = \"https://example.com\"",
	}, Ingress{}))

	if !strings.Contains(out, "# hand-written") {
		t.Errorf("Render() ignored the verbatim body when Values was also set:\n%s", out)
	}
	if strings.Contains(out, "Extra settings your app reads") {
		t.Errorf("Render() used the plain Values rendering despite a verbatim body:\n%s", out)
	}
}

// TestRenderIngressExactOutputWithoutValues is the ingress schema's golden
// test for the commented-example form: present on every image (there's no
// build-time signal to omit it on), placed right after [env] (and, since
// gosd-85bn, immediately before tailscale-funnel's own block rather than
// at the very end of the file), and stating the
// `gosd build --ingress cloudflared` requirement up front so a hand-
// editing user on any other image knows why filling this in does nothing.
func TestRenderIngressExactOutputWithoutValues(t *testing.T) {
	out := string(Render("", false, "", "", EnvSection{}, Ingress{}))

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
	if !strings.Contains(out, want) {
		t.Errorf("Render(..., Ingress{}) is missing the expected commented ingress example:\ngot:\n%s\nwant substring:\n%s", out, want)
	}
}

// TestRenderIngressExactOutputWithValues is the ingress schema's golden
// test for the configured form: real values render uncommented, and the
// build-requirement prose is dropped since it plainly took effect.
func TestRenderIngressExactOutputWithValues(t *testing.T) {
	cloudflared := IngressCloudflared{Token: "example-tunnel-token", Hostname: "app.example.com", Port: 8080}
	out := string(Render("", false, "", "", EnvSection{}, Ingress{Cloudflared: cloudflared}))

	const want = `
# Makes this device's app reachable from the internet through Cloudflare
# Tunnel. To change these, edit the values below.
[ingress.cloudflared]
token = "example-tunnel-token"
hostname = "app.example.com"
port = 8080
`
	if !strings.Contains(out, want) {
		t.Errorf("Render(..., %+v) is missing the expected ingress block:\ngot:\n%s\nwant substring:\n%s", cloudflared, out, want)
	}
}

// TestRenderWithIngressRoundTripsThroughParse mirrors
// TestRenderWithValuesRoundTripsThroughParse for the ingress case: a
// Configured() value round-trips through Parse with no warnings, and the
// commented example text is absent (it's the real block instead).
func TestRenderWithIngressRoundTripsThroughParse(t *testing.T) {
	cloudflared := IngressCloudflared{Token: "example-tunnel-token", Hostname: "app.example.com", Port: 8080}
	out := Render("my-device", true, "", "", EnvSection{}, Ingress{Cloudflared: cloudflared})

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if got.Ingress.Cloudflared != cloudflared {
		t.Errorf("Parse(Render(...)).Ingress.Cloudflared = %+v, want %+v", got.Ingress.Cloudflared, cloudflared)
	}
	if strings.Contains(string(out), "# [ingress.cloudflared]") {
		t.Errorf("Render() commented out ingress despite a Configured() value being set:\n%s", out)
	}
}

// TestRenderIngressTailscaleFunnelExactOutputWithoutValues is the
// tailscale-funnel schema's golden test for the commented-example form,
// mirroring TestRenderIngressExactOutputWithoutValues: present on every
// image, appended after cloudflared's block, and stating both the
// `gosd build --ingress tailscale-funnel` requirement and that the auth
// key is only needed for first registration.
func TestRenderIngressTailscaleFunnelExactOutputWithoutValues(t *testing.T) {
	out := string(Render("", false, "", "", EnvSection{}, Ingress{}))

	const want = `
# Makes an app on this device reachable from the internet through
# Tailscale Funnel — a public address like
# https://my-device.your-tailnet.ts.net, no port forwarding or public IP
# address needed. This only works on a device built with
# "gosd build --ingress tailscale-funnel"; on any other device, filling
# this in does nothing.
#
# To turn this on, remove the "#" from the start of the lines below, then
# fill in your own values:
#   authkey: create a tagged, reusable auth key in your tailnet's admin
#   console — the tag stops this device's key from expiring. It's only
#   needed the first time this device registers with Tailscale; once
#   that's done you can safely remove it again
#   hostname: the public name to use, for example "device-name" — leave
#   this out to use the device's own hostname
#   port: the port number the app on this device listens on, for example
#   8080
#   funnel_port: which internet-facing port to use, one of 443, 8443 or
#   10000 — leave this out to use the default, 443
# [ingress.tailscale-funnel]
# authkey = "tskey-auth-your-key-here"
# hostname = "device-name"
# port = 8080
# funnel_port = 443
`
	if !strings.HasSuffix(out, want) {
		t.Errorf("Render(..., Ingress{}) does not end with the expected commented tailscale-funnel example:\ngot:\n%s\nwant suffix:\n%s", out, want)
	}
	if !strings.Contains(out, "gosd build --ingress tailscale-funnel") {
		t.Errorf("Render(..., Ingress{}) tailscale-funnel example doesn't state the build-flag requirement:\n%s", out)
	}
	if !strings.Contains(out, "first time this device registers with Tailscale") || !strings.Contains(out, "safely remove it again") {
		t.Errorf("Render(..., Ingress{}) tailscale-funnel example doesn't state the authkey is only needed for first registration:\n%s", out)
	}
}

// TestRenderIngressTailscaleFunnelExactOutputWithValues is the
// tailscale-funnel schema's golden test for the configured form, mirroring
// TestRenderIngressExactOutputWithValues: real values render uncommented,
// appended after cloudflared's block.
func TestRenderIngressTailscaleFunnelExactOutputWithValues(t *testing.T) {
	tailscaleFunnel := IngressTailscaleFunnel{
		Authkey:    "tskey-auth-example",
		Hostname:   "my-device",
		Port:       8080,
		FunnelPort: 8443,
	}
	out := string(Render("", false, "", "", EnvSection{}, Ingress{TailscaleFunnel: tailscaleFunnel}))

	const want = `
# Makes this device's app reachable from the internet through Tailscale
# Funnel. To change these, edit the values below.
[ingress.tailscale-funnel]
authkey = "tskey-auth-example"
hostname = "my-device"
port = 8080
funnel_port = 8443
`
	if !strings.HasSuffix(out, want) {
		t.Errorf("Render(..., %+v) does not end with the expected tailscale-funnel block:\ngot:\n%s\nwant suffix:\n%s", tailscaleFunnel, out, want)
	}
}

// TestRenderWithTailscaleFunnelRoundTripsThroughParse mirrors
// TestRenderWithIngressRoundTripsThroughParse for tailscale-funnel: a
// Configured() value (all four fields, so nothing is silently lost - see
// coerceIngressTailscaleFunnel) round-trips through Parse with no
// warnings, and the commented example text is absent.
func TestRenderWithTailscaleFunnelRoundTripsThroughParse(t *testing.T) {
	tailscaleFunnel := IngressTailscaleFunnel{
		Authkey:    "tskey-auth-example",
		Hostname:   "my-device",
		Port:       8080,
		FunnelPort: 8443,
	}
	out := Render("my-device", true, "", "", EnvSection{}, Ingress{TailscaleFunnel: tailscaleFunnel})

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if got.Ingress.TailscaleFunnel != tailscaleFunnel {
		t.Errorf("Parse(Render(...)).Ingress.TailscaleFunnel = %+v, want %+v", got.Ingress.TailscaleFunnel, tailscaleFunnel)
	}
	if strings.Contains(string(out), "# [ingress.tailscale-funnel]") {
		t.Errorf("Render() commented out tailscale-funnel despite a Configured() value being set:\n%s", out)
	}
}

// TestRenderBothIngressBlocksTogether guards the ordering half of the
// locked decision: tailscale-funnel's block is appended after
// cloudflared's, and each renders independently of the other's state.
func TestRenderBothIngressBlocksTogether(t *testing.T) {
	cloudflared := IngressCloudflared{Token: "example-tunnel-token", Hostname: "app.example.com", Port: 8080}
	tailscaleFunnel := IngressTailscaleFunnel{Authkey: "tskey-auth-example", Port: 9090}
	out := string(Render("my-device", true, "", "", EnvSection{}, Ingress{Cloudflared: cloudflared, TailscaleFunnel: tailscaleFunnel}))

	cloudflaredIdx := strings.Index(out, "[ingress.cloudflared]")
	tailscaleFunnelIdx := strings.Index(out, "[ingress.tailscale-funnel]")
	if cloudflaredIdx == -1 || tailscaleFunnelIdx == -1 {
		t.Fatalf("Render() is missing one of the ingress blocks:\n%s", out)
	}
	if cloudflaredIdx > tailscaleFunnelIdx {
		t.Errorf("Render() placed [ingress.tailscale-funnel] before [ingress.cloudflared]:\n%s", out)
	}

	got, warnings, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if got.Ingress.Cloudflared != cloudflared || got.Ingress.TailscaleFunnel != tailscaleFunnel {
		t.Errorf("Parse(Render(...)).Ingress = %+v, want {%+v %+v}", got.Ingress, cloudflared, tailscaleFunnel)
	}
}

func TestRenderWithReservedEnvFillsTheExactReservedSize(t *testing.T) {
	const reserve = 4096
	out, span, err := RenderWithReservedEnv("my-device", true, "", "", EnvSection{Values: map[string]string{"API_URL": "https://example.com"}}, Ingress{}, reserve)
	if err != nil {
		t.Fatalf("RenderWithReservedEnv: %v", err)
	}
	if span.LengthBytes != reserve {
		t.Errorf("span.LengthBytes = %d, want the reserved %d", span.LengthBytes, reserve)
	}
	if got := string(out[span.OffsetBytes-len("[env]\n") : span.OffsetBytes]); got != "[env]\n" {
		t.Errorf("the reserved span starts after %q, want it to start immediately after %q", got, "[env]\n")
	}
	if body := out[span.OffsetBytes : span.OffsetBytes+span.LengthBytes]; body[len(body)-1] != '\n' {
		t.Errorf("reserved body's last byte = %q, want a newline so the next section starts on its own line", body[len(body)-1])
	}
}

func TestRenderWithReservedEnvParsesToTheSameSettingsAsAnUnreservedBuild(t *testing.T) {
	env := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}

	plain, _, err := Parse(Render("my-device", true, "net", "pw", EnvSection{Values: env}, Ingress{}))
	if err != nil {
		t.Fatalf("Parse(Render(...)): %v", err)
	}
	reserved, _, err := RenderWithReservedEnv("my-device", true, "net", "pw", EnvSection{Values: env}, Ingress{}, 4096)
	if err != nil {
		t.Fatalf("RenderWithReservedEnv: %v", err)
	}
	got, warnings, err := Parse(reserved)
	if err != nil {
		t.Fatalf("Parse(RenderWithReservedEnv(...)): %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse warnings = %v, want none — padding is comment only", warnings)
	}
	if !reflect.DeepEqual(got, plain) {
		t.Errorf("reserving space changed the settings the card parses to:\n got %+v\nwant %+v", got, plain)
	}
}

func TestReservedEnvRegionAcceptsSameLengthInjectedContent(t *testing.T) {
	const reserve = 2048
	out, span, err := RenderWithReservedEnv("", false, "", "", EnvSection{Values: map[string]string{"API_URL": "https://example.com"}}, Ingress{}, reserve)
	if err != nil {
		t.Fatalf("RenderWithReservedEnv: %v", err)
	}

	injected := []byte("API_TOKEN = \"t0ken\"\nAPI_URL = \"https://injected.example\"\n")
	injected = append(injected, strings.Repeat("\n", reserve-len(injected))...)
	copy(out[span.OffsetBytes:span.OffsetBytes+span.LengthBytes], injected)

	got, _, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse of the patched file: %v", err)
	}
	want := map[string]string{"API_TOKEN": "t0ken", "API_URL": "https://injected.example"}
	if !reflect.DeepEqual(got.Env, want) {
		t.Errorf("patched [env] = %+v, want %+v", got.Env, want)
	}
}

func TestRenderWithReservedEnvWritesALiveHeaderWithNoValues(t *testing.T) {
	out, span, err := RenderWithReservedEnv("", false, "", "", EnvSection{}, Ingress{}, 1024)
	if err != nil {
		t.Fatalf("RenderWithReservedEnv: %v", err)
	}
	if strings.Contains(string(out[:span.OffsetBytes]), "# [env]") {
		t.Error("reserving space rendered the commented-out [env] example; injected keys would land in the root table")
	}

	injected := []byte("ONLY_INJECTED = \"yes\"\n")
	injected = append(injected, strings.Repeat("\n", span.LengthBytes-len(injected))...)
	copy(out[span.OffsetBytes:span.OffsetBytes+span.LengthBytes], injected)

	got, _, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse of the patched file: %v", err)
	}
	if got.Env["ONLY_INJECTED"] != "yes" {
		t.Errorf("injected key landed as %+v, want it under [env]", got.Env)
	}
}

func TestRenderWithReservedEnvRefusesASizeTooSmallForItsValues(t *testing.T) {
	_, _, err := RenderWithReservedEnv("", false, "", "", EnvSection{Values: map[string]string{"API_URL": "https://example.com"}}, Ingress{}, 64)
	if err == nil {
		t.Fatal("RenderWithReservedEnv with 64 bytes reserved = nil error, want a refusal naming the size it needs")
	}
	if !strings.Contains(err.Error(), "64") {
		t.Errorf("error %q doesn't mention the reserved size, so it can't be acted on", err)
	}
}

func TestRenderWithReservedEnvIsDeterministic(t *testing.T) {
	env := EnvSection{Values: map[string]string{"B": "2", "A": "1", "C": "3"}}
	first, _, err := RenderWithReservedEnv("dev", true, "net", "pw", env, Ingress{}, 3000)
	if err != nil {
		t.Fatalf("RenderWithReservedEnv: %v", err)
	}
	for range 5 {
		again, _, err := RenderWithReservedEnv("dev", true, "net", "pw", env, Ingress{}, 3000)
		if err != nil {
			t.Fatalf("RenderWithReservedEnv: %v", err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatal("RenderWithReservedEnv isn't byte-identical across calls; the image identity would stop being reproducible")
		}
	}
}
