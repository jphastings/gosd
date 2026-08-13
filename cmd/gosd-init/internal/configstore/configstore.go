// Package configstore keeps a copy of a device's own settings on the data
// partition, so that overwriting the boot partition doesn't cost somebody
// the values they typed onto their card.
//
// Re-flashing writes a whole new boot partition, config tree and all, while
// the data partition survives (see docs/design/upgrade-path.md §2). This
// package is what closes that gap: every boot it records the settings that
// differ from the ones this image shipped with, and the first boot under a
// different image puts them back onto the card — where they stay visible and
// editable, in the same files their owner left them in.
//
// # What the store is
//
// A directory (Dir, under the data partition's mount point) mirroring the
// card's config tree:
//
//	values/<tree path>   the setting's value, exactly as the card reads it
//	digests/<tree path>  the SHA-256 of that value file, hex, written second
//	identity             the image identity this store was last reconciled with
//
// Presence in the store is the whole record of intent: no copy of any old
// default is kept. A setting is in the store because, on some boot, the card
// held something other than what its image shipped — and it leaves the store
// the moment the card agrees with its image again.
//
// # The two phases
//
// "Differs from what this image shipped" always means byte-for-byte, over
// the file as the card holds it, padding included: config.json carries a
// digest per value file (initcfg.Config.ConfigDigests), never a copy of the
// value, so that is the only comparison there is. It is also the right one.
// A file byte-identical to the shipped one is the shipped one — nobody can
// have meant anything else by it — while a file somebody retyped, even to
// the same words, is a file they chose.
//
// Restore runs only on a boot whose image identity differs from the one the
// store records, which is the only way to tell a freshly re-flashed card
// (every setting back at its default) from a card somebody has just put
// back to its defaults: per file, the two are byte-identical. Per setting:
// a card that differs from what this image shipped wins outright (an
// injection, or a hand-edit made before this boot, is the freshest
// statement of intent there is); a card that matches gets the stored value
// written onto it, with this image's own value written beside it as
// <name>.new when that value is non-empty and differs — Debian conffile
// semantics, so somebody can see what they are overriding.
//
// Persist runs every boot, after any cloud-init seed has been consumed into
// the tree: a setting that differs from this image's is written into the
// store, and one that matches has its entry deleted. That deletion is the
// locked rule that putting a setting back to the default *is* the default
// (epic gosd-rw6n): it is indistinguishable from one, so it is treated as
// one. Re-flashing the same image is exactly that — it restores every value
// file byte for byte — which is why an image re-flashed over its own card
// ends up with the settings it shipped rather than the ones it had. Somebody
// clearing a setting by hand instead leaves an empty file, which is not the
// file the image shipped and is kept as what it plainly is: the wish for
// that setting to be unset, honoured across the next re-flash too.
//
// A stored setting the running image has no file for at all is an orphan:
// its value is written onto the card as <name>.unused and the entry is
// dropped, giving one re-flash window to retrieve it. config/env/ is exempt
// — an app environment variable somebody created is never in any image's
// baked tree, so absence there is its normal state — which is what lets a
// customer's own env var survive re-flash after re-flash.
//
// # Crash ordering
//
// The data partition may be FAT32 (the default) or ext4, so nothing here
// may assume POSIX rename atomicity, and a file's existence is never taken
// as proof that writing it finished. Instead:
//
//   - An entry commits as value → sync → digest → sync (every write itself
//     the four-step temp-write/rename/fsync sequence of
//     cmd/gosd-init/internal/durable). An entry whose value does not hash to
//     its digest is torn — a crash in that gap, or a half-linked file a FAT
//     rename left behind — and is dropped and deleted, never trusted. That
//     costs an interrupted *update* the entry entirely, old value included;
//     it is the right trade, because the card still holds that value (the
//     card is written first, and is what the store copies) and the next
//     boot's persist pass simply records it again. The store is a copy of
//     the card, never the other way around, and it only has to be right at
//     the moment somebody re-flashes — which is an act that follows a boot.
//   - Deleting an entry removes the digest first, so a crash mid-delete
//     leaves a torn entry, which the next load drops: the same outcome the
//     delete was reaching for. Removals fsync their directory, so a power
//     cut can't resurrect them.
//   - The identity record is written LAST, after both phases have completed
//     without error, and it is the only thing that ends a restore window.
//     Written any earlier it would vouch for entries not yet on the card:
//     the next boot would see a matching identity, skip the restore, find
//     every un-restored setting equal to its baked default, and delete
//     exactly the entries that still had work to do. Written last, a crash
//     anywhere in the restore leaves the identity stale, so the next boot
//     re-runs it — safely, because a setting already restored now differs
//     from what the image baked and so is left alone. Restore is per-file
//     idempotent by construction.
//   - Deletion is additionally gated on the restore phase having completed:
//     if the card could not be written at all (a read-only card, a device
//     with no way to write to its own), no entry is deleted this boot, so a
//     restore that never landed can't be mistaken for a revert on the boot
//     after it.
//   - An orphan's <name>.unused reaches the card, durably, before its entry
//     is dropped: the store never forgets a value that isn't somewhere else
//     yet.
//   - A value that can't be READ is never treated as one that was never
//     WRITTEN. Only a value that reads back and disagrees with its digest is
//     torn; anything that merely refuses to read leaves the whole boot
//     un-reconciled — nothing deleted, no identity recorded — so a failing
//     card costs a device one boot's reconciliation rather than every
//     setting it ever had.
//
// Nothing here authenticates the store. Anything with write access to the
// data partition can put a setting on the card, exactly as anything with
// write access to the card itself can (bean gosd-7m9y tracks /data as a
// trust boundary that survives a re-flash). What this package guarantees is
// that a value it acts on was written completely, not that it was written
// by the device's owner.
//
// # Failure is never fatal
//
// A missing, unwritable or unreadable store is logged and boot continues,
// as it must: a device that won't start because of a byte on its data
// partition is far worse than one that has forgotten a setting. An image
// with no data partition has no store at all, and nothing survives a
// re-flash — the expected, documented consequence of building without
// --data-size=expand.
package configstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/cardconfig"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/durable"
	"github.com/jphastings/gosd/internal/configtree"
)

