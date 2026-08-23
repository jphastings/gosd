// Package cardconfig reads — and writes back — the config/ tree a gosd
// image carries at the root of its boot partition: one setting per file,
// read newline-trimmed, an empty file meaning "not set". The format itself,
// and the build side that assembles a tree, are internal/configtree's.
//
// The tree is the single source of truth for what a device has been told to
// do; config.json's baked values are only the per-field fallback for a
// setting the card leaves unset. That makes reading it deliberately
// forgiving: this is a directory people edit by hand, from whichever
// computer their card fits into, so a card can carry files gosd never wrote
// (an editor's backup, a macOS AppleDouble companion) and can be missing
// files gosd did. Anything unreadable is skipped and the baked default
// stands — a device that refuses to boot over a stray file would be far
// worse than one setting nobody set.
package cardconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/durable"
	"github.com/jphastings/gosd/internal/configtree"
)

// MaxValueBytes caps how much of a single file this will read as a setting.
// A setting is a line or two somebody typed, and the largest reservation
// gosd ships is a kilobyte, so a file past this size is not one: reading it
// would only let something that landed in config/ by accident — a photo, a
// log, a video — exhaust the memory of a device whose entire root
// filesystem is RAM.
const MaxValueBytes = 64 * 1024

// MaxTreeBytes caps what a whole tree may cost, which MaxValueBytes on its
// own does not: the boot partition is a FAT volume of a couple of hundred
// megabytes that anyone holding the card can fill with settings, and every
// one of them would otherwise be read and kept. The device it would be read
// on is PID 1 with its root filesystem in RAM, and Linux does not kill init
// to reclaim memory — it panics, and the card still holds the same files on
// the next boot.
//
// Every setting costs at least its reservation (configtree.MinValueBytes),
// which is what makes this a bound on how many settings there are as well
// as on how big they get: 1 MiB is room for four thousand of them, where
// gosd's own tree ships eleven and an app's --boot-config-dir adds its own
// handful.
const MaxTreeBytes = 1024 * 1024

// MaxDepth caps how deeply Read will descend into a tree. gosd's own
// deepest setting is three levels down (ingress/cloudflared/token), so this
// is generous for anything an app adds. It is here because the walk is
// recursive and the directories it walks are on a FAT volume written by
// whoever holds the card: a directory entry pointing back at an ancestor's
// cluster is a tree with no bottom, and a boot that never ends is as
// effective a brick as one that panics.
const MaxDepth = 8

// Tree is a card's config/ directory in memory: every setting keyed by its
// path within the tree ("wifi/ssid", "env/API_TOKEN"). Documentation
// sidecars, the .new/.unused files the device writes itself, and
// operating-system metadata are not settings and never appear here (see
// configtree.IgnoredName).
//
// Its values are configtree.Value — the same type the build side produced —
// so each carries both the bytes the card holds (padding included, which is
// what a digest is taken over) and the setting they read as.
type Tree map[string]configtree.Value

// Get returns the setting at path, newline-trimmed. A file that isn't
// there and one holding nothing but its padding both answer "" — unset is
// unset, however the card came to be that way.
func (t Tree) Get(path string) string { return t[path].Value }

