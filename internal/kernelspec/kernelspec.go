// Package kernelspec is the Go-native, declarative source of truth for how
// each board's kernel is built: source repo/ref, defconfig, config
// fragment, device-tree patches, toolchain, build outputs, and the
// post-olddefconfig assertions the build must satisfy.
//
// It originally replaced values that lived scattered across
// build/boards/<board>/**/build.sh and docker-build.sh (bean gosd-di6v, epic
// gosd-47rm); those per-board shell scripts built kernels directly until
// internal/kernelbuild's Go builder started reading KernelSpec instead and
// bean gosd-07fl deleted them. This package is now the only source of truth
// for kernel build inputs — TestPiRequiredYIsDerivedFromFragment still
// guards the Pi boards' RequiredY against their kernel.fragment, the one
// remaining on-disk file with independent content; the Rockchip-family
// boards' RequiredY/ForbiddenY here have no second copy to drift against
// anymore (they used to be compared against docker-build.sh's required_y/
// forbidden_y arrays via TestRockchipRequiredYMatchesScript, removed along
// with those scripts).
package kernelspec

import (
	"embed"
	"fmt"
	"path"
	"regexp"
	"sort"

	nanopikernel "github.com/jphastings/gosd/build/boards/nanopi-zero2/kernel"
	pizero2wmanifest "github.com/jphastings/gosd/build/boards/pi-zero-2w"
	pizerowmanifest "github.com/jphastings/gosd/build/boards/pi-zero-w"
	qemuvirtkernel "github.com/jphastings/gosd/build/boards/qemu-virt/kernel"
	radxakernel "github.com/jphastings/gosd/build/boards/radxa-zero-3e/kernel"
	rock4sekernel "github.com/jphastings/gosd/build/boards/rock-4se/kernel"
)

// RefKind distinguishes how Source.Ref must be resolved.
type RefKind int

const (
	// CommitRef means Ref is a full commit SHA, fetched with
	// `git fetch --depth 1 origin <ref>`.
	CommitRef RefKind = iota
	// TagRef means Ref is a tag name, fetched with
	// `git clone --depth 1 --branch <ref>`.
	TagRef
)

// String returns a short label for logs/errors, not a Go-syntax name.
func (k RefKind) String() string {
	if k == TagRef {
		return "tag"
	}
	return "commit"
}

// Source pins the upstream kernel tree a board is built from.
type Source struct {
	Repo    string
	Ref     string
	RefKind RefKind
	// CommitDate is the pinned ref's commit timestamp (RFC3339), used to
	// seed Reproducibility.KBUILDBuildTimestamp. Empty when the board's
	// build script doesn't currently set that pin (see Reproducibility).
	CommitDate string
}

// Toolchain is the ARCH/CROSS_COMPILE pair passed to every `make` invocation.
type Toolchain struct {
	// KernelArch is the kernel's own ARCH= value, e.g. "arm64" or "arm" -
	// not a Go GOARCH (see internal/boards.Arch for that).
	KernelArch string
	// CrossCompile is the CROSS_COMPILE= prefix, e.g. "aarch64-linux-gnu-".
	CrossCompile string
}

// DTB describes how a board's device tree blob is built and named. Boards
// with no DTB to build (qemu-virt: qemu's -M virt machine supplies its own)
// leave KernelSpec.DTB nil.
type DTB struct {
	// MakeTarget is the `make` target that produces this DTB: either
	// "dtbs" (build every DTB in the tree - the Pi boards' kernel build
	// does this, then picks SourcePath out of the result) or a single relative
	// target such as "rockchip/rk3566-radxa-zero-3e.dtb" (the
	// Rockchip-family boards, whose ARCH=arm64 dtb targets resolve
	// relative to arch/arm64/boot/dts/).
	MakeTarget string
	// SourcePath is where the built DTB lands inside the kernel source
	// tree, relative to its root.
	SourcePath string
	// Filename is the name the DTB is copied to for GoSD's artifact
	// pipeline. Where the board's internal/boards.Board.Artifacts()
	// already tracks a DTB artifact, this MUST equal that ArtifactRef.Name
	// (see TestKernelSpecOutputsMatchBoardArtifacts) - pi-zero-2w is a
	// documented exception; see that test.
	Filename string
}

// Patch is one device-tree patch, applied with `patch -p1` in slice order.
type Patch struct {
	Name    string
	Content []byte
}

