package configstore_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/cardconfig"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/configstore"
	"github.com/jphastings/gosd/internal/configtree"
)

// defaults is the config tree a plain gosd image ships: every setting
// present and empty, which is what "not set" looks like on a card.
var defaults = map[string]string{"hostname": "", "wifi/ssid": "", "data_flush": ""}

// flash writes a config tree onto a fresh card exactly as `gosd build`
// does — every value padded out to its reservation — and returns the card's
// root alongside the per-file digests config.json would carry for it.
func flash(t *testing.T, values map[string]string) (string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	tree := cardconfig.Tree{}
	digests := make(map[string]string, len(values))

	for path, value := range values {
		tree.Set(path, value)
		digests[path] = tree[path].SHA256()
		write(t, filepath.Join(root, configtree.Dir, filepath.FromSlash(path)), string(tree[path].Content))
	}
	return root, digests
}

// edit changes a setting the way somebody with the card in a card reader
// does: whatever they typed, and none of the padding the image shipped.
func edit(t *testing.T, root, path, value string) {
	t.Helper()
	write(t, filepath.Join(root, configtree.Dir, filepath.FromSlash(path)), value+"\n")
}

// revert puts a setting back byte for byte, which is what re-flashing the
// same image over a card does to it.
func revert(t *testing.T, root, path, value string) {
	t.Helper()
	tree := cardconfig.Tree{}
	tree.Set(path, value)
	write(t, filepath.Join(root, configtree.Dir, filepath.FromSlash(path)), string(tree[path].Content))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// boot reconciles one boot of image identity against the card at root,
// returning what it restored and the settings the boot ends up acting on.
func boot(t *testing.T, store, root, identity string, digests map[string]string) (configstore.Result, cardconfig.Tree) {
	t.Helper()
	return bootWithEdit(t, store, root, identity, digests, func(edit func(string) error) error {
		return edit(root)
	})
}

func bootWithEdit(t *testing.T, store, root, identity string, digests map[string]string, editBoot func(func(string) error) error) (configstore.Result, cardconfig.Tree) {
	t.Helper()
	tree := cardconfig.Read(filepath.Join(root, configtree.Dir), discardLog)
	result := configstore.Reconcile(configstore.Deps{
		Dir:      store,
		EditBoot: editBoot,
		Log:      discardLog,
	}, tree, configstore.Options{Identity: identity, Baked: digests})
	return result, tree
}

func discardLog(string, ...any) {}

// onCard is the setting the card holds now, as gosd-init would read it.
func onCard(t *testing.T, root, path string) string {
	t.Helper()
	return cardconfig.Read(filepath.Join(root, configtree.Dir), discardLog).Get(path)
}

func fileOnCard(t *testing.T, root, path string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, configtree.Dir, filepath.FromSlash(path)))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func TestAReFlashedCardGetsItsSettingsBack(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	// A new image is written over the card: its config tree is back to the
	// defaults, and the data partition holding the store survives.
	next, nextDigests := flash(t, defaults)
	result, tree := boot(t, store, next, "image-b", nextDigests)

	if !slices.Equal(result.Restored, []string{"hostname"}) {
		t.Errorf("restored %v, want the hostname somebody set", result.Restored)
	}
	if got := onCard(t, next, "hostname"); got != "kitchen-pi" {
		t.Errorf("hostname on the card = %q, want it put back", got)
	}
	if got := tree.Get("hostname"); got != "kitchen-pi" {
		t.Errorf("hostname this boot = %q, want the restored value, not a second reboot's worth of waiting", got)
	}
}

func TestTheSameImageReFlashedRestoresNothing(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	// Re-flashing the same image leaves a card byte-identical to one
	// somebody edited back to its defaults by hand: the two are the same
	// statement, and gosd-init treats them as one.
	same, sameDigests := flash(t, defaults)
	result, _ := boot(t, store, same, "image-a", sameDigests)

	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: this is the image the settings were kept under", result.Restored)
	}
	if got := onCard(t, same, "hostname"); got != "" {
		t.Errorf("hostname on the card = %q, want the image's own value", got)
	}
}

