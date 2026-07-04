package gosdtoml

import (
	"strings"
	"testing"
)

func TestRenderWithValuesRoundTripsThroughParse(t *testing.T) {
	out := Render("my-device", "home-network", "hunter2")

	got, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	want := Config{Hostname: "my-device", Wifi: Wifi{SSID: "home-network", Passphrase: "hunter2"}}
	if got != want {
		t.Errorf("Parse(Render(...)) = %+v, want %+v", got, want)
	}

	if strings.Contains(string(out), "# hostname") {
		t.Errorf("Render() commented out hostname despite a value being set:\n%s", out)
	}
	if strings.Contains(string(out), "# ssid") {
		t.Errorf("Render() commented out wifi despite a value being set:\n%s", out)
	}
}

func TestRenderWithoutValuesProducesCommentedExamplesThatParseAsEmpty(t *testing.T) {
	out := Render("", "", "")

	got, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(Render(...)) error: %v", err)
	}
	if got != (Config{}) {
		t.Errorf("Parse(Render(\"\", \"\", \"\")) = %+v, want zero Config (all commented out)", got)
	}

	for _, want := range []string{`# hostname = "my-device"`, `# ssid = "MyHomeNetwork"`, `# passphrase = "MyWiFiPassword"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("Render(\"\", \"\", \"\") missing example line %q:\n%s", want, out)
		}
	}
}

func TestRenderIncludesPlainLanguageHeader(t *testing.T) {
	out := string(Render("", "", ""))

	for _, want := range []string{"text editor", "Notepad", "restart it"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() header missing expected plain-language phrase %q:\n%s", want, out)
		}
	}
}
