package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildAppliesLDFlags is the end-to-end acceptance test for gosd-wjjn's
// --ldflags: a real `gosd build --ldflags="-X main.version=stamped"` against
// testdata/versionedfixture (a fixture app exporting a `version` var
// specifically to be targeted this way) must stamp the value into the
// binary gosd-init boots as /app.
func TestBuildAppliesLDFlags(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "versionedfixture-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "./testdata/versionedfixture",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--ldflags", "-X main.version=stamped",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	app := readAppBinary(t, imgPath)
	if !bytes.Contains(app, []byte("stamped")) {
		t.Error("/app does not contain the -ldflags-stamped string \"stamped\"; --ldflags did not reach the app compile")
	}
}

// TestBuildTagsMergesWithMandatoryBoardTags is the end-to-end acceptance
// test for gosd-wjjn's --tags: a real `gosd build --tags extratagmarker`
// must merge that tag onto - not replace - the board's own mandatory
// gosd/gosd_<board> tags, so testdata/boardtagfixture's per-board marker
// (proving the mandatory tags survived) and its extratagmarker-gated marker
// (proving --tags reached the compile) both land in the built /app.
func TestBuildTagsMergesWithMandatoryBoardTags(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "boardtagfixture-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "./testdata/boardtagfixture",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--tags", "extratagmarker",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	app := readAppBinary(t, imgPath)
	for _, want := range []string{"boardtagfixture-marker:pi-zero-2w", "boardtagfixture-marker:extratagmarker-set"} {
		if !bytes.Contains(app, []byte(want)) {
			t.Errorf("/app does not contain %q; --tags must merge onto, not replace, the board's mandatory tags", want)
		}
	}
}

// TestBuildRejectsAGosdNamespacedTagsValue mirrors
// TestBuildRefusesADataSizeFAT32CannotHold's shape: a --tags value in
// gosd's own reserved namespace is refused before any image bytes exist.
func TestBuildRejectsAGosdNamespacedTagsValue(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--tags", "gosd_pi_zero_2w",
		"-o", imgPath,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --tags=gosd_pi_zero_2w succeeded, want a refusal")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("gosd_pi_zero_2w")) {
		t.Errorf("refusal %q does not name the rejected tag", err)
	}
	if _, statErr := os.Stat(imgPath); !os.IsNotExist(statErr) {
		t.Errorf("gosd build wrote %s despite refusing --tags; the refusal must come first", imgPath)
	}
}