func TestASettingPutBackToWhatTheImageShipsStopsBeingKept(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	revert(t, card, "hostname", "")
	boot(t, store, card, "image-a", digests)

	// Nothing is left to put back, so the next image's own value stands.
	next, nextDigests := flash(t, map[string]string{"hostname": "new-default", "wifi/ssid": "", "data_flush": ""})
	result, _ := boot(t, store, next, "image-b", nextDigests)

	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: the setting was put back to its default before the re-flash", result.Restored)
	}
	if got := onCard(t, next, "hostname"); got != "new-default" {
		t.Errorf("hostname on the card = %q, want the new image's own value", got)
	}
}

func TestACardChangedSinceItWasFlashedBeatsTheKeptCopy(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	// A provisioning tool injected a value into the downloaded .img (or
	// somebody edited the card before its first boot): the freshest intent
	// there is, and it must not be overwritten by an older one.
	next, nextDigests := flash(t, defaults)
	edit(t, next, "hostname", "injected-name")
	result, tree := boot(t, store, next, "image-b", nextDigests)

	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: the card was changed after it was flashed", result.Restored)
	}
	if got := tree.Get("hostname"); got != "injected-name" {
		t.Errorf("hostname this boot = %q, want the injected value", got)
	}

	// ...and the injected value is what gets kept from now on.
	after, afterDigests := flash(t, defaults)
	result, _ = boot(t, store, after, "image-c", afterDigests)
	if got := onCard(t, after, "hostname"); got != "injected-name" {
		t.Errorf("hostname restored after a later re-flash = %q, want the injected value", got)
	}
	if !slices.Equal(result.Restored, []string{"hostname"}) {
		t.Errorf("restored %v, want the hostname", result.Restored)
	}
}

func TestTheImagesOwnValueIsLeftBesideARestoredOneOnlyWhenItSaysSomething(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	edit(t, card, "wifi/ssid", "kitchen-wifi")
	boot(t, store, card, "image-a", digests)

	// The new image ships a hostname of its own, and no WiFi network — the
	// shape every secret-like setting ships in.
	next, nextDigests := flash(t, map[string]string{"hostname": "new-default", "wifi/ssid": "", "data_flush": ""})
	boot(t, store, next, "image-b", nextDigests)

	if got, ok := fileOnCard(t, next, "hostname"+configtree.NewSuffix); !ok || got != "new-default\n" {
		t.Errorf("hostname%s = %q (present: %t), want this image's own value", configtree.NewSuffix, got, ok)
	}
	if _, ok := fileOnCard(t, next, "wifi/ssid"+configtree.NewSuffix); ok {
		t.Errorf("wifi/ssid%s exists; an empty default has nothing to say, so it must stay quiet", configtree.NewSuffix)
	}
}

func TestASettingThisImageNoLongerHasIsHandedBackOnTheCard(t *testing.T) {
	store := t.TempDir()
	// The tunnel's hostname, not its token: a credential is never kept in
	// the first place, so it can never be an orphan either (see the
	// trust-boundary tests below).
	withTunnel := map[string]string{"hostname": "", "ingress/cloudflared/hostname": ""}
	card, digests := flash(t, withTunnel)
	edit(t, card, "ingress/cloudflared/hostname", "app.example.com")
	boot(t, store, card, "image-a", digests)

	// The next image was built without that ingress agent, so its whole
	// directory is gone from the tree.
	next, nextDigests := flash(t, map[string]string{"hostname": ""})
	result, _ := boot(t, store, next, "image-b", nextDigests)

	unused := "ingress/cloudflared/hostname" + configtree.UnusedSuffix
	if got, ok := fileOnCard(t, next, unused); !ok || got != "app.example.com\n" {
		t.Errorf("%s = %q (present: %t), want the value handed back on the card", unused, got, ok)
	}
	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: this image has no such setting to restore into", result.Restored)
	}

	// It was handed back once, and is not kept beyond that.
	after, afterDigests := flash(t, map[string]string{"hostname": ""})
	boot(t, store, after, "image-c", afterDigests)
	if _, ok := fileOnCard(t, after, unused); ok {
		t.Errorf("%s came back after a second re-flash; one window to retrieve it is the whole point", unused)
	}
}

