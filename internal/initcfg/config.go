// Package initcfg owns the schema gosd-init reads at boot: the config.json
// baked onto every image by the gosd CLI, and the gosd.* kernel command-line
// parameters. Both are pure data formats with no syscall dependencies, so
// this package has no build tags and is fully unit-testable on any OS.
//
// Other packages (the config tree's runtime reader, provisioning-file
// consumption) import this package for the Config type rather than defining
// their own.
package initcfg

import (
	"encoding/json"
	"fmt"
	"time"
)

// Config is the schema of /etc/gosd/config.json, baked into every image at
// build time by the gosd CLI.
type Config struct {
	Board    string `json:"board"`
	Hostname string `json:"hostname"`
	Wifi     Wifi   `json:"wifi"`

	// NTPServers is the ordered list of NTP servers gosd-init's timesync
	// package queries. Optional: a nil/empty slice (including every
	// config.json baked before this field existed) means "use
	// timesync.DefaultServers" (pool.ntp.org); this package only owns the
	// schema, not that default, to keep it free of other packages'
	// constants.
	NTPServers []string `json:"ntpServers,omitempty"`

	// Env holds developer-set default app environment variables, baked in
	// at build time (gosd build --env). It's the lowest-precedence layer
	// in the app env merge: a file in the card's config/env/ directory
	// overrides the same name here. Optional: omitted entirely for
	// every config.json baked before this field existed.
	Env map[string]string `json:"env,omitempty"`

	// ConfigDigests is the SHA-256 (hex) of every value file in this
	// image's config/ tree, keyed by its path within that tree
	// ("wifi/ssid", "env/API_TOKEN") - the bytes gosd baked in, padding
	// included, not the trimmed value they read as (see
	// internal/configtree.Value.SHA256). It is how the device tells a value
	// somebody changed - by hand on the card, or by a provisioning tool
	// injecting into the downloaded .img - from the one this image shipped
	// with, without keeping a second copy of every default anywhere.
	// Optional: absent when the image carries no config tree at all.
	ConfigDigests map[string]string `json:"configDigests,omitempty"`

	// DataExpand marks an image built with --data-size=expand: it ships
	// with no data partition, and gosd-init creates and formats one
	// filling the rest of the card on first boot (see
	// cmd/gosd-init/internal/dataexpand). Optional: absent — including
	// every config.json baked before this field existed — means no
	// expansion, exactly like --data-size=0.
	DataExpand bool `json:"dataExpand,omitempty"`

	// DataFlush marks whether gosd-init mounts the data partition, and blockmount
	// mounts any emmc/disk vfat volume, with the vfat "flush" option: it
	// pushes a file's data and metadata to the card promptly on close(2),
	// at a real write-throughput cost. Baked in by gosd build --data-flush;
	// default false, since normal Linux writeback (~30s dirty_expire) is
	// fast and "flush" was never enough for rename durability anyway
	// (bean gosd-0nk4) — apps that need durability already use the
	// fsync/rename sequence documented in docs/runtime.md's "Making a
	// write durable", which behaves identically either way. Overridable
	// per-device via the card's own data_flush setting (empty there means
	// "use this baked value" — see
	// cmd/gosd-init/internal/boot/sequence.go's effectiveDataFlush).
	// Optional: false for every config.json baked before this field
	// existed, which is exactly this field's own default.
	//
	// Deliberately excluded from ComputeIdentity's payload, along with the
	// rest of config.json (see ComputeIdentity's docstring): flipping
	// --data-flush between builds changes nothing else in the boot
	// payload, so — like DataExpand, and unlike Hostname/Wifi/Env, which
	// settings are also written into the hashed config tree — it can never
	// move Identity. Pinned by
	// TestBuildIdentityUnaffectedByDataFlush (cmd/gosd/build_integration_test.go).
	DataFlush bool `json:"dataFlush,omitempty"`

	// DataFilesystem records gosd build --data-filesystem: which
	// filesystem the data partition is formatted as (fat32, the universal
	// default every host can read and repair, or ext4, journaled and
	// crash-resilient but unreadable from a macOS or Windows host - see
	// COMPATIBILITY.md's ext4 data partition row). gosd-init maps this
	// string to a diskfmt.FS and
	// mounts /data with it, whether the partition ships in the image or
	// (DataExpand) is created and formatted on first boot. Optional:
	// absent - including every config.json baked before this field
	// existed - means fat32.
	//
	// Unlike DataFlush, this is not a mere mount tweak: it changes the
	// data partition's on-card layout, the same on-disk-ABI category
	// --boot-size is in (see docs/design/upgrade-path.md) - an app that
	// changes it between releases loses its existing data partition on the
	// next upgrade, reformatted to the newly requested filesystem, rather
	// than adopted. Despite that, it is still deliberately excluded from
	// ComputeIdentity's hashed payload, for the same structural reason
	// DataExpand and DataFlush are: config.json is excluded from that
	// payload in its entirety (see ComputeIdentity's docstring), and this
	// field never appears anywhere else in the payload the way
	// Hostname/Wifi/Env do via the config tree. That's consistent with
	// Identity's actual job - telling boot *payload* builds apart for
	// upgrade-skew checks - rather than a full-disk layout
	// fingerprint: DataSizeBytes and BootSizeBytes, which change on-card
	// layout at least as much as this field does, aren't even baked into
	// config.json at all, let alone hashed. The layout-ABI story for a
	// partition-affecting flag like this one belongs to
	// docs/design/upgrade-path.md, not to Identity. Pinned by
	// TestBuildIdentityUnaffectedByDataFilesystem
	// (cmd/gosd/build_integration_test.go).
	DataFilesystem string `json:"dataFilesystem,omitempty"`

	// DataLabel is the volume label the data partition carries - per-app
	// (`gosd build --label-prefix`, see internal/naming.LabelsFor), so a
	// flashed card shows up on a person's desktop named after the app
	// rather than after GoSD. gosd-init compares it against what the
	// partition actually holds before adopting a survivor a reflash left
	// behind, and stamps it onto anything it formats itself (see
	// cmd/gosd-init/internal/dataexpand). Nothing on-device reads the boot
	// partition's label, so only this one is baked in.
	//
	// Not optional, and deliberately without omitempty: `gosd build`
	// always resolves a label pair, so every config.json this version
	// writes carries one, and an empty value is a wiring bug rather than
	// a default to fill in - dataexpand refuses it outright rather than
	// guessing a label to compare against.
	//
	// Like DataFilesystem (and unlike DataFlush) this is on-card ABI, in
	// --boot-size's category (see docs/design/upgrade-path.md): an app
	// that changes its label prefix - or is renamed, since the default
	// prefix follows the app's name - between releases finds its old data
	// partition unadoptable on the next reflash-upgrade, and it is
	// reformatted rather than kept. Also deliberately excluded from
	// ComputeIdentity's hashed payload, for exactly the structural reason
	// DataFilesystem is: config.json is excluded from that payload in its
	// entirety, and this field appears nowhere else in it. Pinned by
	// TestBuildIdentityUnaffectedByLabelPrefix
	// (cmd/gosd/build_integration_test.go).
	DataLabel string `json:"dataLabel"`

	// IngressCloudflared marks an image built with `gosd build --ingress
	// cloudflared`: a cloudflared binary is baked into the initramfs at
	// /bin/cloudflared (see internal/cloudflaredpin and
	// cmd/gosd/ingress.go). This is the entire build->runtime contract for
	// ingress (epic gosd-virc, locked decision 7): it carries only the
	// "binary is baked" bit, nothing about whether or how a tunnel is
	// actually configured - that lives in the card's
	// config/ingress/cloudflared/ settings, which gosd-init reads
	// separately at boot. gosd-init never probes the filesystem for the
	// binary itself; this field is the only source of truth for whether
	// it exists. Optional: absent - including every config.json baked
	// before this field existed - means no cloudflared binary was baked
	// in.
	IngressCloudflared bool `json:"ingressCloudflared,omitempty"`

	// IngressTailscaleFunnel marks an image built with `gosd build --ingress
	// tailscale-funnel`: the tsnet-based Funnel shim (cmd/gosd-tsfunnel) is
	// baked into the initramfs at /bin/gosd-tsfunnel (see internal/build's
	// CrossCompileTsfunnel and cmd/gosd/ingress.go). Mirrors
	// IngressCloudflared's contract exactly: this bit only says the binary
	// is baked, nothing about whether or how Funnel is actually configured
	// - that lives in the card's config/ingress/tailscale-funnel/ settings,
	// which gosd-init reads separately at boot. Optional: absent - including every config.json baked before
	// this field existed - means no shim binary was baked in.
	IngressTailscaleFunnel bool `json:"ingressTailscaleFunnel,omitempty"`

	// Identity is a content-derived digest of this build's boot payload,
	// baked in by gosd build (see ComputeIdentity's docstring for the
	// exact recipe, and internal/pipeline for where it's computed).
	// Identical rebuilds from identical inputs produce the identical
	// Identity — it is never a timestamp or a random id — which is what
	// makes it usable for upgrade-skew detection: does the running image
	// match the one a device's stored settings were last written under?
	// Optional: empty for every config.json baked before this field
	// existed; callers must treat that as "unknown, not comparable"
	// rather than as a mismatch (see ShortIdentity).
	Identity string `json:"identity,omitempty"`

	// BoardDisplayName is the selected board's human-readable name (see
	// boards.Board.DisplayName), baked in by gosd build/run for
	// LAST_FATAL_ERROR.md's "device:" line - "Raspberry Pi Zero 2W" rather
	// than the bare "pi-zero-2w" this struct's Board field carries (bean
	// gosd-my8e, epic gosd-47z3). Optional: empty for every config.json
	// baked before this field existed. Developer-set report metadata like
	// AppName/AppVersion/SupportURL below: config.json only, no setting on
	// the card, no GOSD_* override, excluded from ComputeIdentity's hashed
	// payload (config.json is excluded from that payload in its entirety),
	// and no part of the data-partition adoption gate - not on-card ABI.
	//
	// CAUTION for any renderer consuming this (gosd-pun9): it names the
	// board THIS FIELD WAS BAKED FOR, not necessarily the board gosd-init
	// ends up running as at boot. cmd/gosd-init/internal/boot/sequence.go
	// overwrites its in-memory Config.Board with any gosd.board=<id> kernel
	// cmdline parameter it finds (see initcfg.CmdlineArgs.Board) BEFORE most
	// callers ever see the parsed config - and cmdline.txt, which carries
	// that parameter, is a hand-editable file on the FAT boot partition. Once
	// that override has run, Board no longer necessarily names the board
	// BoardDisplayName was baked for, and this field is never touched by
	// that override to keep them in sync. A consumer must capture the board
	// id at the point config.json is parsed - before any gosd.board=
	// override - and pair BoardDisplayName only with THAT id; if the
	// board id actually in effect at report-writing time differs, fall back
	// to printing the bare effective id rather than a display name that may
	// now describe the wrong hardware. Building that fallback is gosd-pun9's
	// job, not this package's - this field only carries the baked value.
	BoardDisplayName string `json:"boardDisplayName,omitempty"`

	// AppName is the app's name, baked in by gosd build: the sanitized
	// basename of the main package's directory - the same source
	// --hostname's unset default uses (see cmd/gosd's deriveAppName),
	// resolved once at build time so it can't drift if --hostname,
	// or the card's own hostname setting, later changes the device's actual
	// hostname.
	// It exists for LAST_FATAL_ERROR.md's "image:" line (bean gosd-my8e,
	// epic gosd-47z3), not for gosd-init's own runtime behavior, which
	// still uses Hostname throughout. Optional: empty for every config.json
	// baked before this field existed; a reader must treat that as
	// "unknown," never silently fall back to Hostname (a user-renamed
	// device would then report the wrong app name).
	//
	// Developer-set, like AppVersion and SupportURL below: config.json only,
	// no setting on the card, no GOSD_* override. Also
	// like them, it's deliberately excluded from ComputeIdentity's hashed
	// payload (config.json is excluded from that payload in its entirety -
	// see ComputeIdentity's docstring) and plays no part in the data-
	// partition adoption gate (docs/design/upgrade-path.md): it's report
	// metadata alone, not on-card ABI.
	AppName string `json:"appName,omitempty"`

	// AppVersion is gosd build --app-version's free-form value - whatever
	// the developer passes (e.g. "1.4.2"), never interpreted or validated by
	// gosd. It's baked in for LAST_FATAL_ERROR.md's "image:" line; when
	// unset (the flag's default), the report falls back to the image's
	// content-derived Identity alone (see ShortIdentity). Optional: empty
	// for every config.json baked before this field existed, and whenever
	// the flag is omitted.
	//
	// LOCKED (JP, 2026-08-11): deliberately not derived from
	// debug.ReadBuildInfo's VCS state. gosd compiles the user's app on their
	// own machine, where that state may be dirty or absent, and a wrong
	// version in a crash report is worse than no version at all - see bean
	// gosd-my8e.
	AppVersion string `json:"appVersion,omitempty"`

	// SupportURL is gosd build --app-support-url's value: an absolute http(s)
	// URL validated at build time (see cmd/gosd's parseSupportURL) - a
	// broken link in a crash report is worse than no link. Baked in so
	// LAST_FATAL_ERROR.md can point a device's owner somewhere when it has
	// no specific fix to suggest. Optional: empty - including every
	// config.json baked before this field existed - means the report's
	// fallback fix text has no link to offer.
	SupportURL string `json:"supportURL,omitempty"`

	// BuildTimestamp is when gosd build assembled this image (UTC,
	// RFC3339Nano), baked in by the CLI. It's gosd-init's timesync
	// package's clock floor (see BuildTime and gosd-0esw): even on a
	// board with a battery-backed RTC (see gosd-achn), a power cut
	// without a coin cell — or a board with no RTC at all, the Pi family
	// — still starts the clock near the Unix epoch, and an SNTP result
	// reporting a time before the image was even built can only be
	// wrong — a forged or badly misbehaving server, never a legitimate
	// one.
	//
	// Deliberately excluded from ComputeIdentity's payload without any
	// special-casing: that function's docstring already excludes
	// config.json in its entirety (not just Identity), so a value that
	// changes on every rebuild lives here safely — it never touches
	// build reproducibility.
	//
	// Optional: empty for every config.json baked before this field
	// existed; BuildTime returns the zero time.Time in that case, which
	// callers must treat as "no floor available," not as "before the
	// epoch."
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
}

