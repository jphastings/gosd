package main

import (
	"strings"
	"testing"
)

func TestSplitExternalFlagBarePathHasNoDest(t *testing.T) {
	path, dest := splitExternalFlag("./mpv")
	if path != "./mpv" || dest != "" {
		t.Errorf("splitExternalFlag(%q) = (%q, %q), want (\"./mpv\", \"\")", "./mpv", path, dest)
	}
}

func TestSplitExternalFlagSplitsOnLastColonWhenSuffixIsAbsolute(t *testing.T) {
	path, dest := splitExternalFlag("./mpv:/bin/mpv")
	if path != "./mpv" || dest != "/bin/mpv" {
		t.Errorf("splitExternalFlag(%q) = (%q, %q), want (\"./mpv\", \"/bin/mpv\")", "./mpv:/bin/mpv", path, dest)
	}
}

func TestSplitExternalFlagDoesNotSplitWindowsDriveLetterPath(t *testing.T) {
	// The only colon in a Windows-style path is followed by a backslash,
	// not a slash, so it must not be treated as a dest separator.
	raw := `C:\tools\mpv.exe`
	path, dest := splitExternalFlag(raw)
	if path != raw || dest != "" {
		t.Errorf("splitExternalFlag(%q) = (%q, %q), want (%q, \"\")", raw, path, dest, raw)
	}
}

func TestSplitExternalFlagUsesLastColonWhenMultiplePresent(t *testing.T) {
	path, dest := splitExternalFlag("./dir:with:colon/mpv:/bin/mpv")
	if path != "./dir:with:colon/mpv" || dest != "/bin/mpv" {
		t.Errorf("splitExternalFlag = (%q, %q), want (\"./dir:with:colon/mpv\", \"/bin/mpv\")", path, dest)
	}
}

func TestParseWithExternalFlagsNil(t *testing.T) {
	got, err := parseWithExternalFlags(nil)
	if err != nil {
		t.Fatalf("parseWithExternalFlags(nil): %v", err)
	}
	if got != nil {
		t.Errorf("parseWithExternalFlags(nil) = %v, want nil", got)
	}
}

func TestParseWithExternalFlagsDefaultsDestToBinBasename(t *testing.T) {
	got, err := parseWithExternalFlags([]string{"./build/mpv"})
	if err != nil {
		t.Fatalf("parseWithExternalFlags: %v", err)
	}
	if len(got) != 1 || got[0].Path != "./build/mpv" || got[0].Dest != "/bin/mpv" {
		t.Errorf("parseWithExternalFlags([./build/mpv]) = %+v, want a single spec with dest /bin/mpv", got)
	}
}

func TestParseWithExternalFlagsExplicitDest(t *testing.T) {
	got, err := parseWithExternalFlags([]string{"./build/mpv:/usr/local/bin/mpv"})
	if err != nil {
		t.Fatalf("parseWithExternalFlags: %v", err)
	}
	if len(got) != 1 || got[0].Path != "./build/mpv" || got[0].Dest != "/usr/local/bin/mpv" {
		t.Errorf("parseWithExternalFlags(explicit dest) = %+v, want dest /usr/local/bin/mpv", got)
	}
}

func TestParseWithExternalFlagsRejectsDuplicateDest(t *testing.T) {
	_, err := parseWithExternalFlags([]string{"./mpv:/bin/thing", "./other:/bin/thing"})
	if err == nil {
		t.Fatal("parseWithExternalFlags with a duplicate dest succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "/bin/thing") {
		t.Errorf("error = %q, want it to mention the colliding dest /bin/thing", err.Error())
	}
}

func TestParseWithExternalFlagsRejectsReservedDests(t *testing.T) {
	for _, dest := range []string{"/init", "/app", "/etc/gosd/config.json", "/lib/firmware/brcm/x.bin"} {
		_, err := parseWithExternalFlags([]string{"./mpv:" + dest})
		if err == nil {
			t.Errorf("parseWithExternalFlags([./mpv:%s]) succeeded, want an error", dest)
			continue
		}
		if !strings.Contains(err.Error(), dest) {
			t.Errorf("error for dest %q = %q, want it to mention %q", dest, err.Error(), dest)
		}
	}
}

func TestValidateExternalDestRejectsNonAbsolute(t *testing.T) {
	for _, dest := range []string{"bin/mpv", "", "relative/path"} {
		if err := validateExternalDest(dest); err == nil {
			t.Errorf("validateExternalDest(%q) succeeded, want an error (dest must be absolute)", dest)
		}
	}
}

func TestValidateExternalDestAcceptsOrdinaryAbsolutePath(t *testing.T) {
	if err := validateExternalDest("/bin/mpv"); err != nil {
		t.Errorf("validateExternalDest(/bin/mpv) = %v, want nil", err)
	}
}