const (
	// Dir is the store's directory, relative to the data partition's mount
	// point — /data/.gosd/config on a running device.
	Dir = ".gosd/config"

	// valuesDir and digestsDir are parallel mirrors of the card's config
	// tree: a setting's value and the digest that vouches for it. Parallel
	// rather than a "<name>.sha256" sidecar beside each value because a
	// setting's name may contain periods (a value legitimately called
	// google-service-account.json), so any suffix convention is a name a
	// real setting could have.
	valuesDir  = "values"
	digestsDir = "digests"

	// identityFile records the image identity the store was last fully
	// reconciled with. See the package doc: it is written last, and is what
	// makes "a freshly flashed card" and "a card edited back to its
	// defaults" tell apart.
	identityFile = "identity"

	// envPrefix is the one part of the tree where a stored setting missing
	// from the running image's baked tree is ordinary rather than an
	// orphan: app environment variables are the customer's namespace, and
	// one they created was never in any image.
	envPrefix = "env/"
)

// Deps is everything the store touches outside its own directory.
type Deps struct {
	// Dir is the store's directory on the mounted data partition. Empty
	// disables the store entirely.
	Dir string

	// EditBoot runs edit against the root of the normally read-only boot
	// partition, with everything it wrote made durable before the
	// read-only mount is restored (see boot.Deps.EditBoot). Nil means this
	// device can't write to its own card: settings still take effect for
	// the boot in progress, but nothing is restored onto the card and — so
	// that a restore which never landed can't read as a revert — nothing
	// is deleted from the store either.
	EditBoot func(edit func(root string) error) error

	// Log is where every decision is narrated. Values are never logged:
	// any of them may be a secret. Paths are.
	Log func(format string, args ...any)
}

// Options is what the running image says about itself.
type Options struct {
	// Identity is the running image's identity (initcfg.Config.Identity).
	// Empty — an image built before that field existed — makes a
	// re-flash undetectable, so settings are still kept but never
	// restored.
	Identity string

	// Baked is the SHA-256 of every value file this image shipped, keyed
	// by tree path (initcfg.Config.ConfigDigests). It is how a card's
	// value is told from the image's own without keeping a copy of every
	// default: a path that isn't in this map is one this image has no
	// setting for.
	Baked map[string]string
}

