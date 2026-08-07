// Package provsnapshot keeps a copy of a device's settled provisioning on
// the data partition, and uses it to heal the first boot after a reflash.
//
// Reflashing a card rewrites the whole of GOSD-BOOT, so every hand-edited
// gosd.toml value and every wizard-provided hostname/WiFi credential is
// replaced by the new image's baked defaults, while GOSD-DATA survives
// (see docs/design/upgrade-path.md §2 and §3). This package closes that
// gap: after provisioning settles on each successful boot it writes a
// snapshot into /data, and when a later boot finds the running image's
// identity differs from the one the snapshot was taken under, it puts the
// operator's provisioning back — into gosd.toml on GOSD-BOOT, so it stays
// visible and editable exactly where they left it.
//
// # What the snapshot is
//
// A directory (Dir, under the data partition's mount point) holding:
//
//   - gosd.toml — the provisioning this boot actually settled on
//     (hostname, WiFi and [env] after the locked gosd.toml > cloud-init >
//     config.json merge), rendered with the same template the gosd CLI
//     writes onto an image. The *effective* values are snapshotted, not a
//     copy of the card's gosd.toml, because provisioning that arrived via
//     the Imager wizard never appears in gosd.toml at all — and rescuing
//     exactly that case ("reflash without re-running the wizard") is what
//     the snapshot is for.
//   - snapshot.json — the image identity the snapshot was taken under, the
//     baked defaults (config.json's hostname/WiFi/[env]) that were
//     contemporaneous with it, and a SHA-256 of gosd.toml. It is written
//     last and acts as the commit record: a snapshot whose gosd.toml
//     doesn't match the recorded digest is a torn write and is ignored
//     wholesale.
//
// # The merge rules
//
// The locked invariant (docs/design/upgrade-path.md §3) is that the wizard
// beats the snapshot and the snapshot beats baked defaults. Each field —
// the hostname, the WiFi pair, and every [env] key independently — is
// classified on the same three-way test:
//
//   - *Fresh intent*: something the operator provided for the card as it is
//     now — a cloud-init hostname or WiFi network left by the Imager wizard
//     (hostname and WiFi only; cloud-init carries no [env]), or a gosd.toml
//     value that differs from the running image's baked default. A freshly
//     flashed card's gosd.toml WiFi/[env] are the rendered template, so they
//     match the baked defaults exactly; any difference is therefore a
//     hand-edit made before this boot. Hostname is the one field that
//     usually *doesn't* match at all on a freshly flashed card: a default
//     (non-explicit) build ships the hostname line commented out (bean
//     gosd-4hz1), so gosd.toml carries no hostname rather than one equal to
//     config.json's baked default — freshHostname's own emptiness check
//     treats a commented-out line the same as a template match, i.e. no
//     fresh intent, so this doesn't change the classification, only why it
//     holds for hostname specifically. Fresh intent only blocks a restore —
//     it is never written into gosd.toml, so the locked precedence chain
//     decides which of the two actually takes effect, exactly as it would
//     have without any snapshot.
//   - *Snapshot intent*: the snapshot's effective value differs from the
//     baked default recorded in that same snapshot — the contemporaneous
//     default, i.e. what the image the snapshot was taken from would have
//     supplied on its own. That difference is the proof it was the
//     operator's doing.
//   - Otherwise the value is just a baked default, and the newly flashed
//     image's own default is the freshest statement of it.
//
// A field is restored if, and only if, there is no fresh intent for it and
// there is snapshot intent for it. Two consequences worth stating:
//
//   - If the new image's template changed a key's baked default and the
//     snapshot value equals the *old* default, the snapshot has no intent
//     for that key (it never differed from its contemporaneous default),
//     so the new default wins — a template change is not overridden by a
//     value the operator never chose.
//   - If the operator did hand-edit a key and the new image also changed
//     that key's default, the hand-edit is restored: the invariant puts the
//     snapshot above baked defaults.
//
// [env] is merged key by key (never as a whole-table replace), matching
// gosd.toml [env]'s own locked precedence; a key the operator added that
// the image never baked counts as a hand-edit, since it differs from the
// (absent, empty) contemporaneous default. WiFi is restored as an
// ssid/passphrase pair, because a changed passphrase for the same SSID is
// as much a hand-edit as a changed network. A restored WiFi passphrase may
// be the 64-hex pre-hashed PSK cloud-init supplies rather than a plaintext
// passphrase; wifiup distinguishes the two by shape, so writing either into
// gosd.toml works unchanged.
//
// Restores are written back to gosd.toml on GOSD-BOOT (durably, briefly
// remounting it read-write) and applied in memory for the boot in progress,
// so a reflashed board rejoins its network without a second reboot. Writing
// the file re-renders it from the gosd CLI's template: an operator's own
// comments in a card gosd.toml they edited before first boot are not
// preserved, only their values.
//
// # Failure is never fatal
//
// Every path here is best-effort, in the same spirit as internal/provision:
// a missing snapshot, a torn or malformed one, an unwritable (read-only,
// or absent) /data, or a failed write-back is logged and boot continues.
// An image built before config.json carried an identity, or a snapshot
// taken by one, cannot detect a reflash at all; the snapshot is still kept
// up to date, and the self-heal is skipped with a log line. When the
// write-back to GOSD-BOOT fails the snapshot is deliberately *not*
// refreshed, so the next boot still sees the identity skew and retries the
// heal rather than silently forgetting it.
package provsnapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/provision"
)