// Paths returns every setting's path, sorted, so anything iterating the
// tree (a log line, a store's persist pass) does so in a stable order.
func (t Tree) Paths() []string {
	paths := make([]string, 0, len(t))
	for p := range t {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// Group returns the settings directly inside dir, keyed by their own names
// rather than their full paths — "env" yields the app's environment
// variables by variable name. Only settings that are actually set are
// included: an empty file means unset, which for a whole group means the
// name simply isn't in the map.
func (t Tree) Group(dir string) map[string]string {
	prefix := dir + "/"
	group := make(map[string]string)
	for path, value := range t {
		name, ok := strings.CutPrefix(path, prefix)
		if !ok || strings.Contains(name, "/") || value.Value == "" {
			continue
		}
		group[name] = value.Value
	}
	return group
}

// Read reads the config tree rooted at dir — the config/ directory of the
// mounted boot partition. A tree that isn't there at all reads as an empty
// tree rather than an error, leaving every setting to fall back to the
// values this image was built with.
//
// The walk is bounded in every direction a card can grow it: how big one
// setting may be (MaxValueBytes), how much the tree may cost in total
// (MaxTreeBytes), and how far down it goes (MaxDepth). Reaching a bound
// stops the walk and leaves every setting not yet read at its baked value,
// which is the same outcome as a card that never carried them.
func Read(dir string, log func(format string, args ...any)) Tree {
	tree := Tree{}
	budget := MaxTreeBytes

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			log("skipping %s, it can't be read: %v", OnCard(treePath(dir, path)), err)
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == dir {
			return nil
		}

		if configtree.IgnoredName(entry.Name()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel := treePath(dir, path)
		if entry.IsDir() {
			if strings.Count(rel, "/")+1 >= MaxDepth {
				log("not looking inside %s: settings are never nested this deeply", OnCard(rel))
				return fs.SkipDir
			}
			return nil
		}

		value, ok := readValue(dir, path, entry, log)
		if !ok {
			return nil
		}
		spend := cost(value)
		if spend > budget {
			log("stopping at %s: the settings on this card add up to more than the %d bytes gosd will read, so the rest keep the values this image was built with", OnCard(rel), MaxTreeBytes)
			return fs.SkipAll
		}
		budget -= spend
		tree[value.Path] = value
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			log("no %s directory on the boot partition; using the settings this image was built with", configtree.Dir)
		} else {
			log("reading the %s directory on the boot partition failed, using the settings this image was built with: %v", configtree.Dir, err)
		}
	}

	return tree
}

// cost is what one setting is charged against Read's budget: its bytes, or
// the reservation every value file holds open (configtree.MinValueBytes),
// whichever is larger. Charging the floor is what stops a tree of empty
// files — which cost nothing to hold but plenty to walk and to key — from
// being free.
func cost(value configtree.Value) int {
	return max(len(value.Content), configtree.MinValueBytes)
}

// readValue reads one setting file, reporting ok=false for anything that
// isn't a setting a person could have typed: a device node or symlink
// somebody put in the tree, a file too big to be a value (see
// MaxValueBytes), one holding a NUL byte (see configtree.PlausibleValue),
// or one that won't read at all.
func readValue(dir, path string, entry fs.DirEntry, log func(format string, args ...any)) (configtree.Value, bool) {
	rel := treePath(dir, path)

	info, err := entry.Info()
	if err != nil {
		log("skipping %s, it can't be read: %v", OnCard(rel), err)
		return configtree.Value{}, false
	}
	if !info.Mode().IsRegular() {
		log("skipping %s: a setting has to be an ordinary file", OnCard(rel))
		return configtree.Value{}, false
	}
	if info.Size() > MaxValueBytes {
		log("skipping %s: it holds %d bytes, far more than any setting (%d at most)", OnCard(rel), info.Size(), MaxValueBytes)
		return configtree.Value{}, false
	}

	content, err := os.ReadFile(path)
	if err != nil {
		log("skipping %s, it can't be read: %v", OnCard(rel), err)
		return configtree.Value{}, false
	}
	if !configtree.PlausibleValue(content) {
		log("skipping %s: it holds a NUL byte, which is not something anybody typed into a settings file and which nothing this value is handed to can carry", OnCard(rel))
		return configtree.Value{}, false
	}
	return configtree.Value{Path: rel, Content: content, Value: configtree.TrimValue(content)}, true
}

// Write writes settings into the tree rooted at dir — keyed by tree path,
// exactly as Read returns them — creating any directory the path names
// along the way, and updates tree so this boot acts on what was written
// whether or not the card accepted it (a card that has gone read-only under
// a failing reader must not also cost the device its settings).
//
// A value is padded with trailing newlines out to the reservation its file
// already holds, or to configtree.MinValueBytes for a file that isn't there
// yet, for the same reason the build pads: the padding IS the reservation
// (see internal/configtree's package doc), so writing a short value over a
// long one would silently shrink a region published for injection, and
// whoever opens the file next sees the blank lines they saw before rather
// than a file that changed shape under them.
//
// Every write goes through durable.WriteFile: a setting that reaches the
// card only to vanish on the next power cut is worse than one that was
// never written, and the card is a FAT filesystem with no journal to lean
// on.
func (t Tree) Write(dir string, values map[string]string) error {
	var errs []error
	for _, path := range sortedKeys(values) {
		if err := writeValue(dir, path, pad(values[path], t[path].Content)); err != nil {
			errs = append(errs, err)
		}
		t.Set(path, values[path])
	}
	return errors.Join(errs...)
}

// Set records a setting in memory alone, in the same padded form Write
// would have put on the card: what a device does with a setting it has been
// given but has no way to write down.
func (t Tree) Set(path, value string) {
	content := pad(value, t[path].Content)
	t[path] = configtree.Value{Path: path, Content: content, Value: configtree.TrimValue(content)}
}

func writeValue(dir, path string, content []byte) error {
	target := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("making room for the setting %s: %w", path, err)
	}
	if err := durable.WriteFile(target, content); err != nil {
		return fmt.Errorf("writing the setting %s: %w", path, err)
	}
	return nil
}

// pad renders value as the bytes its file holds: the value itself, then
// trailing newlines out to whatever the file already reserved (existing),
// and never fewer than configtree.MinValueBytes.
func pad(value string, existing []byte) []byte {
	content := []byte(value + "\n")
	size := max(len(content), len(existing), configtree.MinValueBytes)

	padded := make([]byte, size)
	copy(padded, content)
	for i := len(content); i < size; i++ {
		padded[i] = '\n'
	}
	return padded
}

// treePath is path's location within the tree rooted at dir, forward-slash
// separated: the key a Tree holds it under, and the path config.json's
// digests and an injection manifest both name it by.
func treePath(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// OnCard names a setting the way somebody looking at the card would see it,
// so a log line points at a file they can actually go and open:
// "config/wifi/ssid", not the "/boot"-prefixed path only gosd-init knows.
func OnCard(path string) string { return configtree.Dir + "/" + path }

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