func TestAnAppEnvironmentVariableSurvivesEvenThoughNoImageShipsIt(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	// config/env/GREETING is somebody's own file: no image ever baked it,
	// so its absence from a new image's tree is not it being retired.
	edit(t, card, "env/GREETING", "hello")
	boot(t, store, card, "image-a", digests)

	next, nextDigests := flash(t, defaults)
	result, tree := boot(t, store, next, "image-b", nextDigests)

	if !slices.Equal(result.Restored, []string{"env/GREETING"}) {
		t.Errorf("restored %v, want the environment variable", result.Restored)
	}
	if got := tree.Group("env")["GREETING"]; got != "hello" {
		t.Errorf("GREETING = %q, want it restored", got)
	}
	if _, ok := fileOnCard(t, next, "env/GREETING"+configtree.UnusedSuffix); ok {
		t.Errorf("env/GREETING%s exists; app environment variables are never retired by an image", configtree.UnusedSuffix)
	}
}

func TestAKeptSettingWrittenWhenThePowerWentIsDroppedRatherThanTrusted(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	// The value reached the data partition; its digest never did — or the
	// value was left half-written under a name the rename had already
	// published. Either way it can't be proved, so it can't be used.
	write(t, filepath.Join(store, "values", "hostname"), "half-written-nam")

	next, nextDigests := flash(t, defaults)
	result, _ := boot(t, store, next, "image-b", nextDigests)

	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: a torn value is not a setting", result.Restored)
	}
	if got := onCard(t, next, "hostname"); got != "" {
		t.Errorf("hostname on the card = %q, want the image's own value", got)
	}
	if _, err := os.Stat(filepath.Join(store, "values", "hostname")); !os.IsNotExist(err) {
		t.Errorf("the torn value is still on the data partition (%v), want it dropped", err)
	}
}

func TestAKeptSettingThatWontReadIsLeftAloneRatherThanWrittenOff(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	// A value that can't be read is not a value that was never written:
	// a failing card can produce the first at any moment, and treating it
	// as the second would forget somebody's setting on the strength of one
	// unlucky read.
	digestPath := filepath.Join(store, "digests", "hostname")
	digest, err := os.ReadFile(digestPath)
	if err != nil {
		t.Fatalf("reading the kept digest: %v", err)
	}
	if err := os.Remove(digestPath); err != nil {
		t.Fatalf("removing the kept digest: %v", err)
	}
	if err := os.Mkdir(digestPath, 0o755); err != nil {
		t.Fatalf("putting something unreadable in its place: %v", err)
	}

	next, nextDigests := flash(t, defaults)
	if result, _ := boot(t, store, next, "image-b", nextDigests); len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: this boot can't prove what it holds", result.Restored)
	}

	// The store reads again — and this boot still counts as the first under
	// the new image, because the one that couldn't read it recorded nothing.
	if err := os.Remove(digestPath); err != nil {
		t.Fatalf("removing the unreadable digest: %v", err)
	}
	write(t, digestPath, string(digest))

	result, _ := boot(t, store, next, "image-b", nextDigests)
	if !slices.Equal(result.Restored, []string{"hostname"}) {
		t.Errorf("restored %v, want the setting the unreadable boot couldn't see", result.Restored)
	}
}

