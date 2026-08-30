package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/build"
)

var initForce bool

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [path-to-main-package]",
		Short: "Write a starter gosd-build.toml in the working directory",
		Long: `Write a gosd-build.toml in the working directory: docs/build-config.md's own
example, fully commented, with [app].main, [app].version and label-prefix
filled in wherever they can be confirmed, and every other key left as a
commented example to uncomment and edit.

Detection is best-effort and only fills in what it can confirm: a main
package only defaults when the working directory, or a single unambiguous
cmd/* subdirectory, is confirmed to be package main; version only defaults
to a git: source when the repository is confirmed to have at least one
tag. Refuses to overwrite an existing gosd-build.toml unless --force is
given.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInit,
	}
	cmd.Flags().BoolVarP(&initForce, "force", "f", false, "overwrite an existing gosd-build.toml in the working directory")
	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("gosd init: finding the working directory failed: %w", err)
	}

	var pkgPath, dir string
	if len(args) == 1 {
		if err := validatePkgPath(args[0]); err != nil {
			return err
		}
		dir, err = filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolving %q to an absolute path failed: %w; check the path exists and is accessible", args[0], err)
		}
		if !build.IsMainPackage(args[0]) {
			return fmt.Errorf("%s is not package main; gosd init needs the path to your app's runnable command (package main with a func main)", args[0])
		}
		pkgPath = args[0]
	} else {
		pkgPath, dir = detectMainPackage(cwd)
	}

	labelPrefix := detectLabelPrefix(dir)
	hasTag := detectVersionSource(dir)

	if _, err := os.Stat(defaultBuildConfigFile); err == nil && !initForce {
		return fmt.Errorf("%s already exists in the working directory; pass --force to overwrite it", defaultBuildConfigFile)
	}

	rendered := renderInitTemplate(pkgPath, hasTag, labelPrefix)
	if err := os.WriteFile(defaultBuildConfigFile, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("writing %s failed: %w; check the working directory is writable", defaultBuildConfigFile, err)
	}

	cmd.Printf("gosd init: wrote %s — edit the commented options you need (see docs/build-config.md), then run 'gosd build'.\n", defaultBuildConfigFile)
	return nil
}

// commentedMainLine is [app].main's commented example, written when gosd
// init could not confirm any main package.
const commentedMainLine = "# main = \"./cmd/myapp\"  # the app to build, so a bare `gosd build` works"

// commentedVersionBlock is [app].version's commented example, written when
// gosd init could not confirm the enclosing repository has any git tags.
const commentedVersionBlock = "# version = \"1.4.2\"          # --app-version; or \"git:v*.*.*\" to resolve\n" +
	"#                            # from your repo's tags at build time (see docs/build-config.md)"

// initTemplate is the starter gosd-build.toml gosd init writes: docs/build-
// config.md's own example, with every key other than {{MAIN}}, {{VERSION}}
// and label-prefix left as a commented illustration - copying that doc's
// live example values into a fresh file would silently change what gets
// built. renderInitTemplate substitutes its three tokens.
const initTemplate = "# gosd-build.toml — checked-in defaults for `gosd build` (and the subset\n" +
	"# `gosd run` shares). Written by `gosd init`; every key is the flag of the\n" +
	"# same name, and a flag passed on the command line always wins. See\n" +
	"# docs/build-config.md for the full reference.\n" +
	`
# Boards to build; omit to build every supported board.
# board = ["pi-zero-2w", "radxa-zero-3e"]

# Output directory (several boards) or .img path (one board) — same as -o.
# output = "dist"

# Partition-label prefix — part of your app's on-disk layout (changing it
# after your first release re-establishes an upgrading device's data
# partition on reflash). Derived here from this directory's name.
label-prefix = "{{LABEL_PREFIX}}"

# Bake in an internet tunnel client (same values as --ingress).
# ingress = ["tailscale-funnel"]

# Fixed-size placeholder files on the boot partition, for provisioning
# tools that splice bytes into a downloaded image. Paths are *on the
# image*, never resolved against this file.
# placeholder = ["provision.yaml=32KiB"]

# A prebuilt static companion binary, <path>[:<dest>] — the path half is
# relative to this file; dest is an absolute path inside the image.
# with-external = ["./third_party/mpv:/bin/mpv"]

[app]
{{MAIN}}
{{VERSION}}
# support-url = "https://example.com/support"   # --app-support-url

# Changing boot size, data filesystem, or label-prefix in a later release
# changes the app's on-disk layout: an upgrading device's existing data
# partition is erased and re-established.
[boot]
# size = "256MiB"            # --boot-size
# config-dir = "config"      # --boot-config-dir: on-card setting overlays

[data]
# size = "512MiB"            # --data-size
# filesystem = "fat32"       # --data-filesystem
# flush = false              # --data-flush

[kernel]
# param = ["snd_bcm2835.enable_hdmi=1"]   # --kernel-param
# config = "gosd-kernel.toml"             # --kernel-config

[publish]
# catalog = true             # --publish-catalog: emit rpi-imager os_list.json
# base-url = "https://example.com/downloads"    # --publish-base-url

# Also honoured, less commonly checked in:
#   usb-gadget = true
#   console-baud = 115200
#   artifacts-dir = "gosd-artifacts"
#   gosd-init-src = "../gosd/gosd-init"
#   ldflags = "-X main.version={{.AppVersion}}"
#   tags = "myfeature"
#   trimpath = true
#   gcflags = "-m"
#   asmflags = "-D FOO=1"
`

// renderInitTemplate fills initTemplate's three tokens: {{MAIN}} and
// {{VERSION}} default to commented examples unless pkgPath/hasTag confirm a
// live value to write instead, and {{LABEL_PREFIX}} is always filled in
// (detectLabelPrefix never returns empty). {{.AppVersion}} in the template's
// trailing "also honoured" block is a different, literal string that
// strings.NewReplacer never matches, so it survives untouched.
func renderInitTemplate(pkgPath string, hasTag bool, labelPrefix string) string {
	mainBlock := commentedMainLine
	if pkgPath != "" {
		mainBlock = fmt.Sprintf("main = %q       # the app to build, so a bare `gosd build` works", pkgPath)
	}

	versionBlock := commentedVersionBlock
	if hasTag {
		versionBlock = "version = \"git:v*.*.*\"    # --app-version; resolved from your repo's tags\n" +
			"                            # at build time (see docs/build-config.md)"
	}

	return strings.NewReplacer(
		"{{MAIN}}", mainBlock,
		"{{VERSION}}", versionBlock,
		"{{LABEL_PREFIX}}", labelPrefix,
	).Replace(initTemplate)
}