// Result is what a reconcile did that the rest of the boot has to know
// about.
type Result struct {
	// Restored names every setting whose value this boot put back onto the
	// card, sorted. Anything already resolved from the tree by this point
	// in the boot sequence (the hostname) has to be resolved again when it
	// appears here.
	Restored []string
}

// Reconcile brings the card's settings and the store into agreement: it
// restores what a re-flash wiped (only when the running image differs from
// the one the store was written under), and records what the card now says
// for the next re-flash to restore. config is updated in place with
// anything restored, so the boot in progress acts on the settings it just
// put back rather than on the defaults it found.
//
// It never returns an error. Everything it does is best-effort by design —
// see the package doc.
func Reconcile(deps Deps, config cardconfig.Tree, opts Options) Result {
	if deps.Dir == "" {
		return Result{}
	}

	stored, identity, readable := load(deps.Dir, deps.Log)

	var result Result
	// reconciled is both "the restore phase left nothing undone" and, in
	// consequence, "a setting matching this image's own value can be
	// trusted to be a revert rather than something not yet restored" — the
	// gate on persist's deletions, and on recording this image's identity.
	reconciled := true
	switch {
	case !readable:
		// The store is there but wouldn't read. Nothing can be classified
		// against what it holds, and nothing may be forgotten on the
		// strength of what it appears not to: this boot records what the
		// card says and leaves everything else for a boot that can read it.
		reconciled = false
	case opts.Identity == "":
		deps.Log("this image doesn't say which build it is, so a re-flashed card can't be told from an edited one; settings are kept on the data partition but never restored")
		reconciled = false
	case identity == opts.Identity:
		// The store was last reconciled with the image now running, so the
		// card is already the settled record of what this device has been
		// told — a value back at its default is a revert, and persist below
		// forgets it.
	case len(opts.Baked) == 0:
		// An image with no config tree has no defaults to compare a card
		// against, so nothing here can be classified. Nothing is restored,
		// and (since no setting can equal a default that doesn't exist)
		// nothing is deleted either.
		reconciled = false
	default:
		result.Restored, reconciled = restore(deps, config, opts, stored)
	}

	kept := persist(deps, config, opts, stored, reconciled)
	if reconciled && kept && identity != opts.Identity {
		stamp(deps, opts.Identity)
	}
	return result
}

// sidecar is a file written beside a setting on the card to explain what
// happened to it: <name>.new (this image's own value, which a restore
// overrode) or <name>.unused (a value this image has no setting for).
// Neither is a setting — the device reads past both (see
// configtree.IgnoredName) and the build refuses either as a name.
type sidecar struct {
	path  string
	value string
}

// plan is what the restore phase decided, before any of it is carried out.
type plan struct {
	// values are the settings to write onto the card, keyed by tree path.
	values map[string]string
	// sidecars are the .new/.unused files to write beside them.
	sidecars []sidecar
	// orphans are the stored settings to drop once their .unused file is
	// durably on the card.
	orphans []string
	// kept names the settings the card itself won, for the log alone.
	kept []string
}

func (p plan) empty() bool { return len(p.values) == 0 && len(p.sidecars) == 0 }

// restore puts back what a re-flash wiped. It returns the settings it wrote
// onto the card and whether the phase completed — false leaves the store's
// identity stale, so the next boot tries again, and blocks persist's
// deletions for this boot.
func restore(deps Deps, config cardconfig.Tree, opts Options, stored map[string]string) ([]string, bool) {
	p := restorePlan(stored, config, opts.Baked)
	for _, path := range p.kept {
		deps.Log("%s was changed since this image was flashed, so the card's own value stands", cardconfig.OnCard(path))
	}
	// Nothing to write means nothing to drop either: an orphan is only ever
	// forgotten once its .unused file is on the card, and that file is one
	// of the writes this plan would have made.
	if p.empty() {
		return nil, true
	}

	// The settings take effect this boot whether or not they reach the
	// card: a device whose card has gone read-only shouldn't also lose the
	// network it was just told to join. Applying them here, before the
	// write is even attempted, is also what stops persist below from
	// reading an unwritten card as somebody having put every one of them
	// back to its default by hand.
	for path, value := range p.values {
		config.Set(path, value)
	}

	if deps.EditBoot == nil {
		deps.Log("this device can't write to its own card, so its kept settings apply to this boot only")
		return sortedKeys(p.values), false
	}

	// One edit window for the whole phase: each remount read-write is a
	// window in which a power cut can damage the boot FAT, so the fewer
	// the better.
	err := deps.EditBoot(func(root string) error {
		dir := filepath.Join(root, configtree.Dir)
		errs := []error{config.Write(dir, p.values)}
		for _, s := range p.sidecars {
			errs = append(errs, writeSidecar(dir, s))
		}
		return errors.Join(errs...)
	})
	if err != nil {
		deps.Log("putting this device's kept settings back onto the card failed; they apply to this boot, and the next boot will try again: %v", err)
		return sortedKeys(p.values), false
	}

	restored := sortedKeys(p.values)
	for _, path := range restored {
		deps.Log("%s restored from the copy kept on the data partition", cardconfig.OnCard(path))
	}
	for _, s := range p.sidecars {
		deps.Log("%s holds what this image would have used", cardconfig.OnCard(s.path))
	}

	// Only now that every .unused file is durably on the card may the
	// store forget the values they hold.
	dropOrphans(deps, p.orphans, stored)
	return restored, true
}

