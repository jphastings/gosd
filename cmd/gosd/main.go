// Command gosd cross-compiles a Go application and assembles it into a
// bootable SD-card image for a supported board.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "gosd",
		Short:         "Turn a Go application into a bootable SD-card image",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newBuildCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newBuildKernelCmd())
	cmd.AddCommand(newBuildExternalCmd())
	cmd.AddCommand(newCacheCmd())
	cmd.AddCommand(newVersionCmd())

	// --version is what people try first, so it prints exactly what the
	// version subcommand does rather than cobra's bare version string.
	cmd.Version = renderedVersion()
	cmd.SetVersionTemplate("{{.Version}}")
	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "gosd:", err)
		os.Exit(1)
	}
}