// Dir is the snapshot directory, relative to the data partition's mount
// point (i.e. /data/.gosd/provision-snapshot on a running device).
const Dir = ".gosd/provision-snapshot"

// BootConfigFile is the name of the hand-editable config file at the root
// of GOSD-BOOT that a restore writes back to.
const BootConfigFile = "gosd.toml"

const (
	tomlFile = "gosd.toml"
	metaFile = "snapshot.json"

	// schemaVersion guards against a future gosd-init reading a snapshot
	// format it doesn't understand: an unrecognised version is treated as
	// no snapshot at all rather than misread.
	schemaVersion = 1
)

// Provisioning is one layer of a device's provisioning: the hostname, WiFi
// network and app environment variables it names.
type Provisioning struct {
	Hostname string
	Wifi     gosdtoml.Wifi
	Env      map[string]string
}

func (p Provisioning) equal(other Provisioning) bool {
	return p.Hostname == other.Hostname && p.Wifi == other.Wifi && maps.Equal(p.Env, other.Env)
}

// Snapshot is what a previous boot recorded on the data partition.
type Snapshot struct {
	// Identity is the image identity (initcfg.Config.Identity) the
	// snapshot was taken under. Empty when it was taken by an image built
	// before that field existed, which makes reflash detection impossible.
	Identity string

	// Effective is the provisioning that boot settled on.
	Effective Provisioning

	// Baked is the contemporaneous config.json defaults Effective was
	// merged against — the yardstick that makes a hand-edit provable.
	Baked Provisioning
}

func (s Snapshot) equal(other Snapshot) bool {
	return s.Identity == other.Identity &&
		s.Effective.equal(other.Effective) &&
		s.Baked.equal(other.Baked)
}

// CloudInit is what the Raspberry Pi Imager wizard left on GOSD-BOOT this
// boot (see internal/provision); both fields are empty when the wizard was
// skipped.
type CloudInit struct {
	Hostname string
	Wifi     []provision.WifiNetwork
}

func (c CloudInit) wifi() (gosdtoml.Wifi, bool) {
	if len(c.Wifi) == 0 {
		return gosdtoml.Wifi{}, false
	}
	return gosdtoml.Wifi{SSID: c.Wifi[0].SSID, Passphrase: c.Wifi[0].Password}, true
}