// restorePlan applies the per-setting restore rules (epic gosd-rw6n) to one
// boot's store and card. Pure — it reads and writes nothing — so the rules
// can be tested as rules.
func restorePlan(stored map[string]string, config cardconfig.Tree, baked map[string]string) plan {
	p := plan{values: map[string]string{}}

	for _, path := range sortedKeys(stored) {
		value := stored[path]
		bakedDigest, isBaked := baked[path]
		card, onCard := config[path]

		switch {
		case onCard && !isBaked:
			// A file the image doesn't ship but the card carries — an app
			// environment variable, or one somebody made themselves. There
			// is nothing to compare it against, so what is on the card is
			// the newer statement of it, and it is no orphan: it is right
			// there on the card already.
			p.kept = append(p.kept, path)
		case !isBaked && !strings.HasPrefix(path, envPrefix):
			// This image has no such setting any more, and the card doesn't
			// carry it either. Hand the value back on the card and forget
			// it: one re-flash to collect it.
			p.orphans = append(p.orphans, path)
			p.sidecars = append(p.sidecars, sidecar{path: path + configtree.UnusedSuffix, value: value})
		case onCard && card.SHA256() != bakedDigest:
			// Changed since this image was flashed — injected into the
			// downloaded .img, or hand-edited before this boot. The freshest
			// intent there is.
			p.kept = append(p.kept, path)
		case onCard && card.Value == value:
			// The card already reads as the stored value (its padding may
			// differ, which is the image's business, not the setting's).
			// Writing would only cost a remount.
		default:
			p.values[path] = value
			// The .new copy is this image's own value, which is exactly what
			// the card still holds at this point: the case above proved the
			// two identical. Empty defaults — the shape every secret-like
			// setting ships in — leave no .new at all, so the card stays
			// quiet about the settings nobody set.
			if onCard && card.Value != "" && card.Value != value {
				p.sidecars = append(p.sidecars, sidecar{path: path + configtree.NewSuffix, value: card.Value})
			}
		}
	}
	return p
}

// dropOrphans forgets the settings this image has no file for, now their
// values are on the card. A failure is logged and the entry left alone: it
// will be offered again, on the card, after the next re-flash.
func dropOrphans(deps Deps, orphans []string, stored map[string]string) {
	for _, path := range orphans {
		if err := deleteEntry(deps.Dir, path); err != nil {
			deps.Log("dropping the kept copy of %s failed: %v", cardconfig.OnCard(path), err)
			continue
		}
		delete(stored, path)
		deps.Log("%s isn't a setting in this image; its value is on the card as %s, and is no longer kept", cardconfig.OnCard(path), cardconfig.OnCard(path+configtree.UnusedSuffix))
	}
}

