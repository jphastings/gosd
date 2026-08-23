// Package configtree assembles the config/ directory `gosd build` writes to
// the FAT root of every image's boot partition: one setting per file, read
// newline-trimmed, with an empty file meaning "not set".
//
// A tree is gosd's own checked-in defaults (the embedded defaults directory
// beside this file) overlaid, file by file, with the app's own directory
// (`gosd build --boot-config-dir`): the app's file wins, and any explanation it
// doesn't override is inherited from gosd's. Every value file must be
// documented by a <name>.explain.md sidecar, its own or inherited, and each
// directory may carry a group explain.md - the build refuses a value with no
// explanation, because those files are the only documentation the person
// holding the card will ever see.
//
// Padding is the reservation: a value file is written padded with trailing
// newlines to at least MinValueBytes, and one shipped larger reserves its
// own size (gosd's own ingress/cloudflared/token ships as newlines alone for
// exactly this reason - it reads as unset, but a Cloudflare tunnel token
// fits in the space it holds open). Those bytes are what a provisioning tool
// overwrites in a downloaded .img, so a value's reservation is fixed at
// build time and can never grow afterwards. explain.md files are never
// padded and never injectable.
package configtree

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// defaults is gosd's own config tree, shipped inside the gosd binary and
// used as the base every app's --boot-config-dir overlays onto.
//
//go:embed defaults
var defaults embed.FS

const (
	// Dir is the tree's directory name at the FAT root of the boot
	// partition. A tree path ("wifi/ssid") joined onto it is the file's
	// path on the card ("config/wifi/ssid").
	Dir = "config"

	// MinValueBytes is the smallest reservation any value file gets: a
	// value shorter than this is padded with trailing newlines up to it,
	// and one shipped longer reserves its own size instead.
	MinValueBytes = 256

	// GroupDoc is the file name documenting a whole directory.
	GroupDoc = "explain.md"

	// DocSuffix marks a single value's documentation sidecar:
	// "wifi/ssid" is documented by "wifi/ssid.explain.md".
	DocSuffix = ".explain.md"

	// NewSuffix marks a copy of a setting's current default, written onto
	// the card beside a value the device restored from its own store, so
	// whoever holds the card can see what they are overriding. Reserved:
	// the build refuses a setting named this way and the device never
	// reads one back as a value.
	NewSuffix = ".new"

	// UnusedSuffix marks a value the device kept but this image has no
	// setting for any more, written onto the card so it can be retrieved
	// before it's dropped. Reserved on the same terms as NewSuffix.
	UnusedSuffix = ".unused"

	// envDir is the directory whose value names are app environment
	// variables rather than free-form setting names, so they're held to a
	// POSIX environment name's shape and to gosd's GOSD_* reservation.
	envDir = "env"
)

// Features says which optional parts of an image this build actually
// contains. A feature's directory is written only when the feature is
// present - board capability AND build flags - so a card never documents a
// setting the device it came from could not possibly act on.
type Features struct {
	// IngressCloudflared writes config/ingress/cloudflared/: `gosd build
	// --ingress cloudflared`, and only for a board whose architecture
	// cloudflared actually ships for (it is arm64-only, so never
	// pi-zero-w).
	IngressCloudflared bool

	// IngressTailscaleFunnel writes config/ingress/tailscale-funnel/:
	// `gosd build --ingress tailscale-funnel`, which every board's
	// architecture supports.
	IngressTailscaleFunnel bool
}

// Value is one setting file in a built tree.
type Value struct {
	// Path is the file's path within the tree, forward-slash separated
	// and relative to Dir, e.g. "wifi/ssid" or "env/API_TOKEN".
	Path string

	// Content is the file's bytes exactly as written to the card: the
	// shipped content padded with trailing newlines to its reservation
	// (see the package doc).
	Content []byte

	// Value is the setting this file reads as, newline-trimmed. Empty
	// means unset, which is what every padding-only file reads as.
	Value string
}

// SHA256 is the hex-encoded digest of v.Content: the bytes on the card, not
// the trimmed value. It's what config.json records per value file, and what
// an injection manifest publishes so a client can prove a region is still
// pristine before overwriting it.
func (v Value) SHA256() string {
	sum := sha256.Sum256(v.Content)
	return hex.EncodeToString(sum[:])
}

// Doc is one documentation file in a built tree: a value's
// <name>.explain.md sidecar or a directory's explain.md. Written verbatim,
// never padded.
type Doc struct {
	// Path is the file's path within the tree, as Value.Path is.
	Path string
	// Content is the markdown, exactly as authored.
	Content []byte
}

// Tree is one board's assembled config directory, sorted by path.
type Tree struct {
	Values []Value
	Docs   []Doc
}