// Input is everything the boot in progress resolved before the data
// partition was mounted.
type Input struct {
	// Identity is the running image's identity (initcfg.Config.Identity),
	// empty on any image built before that field existed.
	Identity string

	// Baked is config.json's provisioning: the defaults this card's
	// gosd.toml template was rendered from.
	Baked Provisioning

	// CloudInit is the wizard's provisioning, if any.
	CloudInit CloudInit

	// GosdToml is the card's gosd.toml exactly as it was read this boot.
	GosdToml gosdtoml.Config
}

// Result is what the caller should act on for the rest of this boot.
type Result struct {
	// GosdToml is the card's gosd.toml with anything the snapshot restored
	// merged in. It equals Input.GosdToml when nothing was restored.
	GosdToml gosdtoml.Config

	// HostnameRestored reports that GosdToml.Hostname came from the
	// snapshot, so the caller must re-apply it before starting the app.
	HostnameRestored bool
}

// Deps is the file-IO seam: production wires it to the real data partition
// and boot partition through NewDeps, tests supply fakes.
type Deps struct {
	// ReadFile reads one file from the snapshot directory. A non-existent
	// file must report an error satisfying os.IsNotExist.
	ReadFile func(name string) ([]byte, error)

	// WriteFile durably writes one file into the snapshot directory,
	// creating the directory if it isn't there yet.
	WriteFile func(name string, data []byte) error

	// WriteBootFile durably writes one file at the root of the normally
	// read-only GOSD-BOOT partition.
	WriteBootFile func(name string, data []byte) error

	Log func(format string, args ...any)
}

// NewDeps wires Deps against the real filesystem: dataDir is the snapshot
// directory on the mounted data partition (dataMount/Dir), and
// writeBootFile writes a named file at the root of the mounted GOSD-BOOT
// partition (see boot.Platform.WriteBootFile, which handles the
// read-write remount that needs).
func NewDeps(dataDir string, writeBootFile func(name string, data []byte) error, log func(format string, args ...any)) Deps {
	return Deps{
		ReadFile: func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dataDir, name)) },
		WriteFile: func(name string, data []byte) error {
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return err
			}
			return WriteFileDurably(filepath.Join(dataDir, name), data)
		},
		WriteBootFile: writeBootFile,
		Log:           log,
	}
}

// Run heals the provisioning of a first boot after a reflash and then
// brings the snapshot up to date. It never returns an error: every problem
// is logged and boot continues (see the package doc).
func Run(deps Deps, in Input) Result {
	result := Result{GosdToml: in.GosdToml}

	snap, found := load(deps)
	saveAllowed := true
	if found {
		result, saveAllowed = heal(deps, in, snap)
	}

	if saveAllowed {
		save(deps, in, result, snap, found)
	}
	return result
}