// persist records what the card now says, so the next re-flash can put it
// back: a setting that differs from this image's own is written into the
// store, and one that matches has its entry deleted — the locked rule that
// an edit back to the default is the default.
//
// Deletions only happen when the boot reconciled cleanly (see Reconcile):
// on any other boot a setting equal to its default may simply be one the
// restore phase hasn't managed to put back yet.
//
// A setting the card doesn't carry at all is neither: cardconfig skips a
// file it can't read, so a transient read error would otherwise be enough
// to forget somebody's value. To drop a kept env var, empty its file rather
// than deleting it.
//
// It returns whether the store is in the state this boot intended, which is
// what decides whether the image's identity may be recorded.
func persist(deps Deps, config cardconfig.Tree, opts Options, stored map[string]string, allowDelete bool) bool {
	if err := os.MkdirAll(filepath.Join(deps.Dir, valuesDir), 0o755); err != nil {
		deps.Log("this device's settings can't be kept for the next re-flash (%s isn't writable): %v", deps.Dir, err)
		return false
	}

	ok := true
	for _, path := range config.Paths() {
		value := config[path]
		bakedDigest, isBaked := opts.Baked[path]

		if isBaked && value.SHA256() == bakedDigest {
			if _, have := stored[path]; !have || !allowDelete {
				continue
			}
			if err := deleteEntry(deps.Dir, path); err != nil {
				deps.Log("forgetting the kept copy of %s failed: %v", cardconfig.OnCard(path), err)
				ok = false
				continue
			}
			delete(stored, path)
			deps.Log("%s matches this image's own value again, so it is no longer kept for the next re-flash", cardconfig.OnCard(path))
			continue
		}

		if existing, have := stored[path]; have && existing == value.Value {
			continue
		}
		if err := writeEntry(deps.Dir, path, value.Value); err != nil {
			deps.Log("keeping %s for the next re-flash failed: %v", cardconfig.OnCard(path), err)
			ok = false
			continue
		}
		stored[path] = value.Value
	}

	if len(stored) > 0 {
		deps.Log("kept for the next re-flash: %s", strings.Join(onCardPaths(sortedKeys(stored)), ", "))
	}
	return ok
}

// stamp records the image identity the store now agrees with. It is the
// store's commit record, and so is written after everything it vouches for
// — see the package doc's crash ordering.
func stamp(deps Deps, identity string) {
	if err := durable.WriteFile(filepath.Join(deps.Dir, identityFile), []byte(identity+"\n")); err != nil {
		deps.Log("recording which image these kept settings belong to failed; the next boot will reconcile them again: %v", err)
	}
}

// load reads the store, healing it as it goes: a torn entry — one whose
// value doesn't hash to its digest, including one whose digest never
// arrived — is deleted rather than trusted, as is the temporary file an
// interrupted write leaves behind.
//
// A store that isn't there yet is no error: it reads as empty, which is
// what every device starts out as. One that is there and won't read is
// reported (ok=false) rather than passed off as empty, because the two mean
// opposite things: a store somebody's settings aren't in yet, versus one
// they may well be in. Treating the second as the first would let a single
// unlucky read turn a freshly re-flashed card into "every setting was put
// back by hand", and forget the lot.
func load(dir string, log func(format string, args ...any)) (map[string]string, string, bool) {
	stored := map[string]string{}

	readable := true
	root := filepath.Join(dir, valuesDir)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			log("something under %s can't be read, so this boot leaves the settings kept there alone: %v", root, err)
			readable = false
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root || entry.IsDir() {
			return nil
		}

		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		if strings.HasPrefix(entry.Name(), ".") {
			// durable.WriteFile's temporary name, left by a power cut
			// between the write and the rename. Never a setting.
			_ = removeDurably(path)
			return nil
		}

		switch value, state := readEntry(dir, rel, path, entry); state {
		case entryTorn:
			log("the kept copy of %s wasn't finished being written, so it is dropped", cardconfig.OnCard(rel))
			if err := deleteEntry(dir, rel); err != nil {
				log("dropping it failed, and it will be dropped again next boot: %v", err)
			}
		case entryUnreadable:
			log("the kept copy of %s can't be read, so this boot leaves the settings kept on the data partition alone", cardconfig.OnCard(rel))
			readable = false
		default:
			stored[rel] = value
		}
		return nil
	})
	if err != nil {
		if !os.IsNotExist(err) {
			log("reading the settings kept on the data partition failed, so this boot leaves them alone: %v", err)
			return map[string]string{}, "", false
		}
		// No store yet: every device's first boot, and every image built
		// without a data partition to keep one on.
		return stored, "", true
	}

	identity, ok := readIdentity(dir)
	return stored, identity, readable && ok
}