// BootFiles returns every file in t keyed by its path on the boot partition
// (Dir joined onto each Path), ready to merge into the image's FAT-root
// contents.
func (t Tree) BootFiles() map[string][]byte {
	files := make(map[string][]byte, len(t.Values)+len(t.Docs))
	for _, v := range t.Values {
		files[path.Join(Dir, v.Path)] = v.Content
	}
	for _, d := range t.Docs {
		files[path.Join(Dir, d.Path)] = d.Content
	}
	return files
}

// Digests returns each value file's SHA-256, keyed by tree path - the map
// config.json carries so the device can tell a hand-edited (or injected)
// value from the one this image was built with, without keeping a copy of
// every default.
func (t Tree) Digests() map[string]string {
	digests := make(map[string]string, len(t.Values))
	for _, v := range t.Values {
		digests[v.Path] = v.SHA256()
	}
	return digests
}

// TrimValue reads a value file's bytes the way the device does: newline
// trimmed, so a file's trailing padding (and any editor's parting newline)
// never becomes part of the value. Exported so the build and runtime sides
// can't drift on what "the value" is.
func TrimValue(content []byte) string {
	return strings.Trim(string(content), "\r\n")
}

// entry is one file on its way into a Tree, remembered with where it came
// from so a refusal can name the file a developer has to go and fix.
type entry struct {
	content []byte
	source  string
}

const defaultsSource = "gosd's built-in config defaults"

// Build assembles the tree for one board: gosd's defaults, overlaid with
// overlayDir (empty for none), validated, pruned to features, and padded.
func Build(overlayDir string, features Features) (Tree, error) {
	files, err := loadDefaults()
	if err != nil {
		return Tree{}, err
	}
	if overlayDir != "" {
		if err := applyOverlay(files, overlayDir); err != nil {
			return Tree{}, err
		}
	}
	if err := validate(files); err != nil {
		return Tree{}, err
	}
	prune(files, features)
	return assemble(files), nil
}

func loadDefaults() (map[string]entry, error) {
	files := make(map[string]entry)
	err := fs.WalkDir(defaults, "defaults", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := defaults.ReadFile(p)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(p, "defaults/")] = entry{content: content, source: defaultsSource}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading gosd's built-in config defaults failed: %w; this is a bug in gosd itself, please report it", err)
	}
	return files, nil
}

// applyOverlay reads every file under dir into files, replacing any default
// of the same path. Names are checked as they're read, so a stray editor or
// operating-system file (a .DS_Store, a Thumbs.db, an AppleDouble ._sidecar)
// is refused where the developer can still delete it - on the card those
// names are indistinguishable from settings, so the device ignores them and
// the build has to be the thing that objects.
func applyOverlay(files map[string]entry, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("reading the config directory %s failed: %w; pass --boot-config-dir <dir> pointing at a directory of setting files, or drop the flag to use gosd's defaults alone", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("the config directory %s is a file, not a directory; --boot-config-dir takes a directory holding one file per setting", dir)
	}

	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("reading %s failed: %w", p, err)
		}
		if p == dir {
			return nil
		}

		rel := filepath.ToSlash(strings.TrimPrefix(p, dir+string(filepath.Separator)))
		if err := checkName(d.Name(), rel, dir); err != nil {
			if d.IsDir() {
				return fmt.Errorf("%w; remove that directory from %s", err, dir)
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file; every entry under --boot-config-dir %s must be a plain file holding one setting's value, or a .explain.md documenting one", p, dir)
		}

		content, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading the config file %s failed: %w", p, err)
		}
		files[rel] = entry{content: content, source: p}
		return nil
	})
}

// nameShape is what each path segment of a setting (or of a directory
// holding settings) must look like. Periods are allowed - a setting can
// legitimately be called google-service-account.json - but nothing that
// would need quoting or escaping on the way to a manifest, a card, or a
// person's terminal.
var nameShape = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// junkNames are operating-system metadata files that turn up inside a
// directory a developer curates by hand.
var junkNames = []string{"thumbs.db", "desktop.ini"}

// IgnoredName reports whether a name in a config tree is something the
// DEVICE reads past rather than reads as a setting: a documentation
// sidecar, one of the .new/.unused files the device writes onto the card
// itself, a dot-file (macOS AppleDouble companions included), or an
// operating system's own metadata. It is the runtime half of checkName,
// which refuses every one of these outright — so an app can never ship one
// — while the device merely ignores them, because a card is edited by hand
// on machines that write such files unbidden.
func IgnoredName(name string) bool {
	switch {
	case name == GroupDoc, strings.HasSuffix(name, DocSuffix):
		return true
	case strings.HasPrefix(name, "."):
		return true
	case strings.HasSuffix(name, NewSuffix), strings.HasSuffix(name, UnusedSuffix):
		return true
	}
	return isJunkName(name)
}

