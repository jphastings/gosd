package fault

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/faultdrop"
	"github.com/jphastings/gosd/internal/secretreg"
)

// subprocessEnv makes the test binary re-enter one test as the child
// process whose real, exported Fatal call is under test: Fatal ends the
// process it is called from, so there is no other way to observe it.
const subprocessEnv = "GOSD_FAULT_TEST_CHILD"

// longSecret stands in for a real credential: long enough to clear
// redact.MinNeedleLength, and deliberately not shaped like any provider's
// key format — a realistic-looking prefix here trips GitHub's push
// protection on the test file itself.
const longSecret = "a-credential-long-enough-to-redact"

func onDevice(t *testing.T) (*reporter, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()
	out := &bytes.Buffer{}
	return &reporter{dir: dir, out: out}, out, dir
}

func offDevice() (*reporter, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &reporter{out: out}, out
}

func TestFatalPrintsTheReportAndExitsWithoutReturning(t *testing.T) {
	if os.Getenv(subprocessEnv) == "1" {
		// A test binary is not an image, whatever build tag it was
		// compiled with, so this asserts the off-device path explicitly
		// rather than depending on how the suite was invoked.
		std.dir = ""
		RegisterSecretString(longSecret, "api-token")
		Fatal(Report{
			Code:    "NO-API-KEY",
			Doing:   "fetching today's forecast",
			Problem: "the weather service rejected " + longSecret,
			Fix:     "add WEATHER_API_KEY to gosd.toml on this card",
			Detail:  errors.New("401 unauthorized"),
		})
		t.Fatal("Fatal returned; it must never return")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestFatalPrintsTheReportAndExitsWithoutReturning$")
	cmd.Env = append(os.Environ(), subprocessEnv+"=1")
	out, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("Fatal exited with %v, want a non-zero exit", err)
	}
	if exit.ExitCode() != faultdrop.ExitCode {
		t.Errorf("Fatal exited %d, want %d", exit.ExitCode(), faultdrop.ExitCode)
	}

	report := string(out)
	for _, want := range []string{
		"error_code: NO-API-KEY",
		"## The problem",
		"the weather service rejected {secret: api-token}",
		"add WEATHER_API_KEY to gosd.toml on this card",
		"401 unauthorized",
		"isn't a GoSD device",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("Fatal printed:\n%s\nwant it to contain %q", report, want)
		}
	}
	if strings.Contains(report, longSecret) {
		t.Error("Fatal printed the registered secret's value")
	}
}

