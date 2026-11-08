package jetsonflash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/l4t"
)

const kitFile = "../l4t/testdata/kit.tbz2"

// servedKit pins internal/l4t's fixture kit, served from a local server that
// counts downloads.
func servedKit(t *testing.T) (l4t.Kit, *int) {
	t.Helper()
	raw, err := os.ReadFile(kitFile)
	if err != nil {
		t.Fatal(err)
	}
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloads++
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(raw)
	return l4t.Kit{URL: srv.URL, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw)), LicenseURL: "https://nvidia.example/licence"}, &downloads
}

func TestNVIDIAsLicenceIsAskedForOnlyBeforeADownload(t *testing.T) {
	for name, tc := range map[string]struct {
		o         FlashOptions
		typed     string // at the prompt, if there is a terminal
		prompted  bool
		downloads int
		wantErr   []string
	}{
		"accepted at the prompt": {typed: "y\n", prompted: true, downloads: 1},
		"declined at the prompt": {typed: "\n", prompted: true, wantErr: []string{"wasn't accepted"}},
		"no terminal to ask on":  {wantErr: []string{"--accept-nvidia-license", "https://nvidia.example/licence"}},
		"accepted with the flag": {o: FlashOptions{AcceptLicense: true}, downloads: 1},
		"a copy passed in":       {o: FlashOptions{Kit: kitFile}},
	} {
		t.Run(name, func(t *testing.T) {
			kit, downloads := servedKit(t)
			if tc.typed != "" {
				tc.o.Terminal = strings.NewReader(tc.typed)
			}
			var out strings.Builder

			in, err := kitInputs(kit, t.TempDir(), tc.o, &out)

			if prompted := strings.Contains(out.String(), "[y/N]"); prompted != tc.prompted {
				t.Errorf("prompted = %v, want %v", prompted, tc.prompted)
			}
			if *downloads != tc.downloads {
				t.Errorf("downloaded the kit %d times, want %d", *downloads, tc.downloads)
			}
			for _, want := range tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("got %v, want an error mentioning %q", err, want)
				}
			}
			if tc.wantErr == nil && (err != nil || string(in.CBoot) != "cboot.bin") {
				t.Errorf("got %v and CBoot %q, want the kit's members", err, in.CBoot)
			}
		})
	}
}

func TestACachedKitIsNeitherDownloadedNorAskedForAgain(t *testing.T) {
	kit, downloads := servedKit(t)
	cache := t.TempDir()
	if _, err := kitInputs(kit, cache, FlashOptions{AcceptLicense: true}, io.Discard); err != nil {
		t.Fatal(err)
	}

	in, err := kitInputs(kit, cache, FlashOptions{}, io.Discard)

	if err != nil || string(in.CBoot) != "cboot.bin" {
		t.Fatalf("got %v and CBoot %q, want the cached members", err, in.CBoot)
	}
	if *downloads != 1 {
		t.Errorf("downloaded the kit %d times, want once", *downloads)
	}
}