// ValidEnvName reports whether name has the shape of an environment
// variable's name — the rule checkEnvValue enforces at build time, exposed
// so a name that reached a running device by some other route is held to
// the identical one. A card is hand-edited and the settings kept on the
// data partition are not authenticated, so neither has been through the
// build's gate; gosd-init applies this to both (see boot's mergeUserEnv).
//
// It says nothing about the GOSD_* namespace gosd reserves: that is a
// separate refusal, with its own explanation, at both call sites.
func ValidEnvName(name string) bool { return envNameShape.MatchString(name) }

// PlausibleValue reports whether content could be a setting somebody typed
// into a file. The only thing it refuses is a NUL byte, which no text
// editor writes and no sink gosd hands a value to can carry: a NUL in an
// app environment variable makes execve(2) fail with EINVAL, so a single
// one planted in the copy kept on the data partition would stop /app
// starting on every boot, and go on doing so through the re-flash somebody
// performed to fix it (bean gosd-7m9y).
//
// It is deliberately no stricter than that. An embedded newline is legal
// in a value — a multi-line credential pasted into config/env/ is a real
// thing people do — and the sinks where a newline actually matters gate it
// themselves (naming.ValidHostname for /etc/hosts, cloudflared's own
// hostname check for its config.yml).
func PlausibleValue(content []byte) bool {
	return !bytes.ContainsRune(content, 0)
}

// isJunkName reports whether name is one of the operating-system metadata
// files junkNames lists, matched the way a FAT card compares names.
func isJunkName(name string) bool {
	for _, junk := range junkNames {
		if strings.EqualFold(name, junk) {
			return true
		}
	}
	return false
}

// checkName refuses a reserved or unusable file/directory name. rel is the
// entry's path within the tree and dir the --boot-config-dir it came from, both
// only used to make the refusal name a real file on disk.
func checkName(name, rel, dir string) error {
	switch {
	case strings.HasPrefix(name, "._"):
		return fmt.Errorf("%s/%s is a macOS AppleDouble file (a \"._\" companion macOS writes for every file it touches on a FAT card); delete it, and consider copying the directory with a tool that doesn't create them", dir, rel)
	case strings.HasPrefix(name, "."):
		return fmt.Errorf("%s/%s starts with a period, which gosd reserves: the device ignores dot-files on the card, so a setting named this way would silently never take effect; rename or delete it", dir, rel)
	case strings.HasSuffix(name, NewSuffix), strings.HasSuffix(name, UnusedSuffix):
		return fmt.Errorf("%s/%s ends with a reserved suffix: the device writes \"<name>%s\" and \"<name>%s\" files onto the card itself, so a setting can't be named that; rename it", dir, rel, NewSuffix, UnusedSuffix)
	}
	if isJunkName(name) {
		return fmt.Errorf("%s/%s is an operating-system metadata file, not a setting; delete it before building", dir, rel)
	}
	if !nameShape.MatchString(name) {
		return fmt.Errorf("%s/%s has an invalid name %q; use only letters, digits, periods, hyphens and underscores, so the name survives a FAT card and a provisioning manifest unchanged", dir, rel, name)
	}
	return nil
}

// isDoc reports whether p is documentation (a group explain.md or a value's
// <name>.explain.md sidecar) rather than a setting.
func isDoc(p string) bool {
	base := path.Base(p)
	return base == GroupDoc || strings.HasSuffix(base, DocSuffix)
}

// docFor returns the sidecar path documenting the value at p.
func docFor(p string) string { return p + DocSuffix }

// validate applies every build-time gate to the merged tree, before
// pruning: a developer's mistake is refused whichever features this
// particular board's image happens to include.
func validate(files map[string]entry) error {
	paths := sortedPaths(files)

	for _, p := range paths {
		if isDoc(p) {
			if err := checkDoc(p, files); err != nil {
				return err
			}
			continue
		}
		if err := checkValue(p, files); err != nil {
			return err
		}
	}
	return checkCollisions(paths, files)
}

// checkValue gates one setting file: it must be documented, and one under
// env/ must additionally be usable as an environment variable name.
func checkValue(p string, files map[string]entry) error {
	if _, ok := files[docFor(p)]; !ok {
		return fmt.Errorf("the setting %s (%s) has no documentation: add %s describing, in plain language, what it does and what to write in it; that file is all the person holding the card ever gets, so gosd refuses to ship an undocumented setting",
			p, files[p].source, path.Join(Dir, docFor(p)))
	}
	return checkEnvValue(p, files)
}