// Reproducibility holds the KBUILD_BUILD_* environment pins that make a
// kernel build byte-identical across runs (tied to the pinned source rather
// than wall-clock time or the build host's identity).
type Reproducibility struct {
	// Empty fields mean this board's build doesn't currently set this pin.
	// As of gosd-di6v, none of the Rockchip-family boards (radxa-zero-3e,
	// nanopi-zero2, qemu-virt) set these - only the two Pi boards do. This
	// is a real gap for gosd-47rm's byte-identity CI gate, left open
	// rather than fixed here (that bean's design doc only made the
	// original shell scripts' behavior declarative, not better).
	KBUILDBuildTimestamp string
	KBUILDBuildUser      string
	KBUILDBuildHost      string
}

// KernelSpec is the complete declarative description of how one board's
// kernel is built.
type KernelSpec struct {
	BoardID   string
	Source    Source
	Defconfig string
	Toolchain Toolchain

	// ConfigFragment is the GoSD-authored Kconfig fragment merged onto
	// Defconfig via scripts/kconfig/merge_config.sh.
	ConfigFragment []byte

	// DTSPatches are device-tree patches applied, in order, before the
	// config step. Empty for the two Pi boards (they use the stock
	// upstream DTS unmodified); populated for the three Rockchip-family
	// boards.
	DTSPatches []Patch

	// DTB is nil for boards with no device tree blob to build.
	DTB *DTB

	// KernelMakeTarget is the `make` target that builds the kernel image,
	// e.g. "Image" or "zImage".
	KernelMakeTarget string
	// KernelSourcePath is where the built kernel image lands inside the
	// kernel source tree, relative to its root.
	KernelSourcePath string
	// KernelFilename is the name the kernel image is copied to. It MUST
	// equal the board's internal/boards.Board.Artifacts() kernel
	// ArtifactRef.Name (see TestKernelSpecOutputsMatchBoardArtifacts).
	KernelFilename string

	// RequiredY lists every CONFIG_*=y option that must survive `make
	// olddefconfig`. For the Pi boards this is mechanically derived from
	// ConfigFragment (every literal CONFIG_*=y line in it - see
	// requiredYFromFragment); for the Rockchip-family boards it's a
	// hand-maintained literal list (originally copied from each board's
	// now-deleted docker-build.sh required_y array - see bean gosd-07fl).
	RequiredY []string

	// ForbiddenY lists CONFIG_*=y/=m options that must NOT survive `make
	// olddefconfig`. Only qemu-virt sets this: it asserts real-hardware
	// drivers stayed cut, catching drift toward a board-specific kernel.
	ForbiddenY []string

	// ModulesDisabled is true for every current board: CONFIG_MODULES
	// must be unset (no loadable modules; every kernel stays monolithic).
	ModulesDisabled bool

	Reproducibility Reproducibility
}

// requiredYFromFragment extracts every literal "CONFIG_FOO=y" line from a
// Kconfig fragment, in file order. This mirrors what the Pi boards' kernel
// build actually does: apply the fragment as-is and rely on
// merge_config.sh + olddefconfig to keep every explicit "=y" line set.
var configYLine = regexp.MustCompile(`^CONFIG_[A-Z0-9_]+=y$`)

func requiredYFromFragment(fragment []byte) []string {
	var required []string
	for _, line := range splitLines(fragment) {
		if configYLine.Match(line) {
			required = append(required, string(line))
		}
	}
	return required
}

func splitLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			lines = append(lines, trimCR(b[start:i]))
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, trimCR(b[start:]))
	}
	return lines
}

func trimCR(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\r' {
		return b[:n-1]
	}
	return b
}

// loadPatches reads every file directly inside dir in fsys, sorted by name,
// as a Patch. Sorting explicitly (rather than relying on fs.ReadDir's
// documented-sorted order) keeps patch application order correct even if
// that implementation detail ever changes.
func loadPatches(fsys embed.FS, dir string) []Patch {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		panic(fmt.Sprintf("kernelspec: reading embedded patches dir %q: %v", dir, err))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	patches := make([]Patch, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fsys.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			panic(fmt.Sprintf("kernelspec: reading embedded patch %q: %v", e.Name(), err))
		}
		patches = append(patches, Patch{Name: e.Name(), Content: data})
	}
	return patches
}