func TestNothingIsForgottenWhenTheCardCantBeWritten(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "image-a", digests)

	// The card has gone read-only under a failing reader: the restore can't
	// land, so this boot must not go on to read the untouched defaults as
	// somebody having put every setting back.
	next, nextDigests := flash(t, defaults)
	result, tree := bootWithEdit(t, store, next, "image-b", nextDigests, func(func(string) error) error {
		return os.ErrPermission
	})
	if got := tree.Get("hostname"); got != "kitchen-pi" {
		t.Errorf("hostname this boot = %q, want the kept value applied even though the card refused it", got)
	}
	if !slices.Equal(result.Restored, []string{"hostname"}) {
		t.Errorf("restored %v, want the hostname reported so the boot can act on it", result.Restored)
	}

	// The card comes good (or is read by a working reader next time).
	later, laterDigests := flash(t, defaults)
	result, _ = boot(t, store, later, "image-b", laterDigests)
	if !slices.Equal(result.Restored, []string{"hostname"}) {
		t.Errorf("restored %v after the card became writable, want the setting still kept", result.Restored)
	}
	if got := onCard(t, later, "hostname"); got != "kitchen-pi" {
		t.Errorf("hostname on the card = %q, want the retry to land it", got)
	}
}

func TestAnImageThatCantSayWhichBuildItIsKeepsSettingsWithoutRestoringThem(t *testing.T) {
	store := t.TempDir()
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")
	boot(t, store, card, "", digests)

	// Kept, but never acted on: with no identity there is no telling a
	// re-flashed card from an edited one, and guessing wrong either
	// resurrects a setting somebody deleted or forgets one they set.
	next, nextDigests := flash(t, defaults)
	result, _ := boot(t, store, next, "", nextDigests)
	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing", result.Restored)
	}

	// An image that does say puts it back.
	result, _ = boot(t, store, next, "image-b", nextDigests)
	if !slices.Equal(result.Restored, []string{"hostname"}) {
		t.Errorf("restored %v, want the setting kept by the identity-less image", result.Restored)
	}
}

func TestWithNoStoreDirectoryNothingHappensAtAll(t *testing.T) {
	card, digests := flash(t, defaults)
	edit(t, card, "hostname", "kitchen-pi")

	result, _ := boot(t, "", card, "image-a", digests)

	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing: an image with no data partition keeps nothing", result.Restored)
	}
}

// withIngress is the config tree an image built with a Cloudflare tunnel
// ships: the token file among the ordinary settings, empty like the rest.
var withIngress = map[string]string{
	"hostname":                  "",
	"wifi/ssid":                 "",
	"data_flush":                "",
	"ingress/cloudflared/token": "",
}

// storeHolds reports the value the store keeps for path, and whether it
// keeps one at all.
func storeHolds(t *testing.T, store, path string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(store, "values", filepath.FromSlash(path)))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func TestATunnelCredentialIsNeverKeptForTheNextReFlash(t *testing.T) {
	// A token is the authorisation to reach this device from anywhere, so
	// it is the one class of setting that never travels on the data
	// partition — see the package doc's trust-boundary section.
	store := t.TempDir()
	card, digests := flash(t, withIngress)
	edit(t, card, "ingress/cloudflared/token", "a-real-tunnel-token")
	edit(t, card, "hostname", "kitchen-pi")

	boot(t, store, card, "image-a", digests)

	if got, kept := storeHolds(t, store, "ingress/cloudflared/token"); kept {
		t.Errorf("the store kept the tunnel token (%q); it must never be copied to the data partition", got)
	}
	if _, kept := storeHolds(t, store, "hostname"); !kept {
		t.Error("the hostname stopped being kept; only credentials are refused, not every setting")
	}
}

