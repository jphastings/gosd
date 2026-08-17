package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/spf13/cobra"
)

// versionInfo is what `gosd version` reports. The artifacts pin is the point
// of the command: which CLI you have matters mostly because it decides which
// board kernels and bootloaders your images are built from, and that is not
// otherwise discoverable from an installed binary (bean gosd-7frv — a board
// that would not boot was traced to a release whose pin predated the fix).
type versionInfo struct {
	// CLI is the module version for a `go install`ed binary, or "(devel)"
	// for one built from a checkout. Empty when the binary carries no build
	// information at all.
	CLI string
	// Revision and Modified come from the VCS stamps the Go toolchain adds
	// when building inside a repository; both are absent for a binary built
	// from a module download, which has no repository to stamp.
	Revision string
	Modified bool
	// Artifacts is the board-artifact release this binary downloads.
	Artifacts string
	// Go is the toolchain that built the binary.
	Go string
}

// readVersionInfo gathers what the binary knows about itself. It never fails:
// build information is absent in some legitimate cases (a binary built with
// -buildinfo stripped, or one constructed by a test), and a version command
// that errors in those cases is less useful than one that says what it can.
func readVersionInfo(bi *debug.BuildInfo, ok bool) versionInfo {
	info := versionInfo{Artifacts: artifacts.Version, Go: runtime.Version()}
	if !ok || bi == nil {
		return info
	}
	info.CLI = bi.Main.Version
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

// write renders the report. It returns the write error so a caller printing
// to something fallible knows; the command itself passes cobra's output.
func (v versionInfo) write(w io.Writer) error {
	cli := v.CLI
	if cli == "" {
		cli = "unknown (built without version information)"
	}
	if v.Revision != "" {
		short := v.Revision
		if len(short) > 12 {
			short = short[:12]
		}
		cli += " (" + short
		if v.Modified {
			cli += ", modified"
		}
		cli += ")"
	}

	_, err := fmt.Fprintf(w, "gosd:      %s\nartifacts: %s\ngo:        %s\n", cli, v.Artifacts, v.Go)
	return err
}

// renderedVersion is the same report the version subcommand prints, for the
// root command's --version flag.
func renderedVersion() string {
	var b strings.Builder
	// A strings.Builder never fails to write.
	_ = readVersionInfo(debug.ReadBuildInfo()).write(&b)
	return b.String()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print this gosd's version and the board artifacts it builds from",
		Long: "Print this gosd's version and the board artifacts it builds from.\n\n" +
			"The artifacts line is the one that changes what your images contain: it\n" +
			"names the release of board kernels and bootloaders `gosd build` downloads.\n" +
			"Two gosd versions with the same artifacts pin produce equivalent boot\n" +
			"chains; a board that boots with one gosd and not another usually differs\n" +
			"here.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return readVersionInfo(debug.ReadBuildInfo()).write(cmd.OutOrStdout())
		},
	}
}
