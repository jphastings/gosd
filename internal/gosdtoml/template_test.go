package gosdtoml

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderWithValuesRoundTripsThroughParse(t *testing.T) {
	env := map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}
	out := Render("my-device", "home-network", "hunter2", env)

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

	if strings.Contains(string(out), "# hostname") {
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
	out := Render("", "", "", nil)

	got, warnings, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if warnings != nil {
		t.Errorf("Parse(Render(...)) warnings = %v, want none", warnings)
	}
	if !reflect.DeepEqual(got, Config{}) {
		t.Errorf(`Parse(Render("", "", "", nil)) = %+v, want zero Config (all commented out)`, got)
	}

	for _, want := range []string{`# hostname = "my-device"`, `# ssid = "MyHomeNetwork"`, `# passphrase = "MyWiFiPassword"`, `# NAME = "value"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf(`Render("", "", "", nil) missing example line %q:`+"\n%s", want, out)
		}
	}
}

func TestRenderIncludesPlainLanguageHeader(t *testing.T) {
	out := string(Render("", "", "", nil))

	for _, want := range []string{"text editor", "Notepad", "restart it"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() header missing expected plain-language phrase %q:\n%s", want, out)
		}
	}
}

func TestRenderEnvExactOutputWithoutBakedValues(t *testing.T) {
	out := string(Render("", "", "", nil))

	const want = `
# Extra settings your app reads when it starts, sometimes called
# "environment variables" — most apps don't need any. To add one, remove
# the "#" from the two lines below and change NAME and "value"; add more
# lines the same way for further settings. Names are case-sensitive, and
# values always need double quotes.
# [env]
# NAME = "value"
`
	if !strings.HasSuffix(out, want) {
		t.Errorf("Render(\"\", \"\", \"\", nil) does not end with the expected [env] section:\ngot:\n%s\nwant suffix:\n%s", out, want)
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
		out := string(Render("", "", "", env))
		if !strings.HasSuffix(out, want) {
			t.Fatalf("Render() [env] section not sorted/deterministic on iteration %d:\ngot:\n%s\nwant suffix:\n%s", i, out, want)
		}
	}
}