// heal applies the snapshot to a freshly flashed card, reporting the
// provisioning the rest of this boot should use and whether the snapshot
// may be refreshed afterwards (it may not if a restore was needed but
// couldn't be written to the card, so that the next boot retries).
func heal(deps Deps, in Input, snap Snapshot) (Result, bool) {
	result := Result{GosdToml: in.GosdToml}

	switch {
	case in.Identity == "" || snap.Identity == "":
		deps.Log("provisioning snapshot: no image identity to compare, so a reflash can't be detected; self-heal skipped")
		return result, true
	case in.Identity == snap.Identity:
		return result, true
	}

	plan := planRestore(snap, in)
	merged, changed := plan.apply(in.GosdToml)
	if !changed {
		deps.Log("first boot after reflash (%s -> %s): nothing in the provisioning snapshot needs restoring", short(snap.Identity), short(in.Identity))
		return result, true
	}

	deps.Log("first boot after reflash (%s -> %s): restoring provisioning from the snapshot", short(snap.Identity), short(in.Identity))
	if plan.Hostname != "" {
		deps.Log("restoring hostname %q from the provisioning snapshot", plan.Hostname)
	}
	if plan.Wifi != nil {
		deps.Log("restoring WiFi network %q from the provisioning snapshot", plan.Wifi.SSID)
	}
	if len(plan.Env) > 0 {
		deps.Log("restoring hand-edited [env] value(s) from the provisioning snapshot: %s", strings.Join(sortedKeys(plan.Env), ", "))
	}

	if deps.WriteBootFile == nil {
		deps.Log("provisioning snapshot: nowhere to write %s back to; the restore applies to this boot only", BootConfigFile)
		return Result{GosdToml: merged, HostnameRestored: plan.Hostname != ""}, false
	}
	// bakeHostname is always true here: a restored hostname is provable
	// operator intent (a hand-edit or a wizard hostname that already took
	// effect), so it's written back uncommented, exactly like a hand-edit -
	// gosdtoml.Render itself still leaves the line commented when merged.
	// Hostname is empty (nothing to restore). merged.Ingress carries
	// through whatever this boot's own gosd.toml already had (see apply),
	// so this write can't blank it out as a side effect.
	rendered := gosdtoml.Render(merged.Hostname, true, merged.Wifi.SSID, merged.Wifi.Passphrase, merged.Env, merged.Ingress.Cloudflared)
	if err := deps.WriteBootFile(BootConfigFile, rendered); err != nil {
		// The values still apply to this boot, so the board comes back
		// now; leaving the snapshot untouched is what makes the next boot
		// try the write again instead of forgetting the restore.
		deps.Log("writing the restored %s back to the boot partition failed, so it applies to this boot only and will be retried next boot: %v", BootConfigFile, err)
		return Result{GosdToml: merged, HostnameRestored: plan.Hostname != ""}, false
	}
	deps.Log("restored provisioning written back to %s on the boot partition", BootConfigFile)

	return Result{GosdToml: merged, HostnameRestored: plan.Hostname != ""}, true
}

// plan is the set of values a snapshot restores onto the current card.
type plan struct {
	Hostname string
	Wifi     *gosdtoml.Wifi
	Env      map[string]string
}

// planRestore applies the package doc's merge rules: restore a field only
// where the card offers no fresh operator intent for it and the snapshot
// provably holds some.
func planRestore(snap Snapshot, in Input) plan {
	var p plan

	if snap.Effective.Hostname != snap.Baked.Hostname && !freshHostname(in) {
		p.Hostname = snap.Effective.Hostname
	}
	if snap.Effective.Wifi != snap.Baked.Wifi && !freshWifi(in) {
		wifi := snap.Effective.Wifi
		p.Wifi = &wifi
	}
	for key, value := range snap.Effective.Env {
		if value == snap.Baked.Env[key] {
			continue // never a hand-edit: it is the contemporaneous default
		}
		if fresh, ok := in.GosdToml.Env[key]; ok && fresh != in.Baked.Env[key] {
			continue // hand-edited on the new card too, which is more recent
		}
		if p.Env == nil {
			p.Env = make(map[string]string)
		}
		p.Env[key] = value
	}

	return p
}

// freshHostname reports whether the card as flashed already carries a
// hostname the operator chose: the wizard's, or a gosd.toml hand-edit.
func freshHostname(in Input) bool {
	if in.CloudInit.Hostname != "" {
		return true
	}
	return in.GosdToml.Hostname != "" && in.GosdToml.Hostname != in.Baked.Hostname
}

// freshWifi is freshHostname's counterpart for the WiFi pair.
func freshWifi(in Input) bool {
	if _, ok := in.CloudInit.wifi(); ok {
		return true
	}
	return in.GosdToml.Wifi.SSID != "" && in.GosdToml.Wifi != in.Baked.Wifi
}

// apply merges a plan into the card's gosd.toml, reporting whether it
// changed anything at all (a plan can be a no-op when a restored value
// already matches what the new image baked).
func (p plan) apply(cfg gosdtoml.Config) (gosdtoml.Config, bool) {
	// Ingress isn't part of a plan (nothing restores it across a reflash
	// yet — bean gosd-7upw is schema-only), but it must still survive an
	// unrelated hostname/WiFi/[env] restore write on the SAME card: cfg is
	// this boot's own gosd.toml, so carrying its Ingress through here is
	// what stops the write below from silently blanking a hand-edited
	// [ingress.cloudflared] table.
	merged := gosdtoml.Config{Hostname: cfg.Hostname, Wifi: cfg.Wifi, Env: maps.Clone(cfg.Env), Ingress: cfg.Ingress}
	if p.Hostname != "" {
		merged.Hostname = p.Hostname
	}
	if p.Wifi != nil {
		merged.Wifi = *p.Wifi
	}
	for key, value := range p.Env {
		if merged.Env == nil {
			merged.Env = make(map[string]string)
		}
		merged.Env[key] = value
	}

	changed := merged.Hostname != cfg.Hostname || merged.Wifi != cfg.Wifi || !maps.Equal(merged.Env, cfg.Env)
	return merged, changed
}