// piZeroCommitRef and piZeroCommitDate are pinned identically for both Pi
// boards, for fleet consistency across the two Broadcom boards - see
// build/boards/pi-zero-2w/README.md for why this particular commit on
// rpi-6.18.y was chosen.
const (
	piZeroCommitRef  = "63598c83153e19b1f99067ab6df7409de2c111f8"
	piZeroCommitDate = "2026-07-01T10:23:21Z"
	piZeroRepo       = "https://github.com/raspberrypi/linux.git"
)

// fleetKernelTag and fleetKernelRepo pin the same mainline stable LTS tag
// across every Rockchip-family board (radxa-zero-3e, nanopi-zero2,
// qemu-virt) - see build/boards/radxa-zero-3e/kernel/README.md.
const (
	fleetKernelTag  = "v6.18.37"
	fleetKernelRepo = "https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git"
)

var specs = map[string]KernelSpec{
	"pi-zero-2w": {
		BoardID: "pi-zero-2w",
		Source: Source{
			Repo:       piZeroRepo,
			Ref:        piZeroCommitRef,
			RefKind:    CommitRef,
			CommitDate: piZeroCommitDate,
		},
		Defconfig: "bcm2711_defconfig",
		Toolchain: Toolchain{KernelArch: "arm64", CrossCompile: "aarch64-linux-gnu-"},

		ConfigFragment: pizero2wmanifest.KernelFragment,

		// pi-zero-2w's kernel build builds every DTB ("make ... Image dtbs")
		// and copies out bcm2710-rpi-zero-2-w.dtb, but
		// internal/boards/pizero2w.Artifacts() does not currently list a
		// DTB artifact - unlike pi-zero-w, this board's BootFiles never
		// asks for one. This predates gosd-di6v; recorded as a discovered
		// gap in the bean rather than silently worked around. DTB is kept
		// non-nil here because it's a true, faithful description of what
		// the build produces; TestKernelSpecOutputsMatchBoardArtifacts
		// documents the pi-zero-2w DTB exemption explicitly.
		DTB: &DTB{
			MakeTarget: "dtbs",
			SourcePath: "arch/arm64/boot/dts/broadcom/bcm2710-rpi-zero-2-w.dtb",
			Filename:   "bcm2710-rpi-zero-2-w.dtb",
		},

		KernelMakeTarget: "Image",
		KernelSourcePath: "arch/arm64/boot/Image",
		KernelFilename:   "kernel8.img",

		RequiredY:       requiredYFromFragment(pizero2wmanifest.KernelFragment),
		ModulesDisabled: true,

		Reproducibility: Reproducibility{
			KBUILDBuildTimestamp: piZeroCommitDate,
			KBUILDBuildUser:      "gosd",
			KBUILDBuildHost:      "gosd-ci",
		},
	},

	"pi-zero-w": {
		BoardID: "pi-zero-w",
		Source: Source{
			Repo:       piZeroRepo,
			Ref:        piZeroCommitRef,
			RefKind:    CommitRef,
			CommitDate: piZeroCommitDate,
		},
		Defconfig: "bcmrpi_defconfig",
		Toolchain: Toolchain{KernelArch: "arm", CrossCompile: "arm-linux-gnueabihf-"},

		ConfigFragment: pizerowmanifest.KernelFragment,

		DTB: &DTB{
			MakeTarget: "dtbs",
			SourcePath: "arch/arm/boot/dts/broadcom/bcm2835-rpi-zero-w.dtb",
			Filename:   "bcm2835-rpi-zero-w.dtb",
		},

		KernelMakeTarget: "zImage",
		KernelSourcePath: "arch/arm/boot/zImage",
		KernelFilename:   "kernel.img",

		RequiredY:       requiredYFromFragment(pizerowmanifest.KernelFragment),
		ModulesDisabled: true,

		Reproducibility: Reproducibility{
			KBUILDBuildTimestamp: piZeroCommitDate,
			KBUILDBuildUser:      "gosd",
			KBUILDBuildHost:      "gosd-ci",
		},
	},

	"radxa-zero-3e": {
		BoardID: "radxa-zero-3e",
		Source: Source{
			Repo:    fleetKernelRepo,
			Ref:     fleetKernelTag,
			RefKind: TagRef,
		},
		Defconfig: "defconfig",
		Toolchain: Toolchain{KernelArch: "arm64", CrossCompile: "aarch64-linux-gnu-"},

		ConfigFragment: radxakernel.ConfigFragment,
		DTSPatches:     loadPatches(radxakernel.PatchesFS, "patches"),

		DTB: &DTB{
			MakeTarget: "rockchip/rk3566-radxa-zero-3e.dtb",
			SourcePath: "arch/arm64/boot/dts/rockchip/rk3566-radxa-zero-3e.dtb",
			Filename:   "rk3566-radxa-zero-3e.dtb",
		},

		KernelMakeTarget: "Image",
		KernelSourcePath: "arch/arm64/boot/Image",
		KernelFilename:   "Image",

		// Asserts these survived olddefconfig, mirroring the "Asserting
		// required options survived olddefconfig" step the board's
		// now-deleted docker-build.sh used to run (bean gosd-07fl) - this
		// hand-maintained list is now the only copy.
		RequiredY: []string{
			"CONFIG_ARCH_ROCKCHIP",
			"CONFIG_MMC_DW",
			"CONFIG_MMC_DW_ROCKCHIP",
			"CONFIG_MMC_SDHCI_OF_DWCMSHC",
			"CONFIG_STMMAC_ETH",
			"CONFIG_DWMAC_ROCKCHIP",
			"CONFIG_REALTEK_PHY",
			"CONFIG_MOTORCOMM_PHY",
			"CONFIG_USB_DWC3",
			"CONFIG_PHY_ROCKCHIP_INNO_USB2",
			"CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY",
			"CONFIG_GPIO_ROCKCHIP",
			"CONFIG_I2C_RK3X",
			"CONFIG_SPI_ROCKCHIP",
			"CONFIG_SPI_SPIDEV",
			"CONFIG_SERIAL_8250_DW",
		},
		ModulesDisabled: true,
		// Reproducibility left zero: this board's build doesn't set any of
		// the KBUILD_BUILD_* pins today - see Reproducibility's doc comment.
	},

	// "rock-4se" is scaffolding-only as of bean gosd-iosp: this board isn't
	// registered in internal/boards yet (bean gosd-0vvh), so
	// TestBoardIDsListsExactlyTheFiveKernelBuildingBoards in
	// kernelspec_test.go fails until that board profile lands and the test
	// is updated to include it - a known, reported cross-bean coupling, not
	// silently worked around here. See the bean body's "Scaffolding status"
	// note.
	"rock-4se": {
		BoardID: "rock-4se",
		Source: Source{
			Repo:    fleetKernelRepo,
			Ref:     fleetKernelTag,
			RefKind: TagRef,
		},
		Defconfig: "defconfig",
		Toolchain: Toolchain{KernelArch: "arm64", CrossCompile: "aarch64-linux-gnu-"},

		ConfigFragment: rock4sekernel.ConfigFragment,
		DTSPatches:     loadPatches(rock4sekernel.PatchesFS, "patches"),

		DTB: &DTB{
			MakeTarget: "rockchip/rk3399-rock-4se.dtb",
			SourcePath: "arch/arm64/boot/dts/rockchip/rk3399-rock-4se.dtb",
			Filename:   "rk3399-rock-4se.dtb",
		},

		KernelMakeTarget: "Image",
		KernelSourcePath: "arch/arm64/boot/Image",
		KernelFilename:   "Image",

		// See the radxa-zero-3e RequiredY comment above - same origin, now
		// a hand-maintained literal list. RK3399-T-specific entries
		// (PHY_ROCKCHIP_TYPEC instead of radxa-zero-3e's RK3566-only
		// PHY_ROCKCHIP_NANENG_COMBO_PHY; PCI/NVMe/exFAT/mass-storage) come
		// from bean gosd-je2r's research findings.
		RequiredY: []string{
			"CONFIG_ARCH_ROCKCHIP",
			"CONFIG_MMC_DW",
			"CONFIG_MMC_DW_ROCKCHIP",
			"CONFIG_PCI",
			"CONFIG_PCIE_ROCKCHIP_HOST",
			"CONFIG_PHY_ROCKCHIP_PCIE",
			"CONFIG_BLK_DEV_NVME",
			"CONFIG_EXFAT_FS",
			"CONFIG_STMMAC_ETH",
			"CONFIG_DWMAC_ROCKCHIP",
			"CONFIG_REALTEK_PHY",
			"CONFIG_USB_DWC3",
			"CONFIG_PHY_ROCKCHIP_INNO_USB2",
			"CONFIG_PHY_ROCKCHIP_TYPEC",
			"CONFIG_USB_CONFIGFS_MASS_STORAGE",
			"CONFIG_GPIO_ROCKCHIP",
			"CONFIG_I2C_RK3X",
			"CONFIG_SPI_ROCKCHIP",
			"CONFIG_SPI_SPIDEV",
			"CONFIG_SERIAL_8250_DW",
		},
		ModulesDisabled: true,
		// Reproducibility left zero: this board's build doesn't set any of
		// the KBUILD_BUILD_* pins today - see Reproducibility's doc comment.
	},

	"nanopi-zero2": {
		BoardID: "nanopi-zero2",
		Source: Source{
			Repo:    fleetKernelRepo,
			Ref:     fleetKernelTag,
			RefKind: TagRef,
		},
		Defconfig: "defconfig",
		Toolchain: Toolchain{KernelArch: "arm64", CrossCompile: "aarch64-linux-gnu-"},

		ConfigFragment: nanopikernel.ConfigFragment,
		DTSPatches:     loadPatches(nanopikernel.PatchesFS, "patches"),

		DTB: &DTB{
			MakeTarget: "rockchip/rk3528-nanopi-zero2.dtb",
			SourcePath: "arch/arm64/boot/dts/rockchip/rk3528-nanopi-zero2.dtb",
			Filename:   "rk3528-nanopi-zero2.dtb",
		},

		KernelMakeTarget: "Image",
		KernelSourcePath: "arch/arm64/boot/Image",
		KernelFilename:   "Image",

		// See the radxa-zero-3e RequiredY comment above - same origin,
		// now a hand-maintained literal list.
		RequiredY: []string{
			"CONFIG_ARCH_ROCKCHIP",
			"CONFIG_MMC_DW",
			"CONFIG_MMC_DW_ROCKCHIP",
			"CONFIG_MMC_SDHCI_OF_DWCMSHC",
			"CONFIG_STMMAC_ETH",
			"CONFIG_DWMAC_ROCKCHIP",
			"CONFIG_REALTEK_PHY",
			"CONFIG_GPIO_ROCKCHIP",
			"CONFIG_I2C_RK3X",
			"CONFIG_SPI_ROCKCHIP",
			"CONFIG_SPI_SPIDEV",
			"CONFIG_SERIAL_8250_DW",
		},
		ModulesDisabled: true,
	},

	"qemu-virt": {
		BoardID: "qemu-virt",
		Source: Source{
			Repo:    fleetKernelRepo,
			Ref:     fleetKernelTag,
			RefKind: TagRef,
		},
		Defconfig: "defconfig",
		Toolchain: Toolchain{KernelArch: "arm64", CrossCompile: "aarch64-linux-gnu-"},

		ConfigFragment: qemuvirtkernel.ConfigFragment,
		// No DTSPatches, no DTB: qemu's -M virt machine supplies its own
		// device tree - see qemuvirtkernel's package doc comment.

		KernelMakeTarget: "Image",
		KernelSourcePath: "arch/arm64/boot/Image",
		KernelFilename:   "Image",

		RequiredY: []string{
			"CONFIG_VIRTIO_BLK",
			"CONFIG_VIRTIO_NET",
			"CONFIG_VIRTIO_PCI",
			"CONFIG_VIRTIO_MMIO",
			"CONFIG_SERIAL_AMBA_PL011",
			"CONFIG_SERIAL_AMBA_PL011_CONSOLE",
			"CONFIG_RTC_DRV_PL031",
		},
		ForbiddenY: []string{
			"CONFIG_ARCH_ROCKCHIP",
			"CONFIG_ARCH_BCM2835",
			"CONFIG_ARCH_BCM_IPROC",
			"CONFIG_ARCH_BCMBCA",
			"CONFIG_WLAN",
			"CONFIG_CFG80211",
			"CONFIG_SOUND",
			"CONFIG_SND",
			"CONFIG_DRM",
			"CONFIG_MEDIA_SUPPORT",
			// Legacy mass-storage gadget: leaks in =y from the arm64
			// defconfig baseline (see this board's kernel-fragment.config
			// for the full rationale). Found during rock-4se real-hardware
			// bring-up (bean gosd-sz6p).
			"CONFIG_USB_MASS_STORAGE",
		},
		ModulesDisabled: true,
	},
}

// Get returns the KernelSpec for boardID and whether one is registered.
func Get(boardID string) (KernelSpec, bool) {
	s, ok := specs[boardID]
	return s, ok
}

// BoardIDs returns every board ID with a registered KernelSpec, sorted.
func BoardIDs() []string {
	ids := make([]string, 0, len(specs))
	for id := range specs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
