// Package buildconfig parses gosd-build.toml, the developer-authored,
// checked-in file of gosd build (and gosd run) options (bean gosd-mwct).
//
// Every key maps to the flag of the same name, structurally: a flag
// --<section>-<rest> whose <section> names one of Config's table fields
// appears in the file as `rest` under [section]; every other flag is a
// top-level key spelled exactly like the flag. The one key with no flag is
// [app].main, which supplies gosd build's positional package-path operand.
// Precedence is decided by the caller (cmd/gosd): a flag given on the
// command line always wins over the file.
//
// Parsing is strict (bean gosd-hkp7): like gosd-kernel.toml, this is a
// developer-authored build input, so any key Parse doesn't recognize -
// anywhere in the file - is an error naming the offending key, not
// silently ignored. Parse does no semantic validation beyond TOML types:
// merged values flow through the same cmd/gosd helpers that validate the
// flags, so a bad value gets the identical actionable error either way.
package buildconfig

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/BurntSushi/toml"
)

// Config is the parsed gosd-build.toml. Each field carries the value in
// the shape the matching flag takes it: repeatable flags are string
// arrays, and composite grammars (placeholder's "<path>=<size>",
// with-external's "<path>[:<dest>]") stay in their flag string form.
type Config struct {
	Board        []string `toml:"board"`
	Output       string   `toml:"output"`
	LabelPrefix  string   `toml:"label-prefix"`
	Ingress      []string `toml:"ingress"`
	Placeholder  []string `toml:"placeholder"`
	WithExternal []string `toml:"with-external"`
	UsbGadget    bool     `toml:"usb-gadget"`
	ConsoleBaud  int      `toml:"console-baud"`
	ArtifactsDir string   `toml:"artifacts-dir"`
	GosdInitSrc  string   `toml:"gosd-init-src"`

	// LDFlags, Tags, TrimPath, GCFlags and ASMFlags mirror gosd build's
	// --ldflags/--tags/--trimpath/--gcflags/--asmflags (bean gosd-wjjn):
	// deliberately top-level, not nested under [app], even though they
	// only affect the app compile - the flags are kept bare
	// (--ldflags, not --app-ldflags) to match `go build`'s own flag names
	// for muscle memory, and the flag<->key mapping is structural (see
	// this package's doc comment), so a bare flag is always a top-level
	// key.
	LDFlags  string `toml:"ldflags"`
	Tags     string `toml:"tags"`
	TrimPath bool   `toml:"trimpath"`
	GCFlags  string `toml:"gcflags"`
	ASMFlags string `toml:"asmflags"`

	App     App     `toml:"app"`
	Boot    Boot    `toml:"boot"`
	Data    Data    `toml:"data"`
	Kernel  Kernel  `toml:"kernel"`
	Publish Publish `toml:"publish"`

	// defined records every dotted key path the file actually wrote, so
	// IsSet can tell `label-prefix = ""` (set, and an error downstream,
	// same as --label-prefix="") from the key being absent. TOML has no
	// null: written-as-zero is set.
	defined map[string]bool
}

// App is the [app] table: what is being built and how it identifies itself.
type App struct {
	// Main supplies gosd build/run's positional package-path operand, so a
	// bare `gosd build` works in a checked-out repo. A positional argument
	// on the command line wins. Relative filesystem forms (./x, ../x, .)
	// are resolved against the file's own directory by the caller.
	Main       string `toml:"main"`
	Version    string `toml:"version"`
	SupportURL string `toml:"support-url"`
}

// Boot is the [boot] table (--boot-* flags).
type Boot struct {
	Size      string `toml:"size"`
	ConfigDir string `toml:"config-dir"`
}

// Data is the [data] table (--data-* flags).
type Data struct {
	Size       string `toml:"size"`
	Filesystem string `toml:"filesystem"`
	Flush      bool   `toml:"flush"`
}

// Kernel is the [kernel] table (--kernel-* flags).
type Kernel struct {
	Param  []string `toml:"param"`
	Config string   `toml:"config"`
}

// Publish is the [publish] table (--publish-* flags).
type Publish struct {
	Catalog bool   `toml:"catalog"`
	BaseURL string `toml:"base-url"`
}

// Parse parses gosd-build.toml's contents into a Config. Missing data (nil
// or empty, as when no file exists in the working directory) yields a zero
// Config and no error - every key is optional.
func Parse(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var cfg Config
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&cfg)
	if err != nil {
		return Config{}, fmt.Errorf("gosd-build.toml: %w", err)
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf(
			"gosd-build.toml has an unknown key %q; every key is a gosd build flag name (--<section>-<rest> flags live under [section])",
			keys[0])
	}

	cfg.defined = make(map[string]bool)
	for _, k := range md.Keys() {
		cfg.defined[k.String()] = true
	}
	return cfg, nil
}

// IsSet reports whether the file wrote the dotted key path (e.g. "board",
// "boot.size"), even to its type's zero value.
func (c Config) IsSet(key string) bool {
	return c.defined[key]
}

// Keys returns every key path the schema recognizes, sorted, derived by
// reflection over Config's toml tags - never a hand-maintained list, so
// cmd/gosd's parity test can hold the schema and the flag set to each
// other structurally.
func Keys() []string {
	var keys []string
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("toml")
		if !f.IsExported() || tag == "" {
			continue
		}
		if f.Type.Kind() == reflect.Struct {
			for j := 0; j < f.Type.NumField(); j++ {
				keys = append(keys, tag+"."+f.Type.Field(j).Tag.Get("toml"))
			}
			continue
		}
		keys = append(keys, tag)
	}
	sort.Strings(keys)
	return keys
}

// ResolvePath resolves a path written in the file against baseDir - the
// directory gosd-build.toml itself lives in, matching how a developer
// editing the file expects a relative path to behave (and how
// gosd-kernel.toml and gosd-external.toml already behave). Absolute paths
// pass through.
func ResolvePath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