// load reads the stored snapshot. Anything wrong with it — absent, torn,
// malformed, from a future schema — is reported as "no snapshot", logged
// once, and leaves boot to carry on without a self-heal.
func load(deps Deps) (Snapshot, bool) {
	metaData, err := deps.ReadFile(metaFile)
	if err != nil {
		if os.IsNotExist(err) {
			deps.Log("no provisioning snapshot on the data partition yet")
		} else {
			deps.Log("reading the provisioning snapshot failed, continuing without it: %v", err)
		}
		return Snapshot{}, false
	}
	tomlData, err := deps.ReadFile(tomlFile)
	if err != nil {
		deps.Log("the provisioning snapshot is incomplete, ignoring it: %v", err)
		return Snapshot{}, false
	}

	snap, err := decode(tomlData, metaData)
	if err != nil {
		deps.Log("the provisioning snapshot is unusable, ignoring it: %v", err)
		return Snapshot{}, false
	}
	return snap, true
}

// save brings the snapshot up to date with this boot's settled
// provisioning, skipping the write entirely when nothing has changed —
// every write is an erase-block rewrite on an SD card.
func save(deps Deps, in Input, result Result, stored Snapshot, found bool) {
	want := Snapshot{
		Identity:  in.Identity,
		Effective: effective(in, result.GosdToml),
		Baked:     in.Baked,
	}
	if found && want.equal(stored) {
		deps.Log("provisioning snapshot unchanged")
		return
	}

	tomlData, metaData, err := want.encode()
	if err != nil {
		deps.Log("building the provisioning snapshot failed, continuing without one: %v", err)
		return
	}
	// gosd.toml first, snapshot.json (which carries its digest) second: the
	// second write is the commit record, so a power cut between them leaves
	// a snapshot that the digest check rejects rather than a mixed one.
	if err := deps.WriteFile(tomlFile, tomlData); err != nil {
		deps.Log("provisioning snapshot not saved (no writable data partition?): %v", err)
		return
	}
	if err := deps.WriteFile(metaFile, metaData); err != nil {
		deps.Log("provisioning snapshot not saved (no writable data partition?): %v", err)
		return
	}
	deps.Log("provisioning snapshot saved")
}

// effective resolves what this boot actually settled on, mirroring the
// locked precedence chain gosd.toml > cloud-init > config.json that
// boot.Run and wifiup.ConfigCredentials apply. It records what took
// effect, not what was offered: a cloud-init hostname sits below the
// card's gosd.toml in that chain, but gosd build only renders a hostname
// into gosd.toml uncommented when --hostname was explicitly chosen (see
// bean gosd-4hz1) - the sanitized-default case ships it commented out, so
// card.Hostname is empty and a wizard hostname is the effective one, same
// as it would be with no gosd.toml at all.
func effective(in Input, card gosdtoml.Config) Provisioning {
	p := Provisioning{Hostname: in.Baked.Hostname, Wifi: in.Baked.Wifi}
	if in.CloudInit.Hostname != "" {
		p.Hostname = in.CloudInit.Hostname
	}
	if card.Hostname != "" {
		p.Hostname = card.Hostname
	}
	if wifi, ok := in.CloudInit.wifi(); ok {
		p.Wifi = wifi
	}
	if card.Wifi.SSID != "" {
		p.Wifi = card.Wifi
	}

	if len(in.Baked.Env) > 0 || len(card.Env) > 0 {
		p.Env = make(map[string]string, len(in.Baked.Env)+len(card.Env))
		maps.Copy(p.Env, in.Baked.Env)
		maps.Copy(p.Env, card.Env)
	}
	return p
}

