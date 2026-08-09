package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "env.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp env-file: %v", err)
	}
	return path
}

func TestParseEnvFileReturnsVerbatimBodyAndActiveDefaults(t *testing.T) {
	content := `# uncomment this if you want the demo to run
# RUN_DEMO = true

# Where telemetry is posted; leave blank to disable
API_URL = "https://example.com"
`
	verbatim, active, warnings, err := parseEnvFile(writeEnvFile(t, content))
	if err != nil {
		t.Fatalf("parseEnvFile: %v", err)
	}
	if verbatim != content {
		t.Errorf("verbatim body = %q, want the file content unchanged %q", verbatim, content)
	}
	if want := (map[string]string{"API_URL": "https://example.com"}); !reflect.DeepEqual(active, want) {
		t.Errorf("active = %+v, want %+v (only the uncommented entry)", active, want)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

func TestParseEnvFileEmptyPathIsNoFile(t *testing.T) {
	verbatim, active, warnings, err := parseEnvFile("")
	if err != nil || verbatim != "" || active != nil || warnings != nil {
		t.Errorf(`parseEnvFile("") = %q, %v, %v, %v; want all zero`, verbatim, active, warnings, err)
	}
}

func TestParseEnvFileWarnsOnBareActiveScalar(t *testing.T) {
	// An active (uncommented) bare scalar is coerced to a string, with a
	// warning the build surfaces — not an error, matching the on-card parser.
	_, active, warnings, err := parseEnvFile(writeEnvFile(t, "PORT = 8080\n"))
	if err != nil {
		t.Fatalf("parseEnvFile: %v", err)
	}
	if active["PORT"] != "8080" {
		t.Errorf(`active["PORT"] = %q, want "8080"`, active["PORT"])
	}
	if len(warnings) == 0 {
		t.Error("want a coercion warning for the bare scalar PORT")
	}
}

func TestParseEnvFileRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"its own [env] header": "[env]\nA = \"1\"\n",
		"a stray [wifi]":       "[wifi]\nssid = \"x\"\n",
		"an array of tables":   "[[thing]]\nx = 1\n",
		"an invalid key":       "1BAD = \"x\"\n",
		"a reserved GOSD_ key": "GOSD_FOO = \"x\"\n",
		"malformed toml":       "A = \n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseEnvFile(writeEnvFile(t, content)); err == nil {
				t.Errorf("parseEnvFile(%q) succeeded, want an error", content)
			}
		})
	}
}

func TestParseEnvFileMissingFileErrors(t *testing.T) {
	if _, _, _, err := parseEnvFile(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Error("parseEnvFile on a missing file should error")
	}
}