func TestAPlantedTunnelCredentialIsNotRestoredOntoAFreshlyFlashedCard(t *testing.T) {
	// The attack bean gosd-7m9y describes: something with write access to
	// /data leaves a token behind, with a digest that agrees with it
	// (unkeyed SHA-256 — anyone who can write one file can write both), and
	// waits for the owner to re-flash believing that resets the device.
	store := t.TempDir()
	plant(t, store, "ingress/cloudflared/token", "attacker-tunnel-token")
	plant(t, store, "hostname", "kitchen-pi")

	card, digests := flash(t, withIngress)
	result, tree := boot(t, store, card, "image-b", digests)

	if got := onCard(t, card, "ingress/cloudflared/token"); got != "" {
		t.Errorf("token on the freshly flashed card = %q, want nothing put back", got)
	}
	if got := tree.Get("ingress/cloudflared/token"); got != "" {
		t.Errorf("token this boot = %q, want the tunnel never opened", got)
	}
	if slices.Contains(result.Restored, "ingress/cloudflared/token") {
		t.Errorf("restored %v, want the token left out", result.Restored)
	}
	if !slices.Contains(result.Restored, "hostname") {
		t.Errorf("restored %v, want the ordinary settings still put back", result.Restored)
	}
}

func TestACredentialLeftInTheStoreByAnOlderGosdIsDeleted(t *testing.T) {
	// Upgrading is the moment the value already on /data has to go: leaving
	// it there would keep it one downgrade away from being restored.
	store := t.TempDir()
	plant(t, store, "ingress/tailscale-funnel/authkey", "tskey-auth-planted")

	card, digests := flash(t, defaults)
	boot(t, store, card, "image-a", digests)

	if got, kept := storeHolds(t, store, "ingress/tailscale-funnel/authkey"); kept {
		t.Errorf("the store still holds the authkey (%q) after a boot that saw it", got)
	}
}

func TestAStoredValueThatCouldNotBeASettingIsDroppedRatherThanPutOnTheCard(t *testing.T) {
	// A NUL makes execve(2) fail, so one planted here would stop /app
	// starting on every boot — and go on doing so through the re-flash
	// somebody performed to fix it.
	store := t.TempDir()
	plant(t, store, "env/GREETING", "hello\x00world")
	plant(t, store, "hostname", "kitchen-pi")

	card, digests := flash(t, defaults)
	result, tree := boot(t, store, card, "image-b", digests)

	if slices.Contains(result.Restored, "env/GREETING") {
		t.Errorf("restored %v, want the NUL-carrying value left out", result.Restored)
	}
	if got := tree.Get("env/GREETING"); got != "" {
		t.Errorf("env/GREETING this boot = %q, want it never applied", got)
	}
	if _, kept := storeHolds(t, store, "env/GREETING"); kept {
		t.Error("the store still holds the NUL-carrying value; it can never be legitimately restored, so it is dropped")
	}
	if !slices.Contains(result.Restored, "hostname") {
		t.Errorf("restored %v, want the settings beside it unaffected", result.Restored)
	}
}

// plant writes an entry into the store the way anything with write access
// to the data partition can: the value, and a digest that agrees with it.
// That the pair is self-consistent is the point — the digest proves the
// write finished, never who made it.
func plant(t *testing.T, store, path, value string) {
	t.Helper()
	sum := sha256.Sum256([]byte(value))
	write(t, filepath.Join(store, "values", filepath.FromSlash(path)), value)
	write(t, filepath.Join(store, "digests", filepath.FromSlash(path)), hex.EncodeToString(sum[:])+"\n")
}

func TestATunnelCredentialPlantedUnderADifferentCapitalizationIsStillRefused(t *testing.T) {
	// Both partitions can be FAT, where "Token" and "token" are one file:
	// restoring the first would write the second, which is the file the
	// device reads.
	store := t.TempDir()
	plant(t, store, "ingress/cloudflared/Token", "attacker-tunnel-token")

	card, digests := flash(t, withIngress)
	result, tree := boot(t, store, card, "image-b", digests)

	if len(result.Restored) != 0 {
		t.Errorf("restored %v, want nothing put back", result.Restored)
	}
	if got := tree.Get("ingress/cloudflared/token"); got != "" {
		t.Errorf("token this boot = %q, want the tunnel never opened", got)
	}
	if _, kept := storeHolds(t, store, "ingress/cloudflared/Token"); kept {
		t.Error("the store still holds the credential under its alternative spelling")
	}
}