// entryState is what one file under values/ turned out to be. The middle
// case is the whole point of keeping three: a value that can't be READ is
// not a value that was never fully WRITTEN, and treating the first as the
// second would let one unlucky read delete somebody's setting.
type entryState int

const (
	entryOK entryState = iota
	entryTorn
	entryUnreadable
)

// readEntry reads one stored value and the digest vouching for it. It is
// torn when the digest disagrees, when it never arrived at all, or when
// what's there could not be a setting in the first place — a device node or
// symlink somebody put in the store, or a file far larger than any card
// could hold (cardconfig.MaxValueBytes), which on a device whose entire
// root filesystem is RAM is worth refusing outright.
func readEntry(dir, rel, path string, entry fs.DirEntry) (string, entryState) {
	info, err := entry.Info()
	if err != nil {
		return "", entryUnreadable
	}
	if !info.Mode().IsRegular() || info.Size() > cardconfig.MaxValueBytes {
		return "", entryTorn
	}

	value, err := os.ReadFile(path)
	if err != nil {
		return "", entryUnreadable
	}
	digest, err := os.ReadFile(filepath.Join(dir, digestsDir, filepath.FromSlash(rel)))
	switch {
	case os.IsNotExist(err):
		return "", entryTorn
	case err != nil:
		return "", entryUnreadable
	case strings.TrimSpace(string(digest)) != digestOf([]byte(value)):
		return "", entryTorn
	}
	return string(value), entryOK
}

// readIdentity reads the image identity the store was last reconciled with.
// A store that has never been stamped answers "" — which no image identity
// equals, so it reads as one belonging to another image and is restored
// from, the safe direction. One that won't read reports ok=false instead, so
// that a boot which can't tell a re-flash from a hand-revert does neither.
func readIdentity(dir string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, identityFile))
	switch {
	case os.IsNotExist(err):
		return "", true
	case err != nil:
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

// writeEntry commits one setting: the value, then the digest that vouches
// for it, each durably. The order is the whole crash-safety argument — see
// the package doc.
func writeEntry(dir, path, value string) error {
	valuePath := filepath.Join(dir, valuesDir, filepath.FromSlash(path))
	digestPath := filepath.Join(dir, digestsDir, filepath.FromSlash(path))

	for _, target := range []string{valuePath, digestPath} {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("making room for it on the data partition: %w", err)
		}
	}
	if err := durable.WriteFile(valuePath, []byte(value)); err != nil {
		return err
	}
	return durable.WriteFile(digestPath, []byte(digestOf([]byte(value))+"\n"))
}

// deleteEntry forgets one setting, digest first: until the value goes too
// the entry reads as torn, which the next load drops — the same outcome
// this delete is reaching for, whenever the power goes in between.
func deleteEntry(dir, path string) error {
	if err := removeDurably(filepath.Join(dir, digestsDir, filepath.FromSlash(path))); err != nil {
		return err
	}
	return removeDurably(filepath.Join(dir, valuesDir, filepath.FromSlash(path)))
}

// removeDurably deletes a file and fsyncs the directory that held it, so
// that a power cut immediately afterwards can't bring it back: on FAT a
// directory block stays dirty for the kernel's full writeback expiry, and
// on ext4 the journal covers the metadata only once it's committed. A file
// that was already gone is not an error.
func removeDurably(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

// writeSidecar writes a .new or .unused file into the card's config tree,
// creating whatever directory it belongs in — an orphaned setting's
// directory may not exist in the new image at all (a feature the new build
// left out takes its whole directory with it). Padding is deliberately not
// applied: neither file is a setting, so neither reserves anything.
func writeSidecar(dir string, s sidecar) error {
	target := filepath.Join(dir, filepath.FromSlash(s.path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("making room for %s: %w", cardconfig.OnCard(s.path), err)
	}
	if err := durable.WriteFile(target, []byte(s.value+"\n")); err != nil {
		return fmt.Errorf("writing %s: %w", cardconfig.OnCard(s.path), err)
	}
	return nil
}

// digestOf is the hex SHA-256 a stored value is vouched for by.
func digestOf(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// onCardPaths names settings the way somebody looking at the card sees
// them, for a log line listing several at once.
func onCardPaths(paths []string) []string {
	named := make([]string, 0, len(paths))
	for _, path := range paths {
		named = append(named, cardconfig.OnCard(path))
	}
	return named
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
