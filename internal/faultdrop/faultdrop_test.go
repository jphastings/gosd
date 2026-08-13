package faultdrop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/faultreport"
)

func TestADroppedReportSurvivesTheHandoffUnchanged(t *testing.T) {
	want := faultreport.Report{
		Code:    "NO-API-KEY",
		Doing:   "fetching today's forecast",
		Problem: "the weather service rejected our API key",
		Fix:     "add WEATHER_API_KEY to config/env/ on this card",
		Detail:  "get \"https://api.example\": 401 unauthorized",
	}

	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := Parse(data)
	if !ok {
		t.Fatalf("Parse rejected what Marshal wrote: %s", data)
	}
	if got != want {
		t.Errorf("Parse(Marshal(r)) = %+v, want %+v", got, want)
	}
}

func TestNothingUntrustworthyIsRead(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"truncated JSON":   `{"code": "X"`,
		"not JSON at all":  "definitely not json",
		"the wrong shape":  `["code", "X"]`,
		"nothing to say":   `{}`,
		"blank everything": `{"code": "", "detail": ""}`,
		"oversized":        `{"detail": "` + strings.Repeat("x", MaxBytes) + `"}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := Parse([]byte(data)); ok {
				t.Errorf("Parse trusted %s", name)
			}
		})
	}
}

func TestAnEnormousDetailIsTrimmedRatherThanDroppingTheReport(t *testing.T) {
	data, err := Marshal(faultreport.Report{Code: "OOM", Detail: strings.Repeat("stack frame\n", 40_000)})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxBytes {
		t.Fatalf("Marshal wrote %d bytes, more than the %d Parse will read back", len(data), MaxBytes)
	}

	got, ok := Parse(data)
	if !ok {
		t.Fatal("a trimmed report was not readable")
	}
	if !strings.HasPrefix(got.Detail, "stack frame\n") {
		t.Error("the detail's head — where an error chain says what failed — was not kept")
	}
	if n := strings.Count(got.Detail, "truncated by gosd"); n != 1 {
		t.Errorf("the detail carries %d truncation markers, want exactly 1", n)
	}
}

func TestHumanFieldsCannotCrowdOutTheReport(t *testing.T) {
	data, err := Marshal(faultreport.Report{Problem: strings.Repeat("p", 10*MaxBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxBytes {
		t.Fatalf("Marshal wrote %d bytes, more than the %d Parse will read back", len(data), MaxBytes)
	}
	if _, ok := Parse(data); !ok {
		t.Error("a report with an absurd Problem was not readable at all")
	}
}

func TestTakeDeliversAReportExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fault.json")
	write(t, path, `{"code": "NO-API-KEY"}`)
	write(t, path+".tmp", `{"code": "HALF-WRITTEN"`)

	got, ok := Take(path)
	if !ok || got.Code != "NO-API-KEY" {
		t.Fatalf("Take() = %+v, %v, want the dropped report", got, ok)
	}

	if _, ok := Take(path); ok {
		t.Error("the same report was delivered twice; it would be reported again on the next exit")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("a .tmp from an app that died mid-write was left behind")
	}
}

func TestAnEnormousDropIsDeletedWithoutBeingRead(t *testing.T) {
	// Marshal keeps a report small, but nothing stops an app writing this
	// path itself, and the reader is PID 1 on a board with as little as
	// 512MB.
	dir := t.TempDir()
	path := filepath.Join(dir, "fault.json")
	write(t, path, `{"detail": "`+strings.Repeat("x", MaxBytes)+`"}`)

	if _, ok := Take(path); ok {
		t.Fatal("Take trusted an oversized drop")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the oversized drop is still there; it would be re-statted on every exit from now on")
	}
}

func TestAnUnreadableDropIsRemovedRatherThanRetriedForever(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fault.json")
	write(t, path, "not json")

	if _, ok := Take(path); ok {
		t.Fatal("Take trusted a file that doesn't parse")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the unparseable drop is still there; it would be re-read on every exit from now on")
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
