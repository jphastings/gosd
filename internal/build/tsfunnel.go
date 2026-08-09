package build

import (
	"fmt"
	"path/filepath"

	"github.com/jphastings/gosd/internal/boards"
)

// gosdTsfunnelRelPkg is where cmd/gosd-tsfunnel lives relative to the root
// of whichever copy of the gosd module we end up building it from (rungs 1
// and 2 of the ladder - see CrossCompileGosdInit's docstring, which this
// mirrors).
const gosdTsfunnelRelPkg = "./cmd/gosd-tsfunnel"

// tsfunnelSrcDirName is cmd/gosd-tsfunnel's own leaf directory name, used to
// derive its location from a --gosd-init-src override (see
// CrossCompileGosdInit's docstring): that flag is documented as pointing at
// gosd-init's own leaf package directory, not a checkout root, so
// CrossCompileTsfunnel swaps the last path element rather than reusing
// overrideDir as-is. Every real gosd source tree ships both binaries
// side-by-side under cmd/ - a full checkout, or a package manager's bundled
// copy (gosd-bfhd's nix hook copies the whole tree, not just cmd/gosd-init)
// - so this holds for every overrideDir a developer would actually pass.
const tsfunnelSrcDirName = "gosd-tsfunnel"

// tsfunnelLDFlags is gosd-65uy decision 2's "-ldflags=-s -w": the shim is
// stripped (no symbol table, no DWARF) since, unlike gosd-init, its content
// never needs to be inspected in the field and every byte counts against
// the boot partition's budget - measured at ~23% smaller in gosd-4fve's bean.
const tsfunnelLDFlags = "-s -w"

// tsfunnelOmitTags is gosd-65uy decision 2's build tag set: every
// ts_omit_<feature> tag from tailscale.com/feature/featuretags.Features
// EXCEPT the four features the shim actually needs (netstack, serve, acme,
// bakedroots) - generated programmatically (not hand-transcribed) by
// filtering featuretags.Features to omittable tags outside that set, the
// same recipe gosd-4fve's bean recorded (74 tags at tailscale.com v1.102.2).
// ts_omit_ssh is in this set: compiling Tailscale SSH out entirely is this
// epic's "no interactive surface" compliance argument (mirroring
// cloudflared's own decision 7). Re-derive this list (iterate
// featuretags.Features, keep only IsOmittable() tags outside the four
// required features, sort, and OmitTag() each) whenever tailscale.com is
// repinned to a version that adds/renames a feature tag.
const tsfunnelOmitTags = "ts_omit_ace,ts_omit_advertiseexitnode,ts_omit_advertiseroutes,ts_omit_appconnectors,ts_omit_aws,ts_omit_bird,ts_omit_c2n,ts_omit_cachenetmap,ts_omit_captiveportal,ts_omit_capture,ts_omit_cliconndiag,ts_omit_clientmetrics,ts_omit_clientupdate,ts_omit_cloud,ts_omit_colorable,ts_omit_completion,ts_omit_completion_scripts,ts_omit_conn25,ts_omit_dbus,ts_omit_debug,ts_omit_debugeventbus,ts_omit_debugportmapper,ts_omit_desktop_sessions,ts_omit_dns,ts_omit_doctor,ts_omit_drive,ts_omit_flashappliance,ts_omit_gro,ts_omit_health,ts_omit_hujsonconf,ts_omit_identityfederation,ts_omit_ipnbus,ts_omit_iptables,ts_omit_kube,ts_omit_linkspeed,ts_omit_linuxdnsfight,ts_omit_listenrawdisco,ts_omit_logtail,ts_omit_netlog,ts_omit_networkmanager,ts_omit_oauthkey,ts_omit_osrouter,ts_omit_outboundproxy,ts_omit_peerapiclient,ts_omit_peerapiserver,ts_omit_portlist,ts_omit_portmapper,ts_omit_posture,ts_omit_qrcodes,ts_omit_relayserver,ts_omit_remoteconfig,ts_omit_resolved,ts_omit_routecheck,ts_omit_runtimemetrics,ts_omit_sdnotify,ts_omit_serviceclientprefs,ts_omit_ssh,ts_omit_synology,ts_omit_syslog,ts_omit_syspolicy,ts_omit_systray,ts_omit_taildrop,ts_omit_tailnetlock,ts_omit_tap,ts_omit_tpm,ts_omit_tundevstats,ts_omit_unixsocketidentity,ts_omit_useexitnode,ts_omit_useproxy,ts_omit_usermetrics,ts_omit_useroutes,ts_omit_wakeonlan,ts_omit_webbrowser,ts_omit_webclient"

// tsfunnelOpts is the crossCompileOpts every CrossCompileTsfunnel rung uses:
// unlike CrossCompileGosdInit's deliberately-empty opts, the shim is always
// tagged and stripped, regardless of which rung located its source.
var tsfunnelOpts = crossCompileOpts{tags: tsfunnelOmitTags, ldflags: tsfunnelLDFlags}

// CrossCompileTsfunnel locates cmd/gosd-tsfunnel's source (gosd's own tsnet
// Funnel shim, epic gosd-65uy decision 1: compiled FROM GOSD SOURCE, the
// main module, never a nested one) and cross-compiles it to outputPath with
// the epic's ts_omit_* tag set and -ldflags="-s -w" (decision 2). It follows
// the exact same 3-rung ladder as CrossCompileGosdInit - dev checkout,
// module cache, --gosd-init-src override - since gosd-tsfunnel ships
// alongside gosd-init in every real gosd source tree; see
// CrossCompileGosdInit's docstring for the two detected rungs, and
// tsfunnelSrcDirName's docstring for how the override rung's directory is
// derived from overrideDir rather than reused as-is.
func CrossCompileTsfunnel(outputPath, overrideDir string, arch boards.Arch) error {
	if overrideDir != "" {
		dir := filepath.Join(filepath.Dir(overrideDir), tsfunnelSrcDirName)
		return crossCompileInDir(dir, ".", outputPath, arch,
			fmt.Sprintf("--gosd-init-src %s (gosd-tsfunnel sibling %s)", overrideDir, dir), tsfunnelOpts)
	}

	if dir, ok := devCheckoutDir(); ok {
		return crossCompileInDir(dir, gosdTsfunnelRelPkg, outputPath, arch,
			fmt.Sprintf("local checkout at %s", dir), tsfunnelOpts)
	}

	dir, err := moduleCacheDir()
	if err != nil {
		return err
	}
	return crossCompileInDir(dir, gosdTsfunnelRelPkg, outputPath, arch,
		fmt.Sprintf("module cache at %s", dir), tsfunnelOpts)
}
