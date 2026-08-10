// Package initcfg owns the schema gosd-init reads at boot: the config.json
// baked onto every image by the gosd CLI, and the gosd.* kernel command-line
// parameters. Both are pure data formats with no syscall dependencies, so
// this package has no build tags and is fully unit-testable on any OS.
//
// Later beans (gosd.toml parsing, provisioning-file consumption) are
// expected to import this package for the Config type rather than defining
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
	// in gosd.toml [env]'s locked merge: a hand-edited gosd.toml [env]
	// entry overrides the same key here. Optional: omitted entirely for
	// every config.json baked before this field existed.
	Env map[string]string `json:"env,omitempty"`

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
	// per-device via gosd.toml's data_flush key (absent there means "use
	// this baked value" — see gosdtoml.Config.DataFlush and
	// cmd/gosd-init/internal/boot/sequence.go's effectiveDataFlush).
	// Optional: false for every config.json baked before this field
	// existed, which is exactly this field's own default.
	//
	// Deliberately excluded from ComputeIdentity's payload, along with the
	// rest of config.json (see ComputeIdentity's docstring): flipping
	// --data-flush between builds changes nothing else in the boot
	// payload, so — like DataExpand, and unlike Hostname/Wifi/Env, which
	// are also baked into the hashed gosd.toml template — it can never
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
	// Hostname/Wifi/Env do via gosd.toml. That's consistent with
	// Identity's actual job - telling boot *payload* builds apart for
	// upgrade-skew/self-update checks - rather than a full-disk layout
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
	// actually configured - that lives in gosd.toml's [ingress.cloudflared]
	// section (see gosdtoml.IngressCloudflared), which gosd-init reads
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
	// - that lives in gosd.toml's [ingress.tailscale-funnel] section (see
	// gosdtoml.IngressTailscaleFunnel), which gosd-init reads separately at
	// boot. Optional: absent - including every config.json baked before
	// this field existed - means no shim binary was baked in.
	IngressTailscaleFunnel bool `json:"ingressTailscaleFunnel,omitempty"`

	// Identity is a content-derived digest of this build's boot payload,
	// baked in by gosd build (see ComputeIdentity's docstring for the
	// exact recipe, and internal/pipeline for where it's computed).
	// Identical rebuilds from identical inputs produce the identical
	// Identity — it is never a timestamp or a random id — which is what
	// makes it usable both for upgrade-skew detection (does the running
	// image match what a provisioning snapshot was taken from?) and for
	// a future self-update's "am I already running this?" check.
	// Optional: empty for every config.json baked before this field
	// existed; callers must treat that as "unknown, not comparable"
	// rather than as a mismatch (see ShortIdentity).
	Identity string `json:"identity,omitempty"`

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