// BuildTime parses BuildTimestamp, returning the zero time.Time if it's
// empty or malformed (e.g. a config.json baked before this field
// existed) — callers must treat that as "no floor available," exactly
// how ShortIdentity treats an empty Identity as "unknown, not
// comparable."
func (c Config) BuildTime() time.Time {
	if c.BuildTimestamp == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, c.BuildTimestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// shortIdentityLen is how many leading hex characters of Identity
// ShortIdentity keeps: enough to tell builds apart at a glance in a boot
// log line, without spelling out the full SHA-256.
const shortIdentityLen = 12

// ShortIdentity returns a truncated, human-scannable form of Identity, for
// logging (e.g. gosd-init's boot-time "[gosd] image identity: <short
// digest>" line). Empty when Identity is empty, e.g. an image built before
// this field existed.
func (c Config) ShortIdentity() string {
	if len(c.Identity) <= shortIdentityLen {
		return c.Identity
	}
	return c.Identity[:shortIdentityLen]
}

// Wifi holds the baked-in WPA2-PSK or open network credentials. Both fields
// empty means no WiFi is configured.
type Wifi struct {
	SSID       string `json:"ssid"`
	Passphrase string `json:"passphrase"`
}

// ParseConfig parses config.json contents into a Config. Missing data (a nil
// or empty slice, as when the file doesn't exist on disk) yields a zero
// Config rather than an error, since every field is optional. Malformed JSON
// is reported as an actionable error rather than crashing the caller.
func ParseConfig(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config.json is not valid JSON: %w", err)
	}
	return cfg, nil
}
