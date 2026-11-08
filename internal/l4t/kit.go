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

// Kit pins one NVIDIA L4T release: where to fetch it, what it and each member
// gosd uses must hash to, and the licence that governs it.
type Kit struct {
	URL        string
	SHA256     string
	Size       int64
	LicenseURL string
	// Members is each member's sha256, keyed by its path in the kit. It is
	// what a cached copy of the members is checked against.
	Members map[string]string
}

// R32_7_6 is the final T210 release. The licence URL really does say t186:
// it is the agreement NVIDIA's R32.7.6 page links.
var R32_7_6 = Kit{
	URL:        "https://developer.download.nvidia.com/embedded/L4T/r32_Release_v7.6/T210/Jetson-210_Linux_R32.7.6_aarch64.tbz2",
	SHA256:     "865717eed7076babd3c193fc39bc7df41cc457b554059d374c624c42bcde0212",
	Size:       279353831,
	LicenseURL: "https://developer.nvidia.com/downloads/embedded/l4t/r32_release_v7.6/t186/tegra_software_license_agreement-tegra-linux.txt",
	Members: map[string]string{
		"Linux_for_Tegra/bootloader/bmp.blob":                                     "bab643bb8adc0f2f9743c2684c8e0fe426296f03f232cc9aa8a62d635f136462",
		"Linux_for_Tegra/bootloader/eks.img":                                      "5dacafcb2ba84b8f9bd903d5633c1c0b080e8a5097588b4450358ae9d57e374d",
		"Linux_for_Tegra/bootloader/nvtboot_cpu.bin":                              "647cde1923685d5cdcc06d73e9292630249b2d570c9e84cf3e1765b6d8609a74",
		"Linux_for_Tegra/bootloader/nvtboot_recovery.bin":                         "cd6ec5b3c212fdc44d6d36e043448d2b3484749105005b612fcd043869bd53ac",
		"Linux_for_Tegra/bootloader/rp4.blob":                                     "2ada7bf481e032c0878fe64016c35dec3131eaf7c848d6461087dcfff93f4ca1",
		"Linux_for_Tegra/bootloader/t210ref/BCT/P3448_A00_lpddr4_204Mhz_P987.cfg": "2d6ed7a91d351c53e39e011e660e87bdf6e6e33b117b19bcc794e4122108d813",
		"Linux_for_Tegra/bootloader/t210ref/cboot.bin":                            "d6ebd1df25ec860003b2392db19565aed99e28ac2827b6ef3390895407e7b635",
		"Linux_for_Tegra/bootloader/t210ref/cfg/flash_l4t_t210_emmc_p3448.xml":    "25a74fa06fbc9bbb21db5166c2e26d574f6deb1467e7c91703e687ea00a27043",
		"Linux_for_Tegra/bootloader/t210ref/nvtboot.bin":                          "1d993484bf99ae99749a932177966a93e73c8824b39b7bd481b75964e1826277",
		"Linux_for_Tegra/bootloader/t210ref/p3450-0000/u-boot.bin":                "8e988e93a5590acda6327cce282075995ca52a2fe35aeef828ee825f4ab4fcf0",
		"Linux_for_Tegra/bootloader/t210ref/sc7entry-firmware.bin":                "c72ffb2c393d63c3278bd5f7057c29cebd6a065bd5587fc86382087f14839864",
		"Linux_for_Tegra/bootloader/t210ref/warmboot.bin":                         "2abd74d0dea43e80bfa0112a093e1abe9498383d671be58edea49172f81e7479",
		"Linux_for_Tegra/bootloader/tos-mon-only.img":                             "45516b92c4cc7009788f3f9f4fa6cd4a4fde29d27153f7d725198ef4281c6263",
		"Linux_for_Tegra/kernel/dtb/tegra210-p3448-0002-p3449-0000-b00.dtb":       "8ae46c0a56d9f9053f569c33b95f408f323dacc3b328f971f8a3ff2bb791693e",
	},
}

// Inputs are the kit members the boot chain and a first flash are built from.
type Inputs struct {
	Recovery, CBoot, BCTConfig, DTB []byte

	NVTBoot, NVTBootCPU, WarmBoot, SC7Entry, TOS, EKS, BMP, RP4, UBoot []byte
	// Layout is the eMMC partition layout template.
	Layout []byte
}