// snapshotMeta is snapshot.json's on-disk schema.
type snapshotMeta struct {
	Schema         int          `json:"schema"`
	Identity       string       `json:"identity,omitempty"`
	Baked          bakedDefault `json:"baked"`
	GosdTomlSHA256 string       `json:"gosdTomlSha256"`
}

type bakedDefault struct {
	Hostname string            `json:"hostname,omitempty"`
	Wifi     bakedWifi         `json:"wifi"`
	Env      map[string]string `json:"env,omitempty"`
}

type bakedWifi struct {
	SSID       string `json:"ssid,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

// encode renders the two snapshot files, in write order. bakeHostname is
// always true: this gosd.toml lives in the snapshot directory, never on
// GOSD-BOOT, so the "leave it commented for the wizard" concern that
// gosdtoml.Render's bakeHostname flag exists for doesn't apply here - the
// snapshot must round-trip whatever Effective.Hostname actually is through
// decode's gosdtoml.Parse. Ingress passes the zero value: like DataFlush,
// it isn't restored by the provisioning snapshot across a reflash yet
// (Provisioning carries no Ingress field — that's the later provsnapshot
// child bean in the ingress epic, gosd-virc), so there's nothing "real" to
// round-trip through this file yet.
func (s Snapshot) encode() ([]byte, []byte, error) {
	tomlData := gosdtoml.Render(s.Effective.Hostname, true, s.Effective.Wifi.SSID, s.Effective.Wifi.Passphrase, s.Effective.Env, gosdtoml.IngressCloudflared{})
	meta := snapshotMeta{
		Schema:   schemaVersion,
		Identity: s.Identity,
		Baked: bakedDefault{
			Hostname: s.Baked.Hostname,
			Wifi:     bakedWifi{SSID: s.Baked.Wifi.SSID, Passphrase: s.Baked.Wifi.Passphrase},
			Env:      s.Baked.Env,
		},
		GosdTomlSHA256: digest(tomlData),
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return tomlData, append(metaData, '\n'), nil
}

// decode reads a snapshot back, rejecting anything that isn't a complete,
// self-consistent one of a schema this gosd-init understands.
func decode(tomlData, metaData []byte) (Snapshot, error) {
	var meta snapshotMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return Snapshot{}, fmt.Errorf("%s is not valid JSON: %w", metaFile, err)
	}
	if meta.Schema != schemaVersion {
		return Snapshot{}, fmt.Errorf("%s uses snapshot schema %d, but this gosd-init only understands %d", metaFile, meta.Schema, schemaVersion)
	}
	if got := digest(tomlData); got != meta.GosdTomlSHA256 {
		return Snapshot{}, fmt.Errorf("%s doesn't match the digest recorded in %s: a torn or edited snapshot", tomlFile, metaFile)
	}

	cfg, _, err := gosdtoml.Parse(tomlData)
	if err != nil {
		return Snapshot{}, fmt.Errorf("the snapshot's %s: %w", tomlFile, err)
	}

	return Snapshot{
		Identity:  meta.Identity,
		Effective: Provisioning{Hostname: cfg.Hostname, Wifi: cfg.Wifi, Env: cfg.Env},
		Baked: Provisioning{
			Hostname: meta.Baked.Hostname,
			Wifi:     gosdtoml.Wifi{SSID: meta.Baked.Wifi.SSID, Passphrase: meta.Baked.Wifi.Passphrase},
			Env:      meta.Baked.Env,
		},
	}, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// shortIdentityLen matches initcfg.Config.ShortIdentity's, so the identities
// in this package's log lines line up with the one gosd-init logs at boot.
const shortIdentityLen = 12

func short(identity string) string {
	if len(identity) <= shortIdentityLen {
		return identity
	}
	return identity[:shortIdentityLen]
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