func TestReportsHandedOverAreWaitingForGosdInit(t *testing.T) {
	r, out, dir := onDevice(t)

	r.deliver(Report{Code: "NO-API-KEY", Doing: "fetching the forecast", Problem: "the key was rejected", Detail: errors.New("401 unauthorized")})

	path := filepath.Join(dir, "fault.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no report was left for gosd-init: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the dropped report is mode %v, want 0600: it can carry anything the app's error chain carried", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("the .tmp the report was written through is still there; gosd-init must only ever see the finished file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := faultdrop.Parse(data)
	if !ok {
		t.Fatalf("gosd-init would not trust the dropped report: %s", data)
	}
	if got.Code != "NO-API-KEY" || got.Doing != "fetching the forecast" || got.Problem != "the key was rejected" || got.Detail != "401 unauthorized" {
		t.Errorf("dropped report = %+v, want the app's own fields", got)
	}
	if !strings.Contains(out.String(), "stays down until someone power-cycles it") {
		t.Errorf("console said:\n%s\nwant it to say the device stays down", out)
	}
}

func TestAReportThatCannotBeHandedOverStillReachesTheConsole(t *testing.T) {
	// A file where the directory should be: nothing can be written under
	// it, which is the only way a device's tmpfs realistically fails.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	r := &reporter{dir: blocked, out: out}

	r.deliver(Report{Code: "NO-API-KEY", Problem: "the key was rejected"})

	if !strings.Contains(out.String(), "the key was rejected") {
		t.Errorf("console said:\n%s\nwant the report itself", out)
	}
	if !strings.Contains(out.String(), "handing this report to gosd-init failed") {
		t.Errorf("console said:\n%s\nwant it to admit the card will not carry the report", out)
	}
}

func TestTheReportPrintedOffDeviceIsTheWholeDocument(t *testing.T) {
	r, out := offDevice()

	r.deliver(Report{
		Code:    "SENSOR-UNSUPPORTED",
		Doing:   "reading the greenhouse sensor",
		Problem: "this build has no driver for a BME280",
		Fix:     "set sensor = \"dht22\" in gosd.toml on this card",
		Detail:  errors.New("open /dev/i2c-1: no such device"),
	})

	for _, want := range []string{
		"---\nerror_code: SENSOR-UNSUPPORTED\n",
		"crash report\n",
		"stopped while reading the greenhouse sensor",
		"## The problem\n\nthis build has no driver for a BME280",
		"## The fix\n\nset sensor = \"dht22\" in gosd.toml on this card",
		"## What to send",
		"## Technical detail\n\n    open /dev/i2c-1: no such device",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("printed report:\n%s\nwant it to contain %q", out, want)
		}
	}
}

func TestAReportWithNoDetailSaysSoRatherThanShowingAnEmptySection(t *testing.T) {
	r, out := offDevice()

	r.deliver(Report{Code: "NO-API-KEY", Problem: "the key was rejected"})

	if !strings.Contains(out.String(), "Nothing was captured for this failure.") {
		t.Errorf("printed report:\n%s\nwant it to say nothing was captured", out)
	}
}

func TestSecretsAreRegisteredWithGosdInitAtTheMomentOfTheCall(t *testing.T) {
	r, _, dir := onDevice(t)

	// No Fatal, no crash: the point of writing through immediately is
	// that a panic never gives the app a chance to hand anything over.
	r.register(longSecret, "api-token")

	path := filepath.Join(dir, "secrets.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("nothing was registered for gosd-init to read: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the registration file is mode %v, want 0600: it holds plaintext secrets", info.Mode().Perm())
	}

	data := read(t, path)
	rules := secretreg.Parse(data)
	if len(rules) != 1 || rules[0].Needle != longSecret || rules[0].Replacement != "{secret: api-token}" {
		t.Errorf("gosd-init would apply %+v, want the registered secret under its label", rules)
	}
}

func TestRegisteringTheSameSecretTwiceIsNotAnError(t *testing.T) {
	r, _, dir := onDevice(t)

	r.register(longSecret, "api-token")
	r.register(longSecret, "api-token")
	r.register(longSecret, "a-different-label")
	r.register(longSecret+"-two", "session-token")

	rules := secretreg.Parse(read(t, filepath.Join(dir, "secrets.json")))
	if len(rules) != 2 {
		t.Fatalf("registered %d secrets, want 2: repeats add nothing", len(rules))
	}
	if rules[0].Replacement != "{secret: api-token}" {
		t.Errorf("first rule = %+v, want the label the secret was first registered under", rules[0])
	}
}

func TestARegistrationTooManyLeavesTheOthersWorking(t *testing.T) {
	r, out, dir := onDevice(t)

	for i := 0; i <= secretreg.MaxRegistrations; i++ {
		r.register(longSecret+string(rune('a'+i%26))+strings.Repeat("x", i), "label-"+string(rune('a'+i%26)))
	}

	rules := secretreg.Parse(read(t, filepath.Join(dir, "secrets.json")))
	if len(rules) != secretreg.MaxRegistrations {
		t.Errorf("gosd-init would apply %d rules, want all %d that fit rather than none", len(rules), secretreg.MaxRegistrations)
	}
	if !strings.Contains(out.String(), "was not registered") {
		t.Errorf("console said:\n%s\nwant the refused registration named", out)
	}
}

func TestARegistrationTooLargeToReadBackIsRefusedWhole(t *testing.T) {
	r, out, dir := onDevice(t)
	r.register(longSecret, "api-token")

	r.register(strings.Repeat("s", secretreg.MaxTotalBytes+1), "enormous")

	rules := secretreg.Parse(read(t, filepath.Join(dir, "secrets.json")))
	if len(rules) != 1 || rules[0].Replacement != "{secret: api-token}" {
		t.Errorf("gosd-init would apply %+v, want only the registration that came before the oversized one", rules)
	}
	if !strings.Contains(out.String(), "{secret: enormous} was not registered") {
		t.Errorf("console said:\n%s\nwant the refused registration named", out)
	}
}

func TestAShortSecretIsLeftInTheReportAndSaidSo(t *testing.T) {
	r, out := offDevice()
	r.register("hunter2", "short-password")

	r.deliver(Report{Code: "BAD-LOGIN", Problem: "the device was refused with hunter2"})

	if !strings.Contains(out.String(), "the device was refused with hunter2") {
		t.Errorf("printed report:\n%s\nwant the short secret left as it stands (gosd's redaction floor)", out)
	}
	if !strings.Contains(out.String(), "{secret: short-password} is shorter than gosd redacts") {
		t.Errorf("console said:\n%s\nwant the omission named by label", out)
	}
	if strings.Contains(out.String(), "hunter2\"") {
		t.Error("the warning quoted the secret itself")
	}
}

func TestALabelQuotingItsOwnSecretIsNotUsed(t *testing.T) {
	r, out := offDevice()
	r.register(longSecret, "the key is "+longSecret)

	r.deliver(Report{Code: "NO-API-KEY", Problem: "rejected " + longSecret})

	if strings.Contains(out.String(), longSecret) {
		t.Errorf("printed report:\n%s\nwant no occurrence of the secret: a label that quotes it would publish it", out)
	}
	if !strings.Contains(out.String(), "{secret: "+unnamedSecret+"}") {
		t.Errorf("printed report:\n%s\nwant the fallback label", out)
	}
}

func TestAnEmptySecretRegistersNothing(t *testing.T) {
	r, _, dir := onDevice(t)

	r.register("", "nothing")

	if _, err := os.Stat(filepath.Join(dir, "secrets.json")); !os.IsNotExist(err) {
		t.Error("an empty secret was registered; there is nothing to redact and an empty needle matches everywhere")
	}
}

func TestRegistrationsAreWrittenWhereGosdInitLooksForThem(t *testing.T) {
	// The reporter joins its directory with each file's base name, so the
	// two files have to share the directory their readers expect.
	if got := filepath.Join(faultdrop.Dir, filepath.Base(secretreg.Path)); got != secretreg.Path {
		t.Errorf("registrations would be written to %s, but gosd-init reads %s", got, secretreg.Path)
	}
	if got := filepath.Join(faultdrop.Dir, filepath.Base(faultdrop.Path)); got != faultdrop.Path {
		t.Errorf("reports would be written to %s, but gosd-init reads %s", got, faultdrop.Path)
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