func (in *Inputs) members() map[string]*[]byte {
	const bl = "Linux_for_Tegra/bootloader/"
	return map[string]*[]byte{
		bl + "nvtboot_recovery.bin":                                         &in.Recovery,
		bl + "t210ref/cboot.bin":                                            &in.CBoot,
		bl + "t210ref/BCT/P3448_A00_lpddr4_204Mhz_P987.cfg":                 &in.BCTConfig,
		"Linux_for_Tegra/kernel/dtb/tegra210-p3448-0002-p3449-0000-b00.dtb": &in.DTB,
		bl + "t210ref/nvtboot.bin":                                          &in.NVTBoot,
		bl + "nvtboot_cpu.bin":                                              &in.NVTBootCPU,
		bl + "t210ref/warmboot.bin":                                         &in.WarmBoot,
		bl + "t210ref/sc7entry-firmware.bin":                                &in.SC7Entry,
		bl + "tos-mon-only.img":                                             &in.TOS,
		bl + "eks.img":                                                      &in.EKS,
		bl + "bmp.blob":                                                     &in.BMP,
		bl + "rp4.blob":                                                     &in.RP4,
		bl + "t210ref/p3450-0000/u-boot.bin":                                &in.UBoot,
		bl + "t210ref/cfg/flash_l4t_t210_emmc_p3448.xml":                    &in.Layout,
	}
}

// maxMemberSize is several times the largest member (u-boot.bin, 650 KiB).
const maxMemberSize = 4 << 20

// Open starts downloading the kit from k.URL. The caller passes the body to
// [Kit.Read], which checks it against the pin, and closes it.
func (k Kit) Open(ctx context.Context, c *http.Client) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request for %s failed: %w", k.URL, err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s failed: %w; check the network and try again", k.URL, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("downloading %s failed with HTTP %d; try again later, or download the kit from NVIDIA's L4T R32.7.6 page and use that copy", k.URL, resp.StatusCode)
	}
	return resp.Body, nil
}

// Read consumes a whole kit .tbz2 stream in one pass, returning its members
// only if the stream matches the pinned hash and size, and then each member
// its own pin. Once every member is in hand the rest is only hashed, since
// decompression is the slow part.
func (k Kit) Read(r io.Reader) (Inputs, error) {
	h := sha256.New()
	raw := &countingReader{r: io.TeeReader(r, h)}

	var in Inputs
	members := in.members()
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
		dst, want := members[hdr.Name]
		if !want || *dst != nil {
			continue
		}
		// Nothing is verified until the stream ends, so don't let it claim
		// more memory than a real member needs.
		if hdr.Size > maxMemberSize {
			return Inputs{}, fmt.Errorf("the kit's %s is %d bytes, far more than R32.7.6's; the download is corrupt or is not R32.7.6, so retry it or download it from NVIDIA", hdr.Name, hdr.Size)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return Inputs{}, fmt.Errorf("reading %s from the kit failed: %w; the download is probably corrupt, so retry it or download it from NVIDIA", hdr.Name, err)
		}
		if data == nil {
			data = []byte{}
		}
		*dst = data
		found++
	}

	if _, err := io.Copy(io.Discard, raw); err != nil {
		return Inputs{}, fmt.Errorf("reading the kit failed: %w; retry the download", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != k.SHA256 || raw.n != k.Size {
		return Inputs{}, fmt.Errorf("the kit is %d bytes with sha256 %s, but L4T R32.7.6 is %d bytes with sha256 %s; the download is corrupt or is not R32.7.6, so retry it or download it from NVIDIA", raw.n, got, k.Size, k.SHA256)
	}
	for name, data := range members {
		if *data == nil {
			return Inputs{}, errors.New("the kit matches its hash but has no " + name + "; this is a bug in gosd, please report it")
		}
		if !k.matches(name, *data) {
			return Inputs{}, errors.New("the kit matches its hash but its " + name + " doesn't match that member's pin; this is a bug in gosd, please report it")
		}
	}
	return in, nil
}

func (k Kit) matches(member string, data []byte) bool {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == k.Members[member]
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