// envNameShape is the shape an environment variable name must have to reach
// an app - the same rules any shell or exec environment already expects.
var envNameShape = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func checkEnvValue(p string, files map[string]entry) error {
	if path.Dir(p) == "." || strings.Split(p, "/")[0] != envDir {
		return nil
	}

	name, isDirect := strings.CutPrefix(p, envDir+"/")
	if !isDirect || strings.Contains(name, "/") {
		return fmt.Errorf("%s (%s) is nested inside %s/, but an environment variable's name can't contain a directory; move it to %s/<NAME>",
			p, files[p].source, envDir, envDir)
	}
	if strings.HasPrefix(name, "GOSD_") {
		return fmt.Errorf("%s (%s) is in the GOSD_* namespace gosd reserves for itself; the device ignores those names, so rename it to something else",
			p, files[p].source)
	}
	if !ValidEnvName(name) {
		return fmt.Errorf("%s (%s) isn't a valid environment variable name; use only letters, digits and underscores, and don't start with a digit",
			p, files[p].source)
	}
	return nil
}

// checkDoc gates one documentation file: a group explain.md always stands
// on its own, but a <name>.explain.md with no <name> beside it is a typo
// that would otherwise ship as documentation for a setting that doesn't
// exist.
func checkDoc(p string, files map[string]entry) error {
	if path.Base(p) == GroupDoc {
		return nil
	}
	value := strings.TrimSuffix(p, DocSuffix)
	if _, ok := files[value]; !ok {
		return fmt.Errorf("%s (%s) documents a setting %q that doesn't exist; create that setting file (an empty file means \"not set\"), or delete the documentation",
			p, files[p].source, value)
	}
	return nil
}

// checkCollisions refuses two paths a FAT card could not hold at once: two
// names differing only in case (FAT is case-insensitive, which matters most
// for environment variable names), and a setting whose name is also a
// directory.
func checkCollisions(paths []string, files map[string]entry) error {
	seen := make(map[string]string, len(paths))
	for _, p := range paths {
		lower := strings.ToLower(p)
		if existing, dup := seen[lower]; dup {
			return fmt.Errorf("%s (%s) and %s (%s) differ only in capitalization; a FAT memory card can't hold both, so rename one of them",
				p, files[p].source, existing, files[existing].source)
		}
		seen[lower] = p
	}

	dirs := make(map[string]bool, len(paths))
	for _, p := range paths {
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			dirs[strings.ToLower(dir)] = true
		}
	}
	for _, p := range paths {
		if isDoc(p) {
			continue
		}
		if dirs[strings.ToLower(p)] {
			return fmt.Errorf("%s (%s) is both a setting and a directory holding other settings; a memory card can't hold both, so rename one of them",
				p, files[p].source)
		}
	}
	return nil
}

// prune drops the directories whose feature this image doesn't carry, so a
// card only ever documents settings the device it came from can act on. The
// ingress group itself goes when no ingress agent is baked in at all.
func prune(files map[string]entry, features Features) {
	if !features.IngressCloudflared {
		pruneDir(files, "ingress/cloudflared")
	}
	if !features.IngressTailscaleFunnel {
		pruneDir(files, "ingress/tailscale-funnel")
	}
	if !features.IngressCloudflared && !features.IngressTailscaleFunnel {
		pruneDir(files, "ingress")
	}
}

func pruneDir(files map[string]entry, dir string) {
	for p := range files {
		if p == dir || strings.HasPrefix(p, dir+"/") {
			delete(files, p)
		}
	}
}

// assemble turns the validated, pruned file set into a Tree: documentation
// verbatim, settings padded out to their reservation.
func assemble(files map[string]entry) Tree {
	tree := Tree{}
	for _, p := range sortedPaths(files) {
		content := files[p].content
		if isDoc(p) {
			tree.Docs = append(tree.Docs, Doc{Path: p, Content: content})
			continue
		}
		tree.Values = append(tree.Values, Value{
			Path:    p,
			Content: pad(content),
			Value:   TrimValue(content),
		})
	}
	return tree
}

// pad writes a value's shipped bytes out to its reservation: trailing
// newlines up to MinValueBytes, or nothing at all when the file already
// reserves more than that itself. Newlines rather than NULs or spaces
// because the padding has to read back as part of an unset (or unchanged)
// value, and because a person who opens the file sees blank lines rather
// than mojibake.
func pad(content []byte) []byte {
	size := max(len(content), MinValueBytes)
	padded := make([]byte, size)
	copy(padded, content)
	for i := len(content); i < size; i++ {
		padded[i] = '\n'
	}
	return padded
}

func sortedPaths(files map[string]entry) []string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
