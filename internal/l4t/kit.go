package l4t

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Kit pins one NVIDIA L4T release: where to fetch it, what it must hash to,
// and the licence that governs it.
type Kit struct {
	URL        string
	SHA256     string
	Size       int64
	LicenseURL string
}

// R32_7_6 is the final T210 release. The licence URL really does say t186:
// it is the agreement NVIDIA's R32.7.6 page links.
var R32_7_6 = Kit{
	URL:        "https://developer.download.nvidia.com/embedded/L4T/r32_Release_v7.6/T210/Jetson-210_Linux_R32.7.6_aarch64.tbz2",
	SHA256:     "865717eed7076babd3c193fc39bc7df41cc457b554059d374c624c42bcde0212",
	Size:       279353831,
	LicenseURL: "https://developer.nvidia.com/downloads/embedded/l4t/r32_release_v7.6/t186/tegra_software_license_agreement-tegra-linux.txt",
}

// Inputs are the four kit members the boot chain is built from.
type Inputs struct {
	Recovery, CBoot, BCTConfig, DTB []byte
}

const (
	memberRecovery  = "Linux_for_Tegra/bootloader/nvtboot_recovery.bin"
	memberCBoot     = "Linux_for_Tegra/bootloader/t210ref/cboot.bin"
	memberBCTConfig = "Linux_for_Tegra/bootloader/t210ref/BCT/P3448_A00_lpddr4_204Mhz_P987.cfg"
	memberDTB       = "Linux_for_Tegra/kernel/dtb/tegra210-p3448-0002-p3449-0000-b00.dtb"
)

// Download fetches the kit from k.URL and returns its members, per [Kit.Read].
func (k Kit) Download(ctx context.Context, c *http.Client) (Inputs, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.URL, nil)
	if err != nil {
		return Inputs{}, fmt.Errorf("building the request for %s failed: %w", k.URL, err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return Inputs{}, fmt.Errorf("downloading %s failed: %w; check the network and try again", k.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Inputs{}, fmt.Errorf("downloading %s failed with HTTP %d; try again later, or download the kit from NVIDIA's L4T R32.7.6 page and use that copy", k.URL, resp.StatusCode)
	}
	return k.Read(resp.Body)
}

// Read consumes a whole kit .tbz2 stream in one pass, returning its members
// only if the stream matches the pinned hash and size. Once all four members
// are in hand the rest is only hashed, since decompression is the slow part.
func (k Kit) Read(r io.Reader) (Inputs, error) {
	h := sha256.New()
	raw := &countingReader{r: io.TeeReader(r, h)}

	members := map[string][]byte{memberRecovery: nil, memberCBoot: nil, memberBCTConfig: nil, memberDTB: nil}
	found := 0
	tr := tar.NewReader(bzip2.NewReader(raw))
	for found < len(members) {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Inputs{}, fmt.Errorf("reading the kit archive failed: %w; the download is probably corrupt, so retry it or download it from NVIDIA", err)
		}
		if _, want := members[hdr.Name]; !want || members[hdr.Name] != nil {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return Inputs{}, fmt.Errorf("reading %s from the kit failed: %w; the download is probably corrupt, so retry it or download it from NVIDIA", hdr.Name, err)
		}
		if data == nil {
			data = []byte{}
		}
		members[hdr.Name] = data
		found++
	}

	if _, err := io.Copy(io.Discard, raw); err != nil {
		return Inputs{}, fmt.Errorf("reading the kit failed: %w; retry the download", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != k.SHA256 || raw.n != k.Size {
		return Inputs{}, fmt.Errorf("the kit is %d bytes with sha256 %s, but L4T R32.7.6 is %d bytes with sha256 %s; the download is corrupt or is not R32.7.6, so retry it or download it from NVIDIA", raw.n, got, k.Size, k.SHA256)
	}
	for name, data := range members {
		if data == nil {
			return Inputs{}, errors.New("the kit matches its hash but has no " + name + "; this is a bug in gosd, please report it")
		}
	}
	return Inputs{
		Recovery:  members[memberRecovery],
		CBoot:     members[memberCBoot],
		BCTConfig: members[memberBCTConfig],
		DTB:       members[memberDTB],
	}, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
